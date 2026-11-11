package mcp

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/util"
)

var supportedVersions = []string{"2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26"}

const modernVersion = "2026-07-28"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Meta    json.RawMessage `json:"_meta"`
}

type operation struct {
	Method      string
	Path        string
	Name        string
	Description string
	AdminOnly   bool
	UserOnly    bool
	PathParams  []string
}

type Server struct {
	Router  chi.Router
	Config  *config.AppConfig
	DB      *sql.DB
	UsersDB *sql.DB
	APIPath string
	Version string
}

func NewServer(router chi.Router, cfg *config.AppConfig, db *sql.DB, version string) *Server {
	apiPath := "/api/v1"
	if cfg != nil {
		apiPath = cfg.GetAPIPath()
	}
	if version == "" {
		version = "dev"
	}
	return &Server{Router: router, Config: cfg, DB: db, UsersDB: database.GetUsersDB(), APIPath: apiPath, Version: version}
}

func (s *Server) Enabled() bool {
	defaultValue := true
	if s.Config != nil {
		defaultValue = s.Config.Server.MCP.Enabled
	}
	if s.DB == nil {
		return defaultValue
	}
	return (&model.SettingsModel{DB: s.DB}).GetBool("server.mcp.enabled", defaultValue)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.Enabled() {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	accept := r.Header.Get("Accept")
	if !strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/event-stream") {
		http.Error(w, "not acceptable", http.StatusNotAcceptable)
		return
	}
	contentType, _, contentTypeErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentTypeErr != nil || contentType != "application/json" {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	if !s.validOrigin(r) {
		s.writeRPCError(w, nil, -32000, "Forbidden origin", http.StatusForbidden)
		return
	}
	var request rpcRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.JSONRPC != "2.0" || request.Method == "" {
		s.writeRPCError(w, nil, -32700, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	if err := ensureEOF(decoder); err != nil {
		s.writeRPCError(w, request.ID, -32600, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	if !s.validHeaders(r, request) {
		s.writeRPCError(w, request.ID, -32020, "HeaderMismatch", http.StatusBadRequest)
		return
	}
	protocolVersion := requestProtocolVersion(r, request)
	modern := protocolVersion == modernVersion
	if modern {
		if !contains(supportedVersions, protocolVersion) {
			s.writeUnsupportedVersion(w, request.ID, protocolVersion)
			return
		}
		if err := requireModernHeaders(r, request); err != nil {
			s.writeRPCError(w, request.ID, -32020, "HeaderMismatch", http.StatusBadRequest)
			return
		}
	} else if protocolVersion != "" && !contains(supportedVersions, protocolVersion) {
		s.writeUnsupportedVersion(w, request.ID, protocolVersion)
		return
	}
	if request.Method == "initialize" {
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(request.Params, &params)
		if params.ProtocolVersion != "" && !contains(supportedVersions, params.ProtocolVersion) {
			s.writeUnsupportedVersion(w, request.ID, params.ProtocolVersion)
			return
		}
		if version := r.Header.Get("MCP-Protocol-Version"); version != "" && params.ProtocolVersion != "" && version != params.ProtocolVersion {
			s.writeRPCError(w, request.ID, -32600, "HeaderMismatch", http.StatusBadRequest)
			return
		}
	}
	if request.Method == "notifications/initialized" || request.Method == "notifications/cancelled" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if len(request.ID) == 0 || bytes.Equal(request.ID, []byte("null")) {
		s.writeRPCError(w, nil, -32600, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	var result interface{}
	switch request.Method {
	case "server/discover":
		if protocolVersion != "" && protocolVersion != modernVersion {
			s.writeRPCError(w, request.ID, -32601, "Method not found", http.StatusOK)
			return
		}
		result = map[string]interface{}{"supportedVersions": supportedVersions, "capabilities": map[string]interface{}{"tools": map[string]interface{}{"listChanged": false}}, "ttlMs": 0, "cacheScope": "private"}
	case "initialize":
		if modern {
			s.writeRPCError(w, request.ID, -32601, "Method not found", http.StatusOK)
			return
		}
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(request.Params, &params)
		if params.ProtocolVersion == modernVersion {
			s.writeRPCError(w, request.ID, -32601, "Method not found", http.StatusOK)
			return
		}
		result = map[string]interface{}{
			"protocolVersion": negotiatedVersion(r, request.Params),
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{"listChanged": false}},
			"serverInfo":      map[string]string{"name": "wthr", "version": s.Version},
		}
	case "ping":
		if modern {
			s.writeRPCError(w, request.ID, -32601, "Method not found", http.StatusOK)
			return
		}
		result = map[string]interface{}{}
	case "tools/list":
		result = map[string]interface{}{"tools": s.tools(r)}
		if modern {
			result.(map[string]interface{})["ttlMs"] = 0
			result.(map[string]interface{})["cacheScope"] = "private"
		}
	case "tools/call":
		result = s.callTool(r, request.Params)
	default:
		s.writeRPCError(w, request.ID, -32601, "Method not found", http.StatusOK)
		return
	}
	if modern {
		value, ok := result.(map[string]interface{})
		if !ok {
			value = map[string]interface{}{"value": result}
		}
		value["resultType"] = "complete"
		value["_meta"] = map[string]interface{}{"io.modelcontextprotocol/serverInfo": map[string]string{"name": "wthr", "version": s.Version}}
		result = value
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "result": result})
}

func requestProtocolVersion(r *http.Request, request rpcRequest) string {
	if header := r.Header.Get("MCP-Protocol-Version"); header != "" {
		return header
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(request.Meta, &metadata); err == nil {
		var version string
		_ = json.Unmarshal(metadata["io.modelcontextprotocol/protocolVersion"], &version)
		return version
	}
	var params struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	_ = json.Unmarshal(request.Params, &params)
	var version string
	_ = json.Unmarshal(params.Meta["io.modelcontextprotocol/protocolVersion"], &version)
	return version
}

func (s *Server) writeUnsupportedVersion(w http.ResponseWriter, id json.RawMessage, requested string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": id, "error": map[string]interface{}{"code": -32022, "message": "Unsupported protocol version", "data": map[string]interface{}{"supported": supportedVersions, "requested": requested}}})
}

func requireModernHeaders(r *http.Request, request rpcRequest) error {
	if r.Header.Get("Mcp-Method") == "" || r.Header.Get("MCP-Protocol-Version") == "" {
		return fmt.Errorf("missing required MCP headers")
	}
	var params map[string]interface{}
	_ = json.Unmarshal(request.Params, &params)
	meta, ok := params["_meta"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing protocol metadata")
	}
	protocolVersion, ok := meta["io.modelcontextprotocol/protocolVersion"].(string)
	if !ok || protocolVersion != r.Header.Get("MCP-Protocol-Version") {
		return fmt.Errorf("missing or mismatched protocol version")
	}
	if _, ok := meta["io.modelcontextprotocol/clientCapabilities"].(map[string]interface{}); !ok {
		return fmt.Errorf("missing client capabilities")
	}
	name, hasName := params["name"].(string)
	if !hasName {
		name, hasName = params["uri"].(string)
	}
	if hasName && (name == "" || r.Header.Get("Mcp-Name") != name) {
		return fmt.Errorf("missing or mismatched MCP name")
	}
	if !hasName && contains([]string{"tools/call", "resources/read", "prompts/get"}, request.Method) {
		return fmt.Errorf("missing required MCP name parameter")
	}
	if !hasName && r.Header.Get("Mcp-Name") != "" {
		return fmt.Errorf("unexpected MCP name")
	}
	return nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected trailing JSON")
	}
	return nil
}

func (s *Server) validOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	current, err := url.Parse(util.BuildURL(r, ""))
	if err == nil && sameOrigin(parsed, current) {
		return true
	}
	if s.Config != nil && s.Config.Server.FQDN != "" {
		expectedScheme := "http"
		if s.Config.Server.SSL.Enabled {
			expectedScheme = "https"
		}
		host := strings.TrimSuffix(strings.Trim(s.Config.Server.FQDN, "[]"), ".")
		if strings.EqualFold(parsed.Scheme, expectedScheme) && strings.EqualFold(parsed.Hostname(), host) {
			port := parsed.Port()
			if port == "" || port == defaultOriginPort(expectedScheme) {
				return true
			}
		}
	}
	return false
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && originPort(a) == originPort(b)
}

func originPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	return defaultOriginPort(u.Scheme)
}

func defaultOriginPort(scheme string) string {
	if strings.EqualFold(scheme, "https") {
		return "443"
	}
	return "80"
}

func (s *Server) validHeaders(r *http.Request, request rpcRequest) bool {
	if value := r.Header.Get("MCP-Protocol-Version"); value != "" {
		bodyVersion := requestProtocolVersion(&http.Request{}, request)
		if request.Method == "initialize" {
			var init struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(request.Params, &init)
			bodyVersion = init.ProtocolVersion
		}
		if bodyVersion != "" && bodyVersion != value {
			return false
		}
	}
	if value := r.Header.Get("Mcp-Method"); value != "" && value != request.Method {
		return false
	}
	if value := r.Header.Get("Mcp-Name"); value != "" {
		var params map[string]interface{}
		_ = json.Unmarshal(request.Params, &params)
		name, _ := params["name"].(string)
		if name == "" {
			name, _ = params["uri"].(string)
		}
		if name == "" || name != value {
			return false
		}
	}
	return true
}

func negotiatedVersion(r *http.Request, params json.RawMessage) string {
	if header := r.Header.Get("MCP-Protocol-Version"); header != "" {
		return header
	}
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &init)
	if contains(supportedVersions, init.ProtocolVersion) {
		return init.ProtocolVersion
	}
	return "2025-11-25"
}

func (s *Server) writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": id, "error": map[string]interface{}{"code": code, "message": message}})
}

func (s *Server) operations() []operation {
	seen := map[string]bool{}
	var ops []operation
	_ = chi.Walk(s.Router, func(method, route string, h http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, s.APIPath+"/") || strings.Contains(route, "/server/mcp") || strings.HasSuffix(route, "/*") {
			return nil
		}
		method = strings.ToUpper(method)
		if !contains([]string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}, method) {
			return nil
		}
		key := method + " " + route
		if seen[key] {
			return nil
		}
		seen[key] = true
		name := operationID(method, route)
		fn := "API operation"
		if h != nil {
			if value := reflect.ValueOf(h); value.IsValid() && value.Kind() == reflect.Func {
				if runtimeFn := runtime.FuncForPC(value.Pointer()); runtimeFn != nil {
					fn = strings.TrimPrefix(runtimeFn.Name(), "github.com/webappsgo/wthr/")
				}
			}
		}
		adminPath := "administration"
		if s.Config != nil && s.Config.GetAdminPath() != "" {
			adminPath = s.Config.GetAdminPath()
		}
		adminOnly := strings.Contains(route, "/server/"+adminPath+"/")
		userOnly := strings.HasPrefix(route, s.APIPath+"/users/") || route == s.APIPath+"/server/auth/logout" || route == s.APIPath+"/server/auth/refresh"
		ops = append(ops, operation{Method: method, Path: route, Name: name, Description: method + " " + route + " (" + fn + ")", AdminOnly: adminOnly, UserOnly: userOnly, PathParams: pathParams(route)})
		return nil
	})
	sort.Slice(ops, func(i, j int) bool { return ops[i].Name < ops[j].Name })
	return ops
}

func operationID(method, route string) string {
	route = normalizeRoute(route)
	var out strings.Builder
	for _, part := range strings.FieldsFunc(route, func(r rune) bool { return r == '/' || r == '{' || r == '}' || r == '-' || r == '_' }) {
		if part == "api" || part == "v1" || part == "v2" {
			continue
		}
		if len(part) > 0 {
			out.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return strings.ToLower(method) + out.String()
}

func pathParams(route string) []string {
	var out []string
	for rest := route; ; {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			break
		}
		rest = rest[start+1:]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			break
		}
		out = append(out, strings.SplitN(rest[:end], ":", 2)[0])
		rest = rest[end+1:]
	}
	return out
}

func normalizeRoute(route string) string {
	var out strings.Builder
	for rest := route; ; {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			out.WriteString(rest)
			break
		}
		out.WriteString(rest[:start+1])
		rest = rest[start+1:]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			out.WriteString(rest)
			break
		}
		out.WriteString(strings.SplitN(rest[:end], ":", 2)[0])
		out.WriteByte('}')
		rest = rest[end+1:]
	}
	return out.String()
}

func substitutePathParam(route, name, value string) string {
	for rest, offset := route, 0; ; {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			return route
		}
		end := strings.IndexByte(rest[start+1:], '}')
		if end < 0 {
			return route
		}
		end += start + 1
		key := strings.SplitN(rest[start+1:end], ":", 2)[0]
		if key == name {
			return route[:offset+start] + value + route[offset+end+1:]
		}
		next := end + 1
		offset += next
		rest = rest[next:]
	}
}

func (s *Server) tools(r *http.Request) []map[string]interface{} {
	admin := s.operator(r)
	user := s.user(r)
	var tools []map[string]interface{}
	for _, op := range s.operations() {
		if op.AdminOnly && !admin || op.UserOnly && !user {
			continue
		}
		props := map[string]interface{}{}
		required := []string{}
		for _, name := range op.PathParams {
			props[name] = map[string]string{"type": "string", "description": "Path parameter"}
			required = append(required, name)
		}
		props["query"] = map[string]interface{}{"type": "object", "description": "URL query parameters", "additionalProperties": true}
		props["body"] = map[string]interface{}{"type": "object", "description": "JSON request body", "additionalProperties": true}
		tools = append(tools, map[string]interface{}{"name": op.Name, "description": op.Description, "inputSchema": map[string]interface{}{"type": "object", "properties": props, "required": required, "additionalProperties": false}})
	}
	return tools
}

func (s *Server) OpenAPI(w http.ResponseWriter, _ *http.Request) {
	paths := make(map[string]map[string]interface{})
	for _, op := range s.operations() {
		path := strings.TrimPrefix(normalizeRoute(op.Path), s.APIPath)
		item := paths[path]
		if item == nil {
			item = make(map[string]interface{})
			paths[path] = item
		}
		parameters := make([]map[string]interface{}, 0, len(op.PathParams))
		for _, name := range op.PathParams {
			parameters = append(parameters, map[string]interface{}{"name": name, "in": "path", "required": true, "schema": map[string]string{"type": "string"}})
		}
		item[strings.ToLower(op.Method)] = map[string]interface{}{
			"operationId": op.Name,
			"summary":     op.Description,
			"description": op.Description,
			"parameters":  parameters,
			"requestBody": map[string]interface{}{"required": false, "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": map[string]interface{}{"type": "object", "additionalProperties": true}}}},
			"responses":   map[string]interface{}{"200": map[string]interface{}{"description": "API response"}},
			"security":    []map[string][]string{{}, {"BearerAuth": {}}},
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"openapi":    "3.0.3",
		"info":       map[string]string{"title": "wthr API", "version": "1.0.0", "description": "Generated from the live API route definitions."},
		"servers":    []map[string]string{{"url": s.APIPath}},
		"paths":      paths,
		"components": map[string]interface{}{"securitySchemes": map[string]interface{}{"BearerAuth": map[string]string{"type": "http", "scheme": "bearer"}}},
	})
}

func (s *Server) LLMS(w http.ResponseWriter, r *http.Request) {
	if s.Config != nil && !s.Config.Web.LLMS.Enabled {
		http.NotFound(w, r)
		return
	}
	settings := &model.SettingsModel{DB: s.DB}
	name := "wthr"
	description := "Weather API and web application."
	if s.Config != nil {
		if s.Config.Server.Branding.Title != "" {
			name = s.Config.Server.Branding.Title
		}
		if strings.TrimSpace(s.Config.Server.Branding.Description) != "" {
			description = strings.TrimSpace(s.Config.Server.Branding.Description)
		} else if strings.TrimSpace(s.Config.Server.Branding.Tagline) != "" {
			description = strings.TrimSpace(s.Config.Server.Branding.Tagline)
		}
	}
	baseURL := util.BuildURL(r, "")
	apiURL := util.BuildURL(r, s.APIPath)
	readLimit := 120
	readWindow := 60
	if s.Config != nil && s.Config.Server.RateLimit.Read.Requests > 0 {
		readLimit = s.Config.Server.RateLimit.Read.Requests
		if s.Config.Server.RateLimit.Read.Window > 0 {
			readWindow = s.Config.Server.RateLimit.Read.Window
		}
	}
	var output strings.Builder
	fmt.Fprintf(&output, "# %s\n\n> %s\n\n", name, description)
	output.WriteString("## API\n\n")
	fmt.Fprintf(&output, "Base URL: %s\n", apiURL)
	output.WriteString("Authentication: Bearer token where required by an endpoint\n")
	fmt.Fprintf(&output, "Rate limit: %d requests per %d seconds\n", readLimit, readWindow)
	fmt.Fprintf(&output, "MCP: %s/mcp (Streamable HTTP; same Bearer token as the API)\n\n", baseURL)
	if s.Config == nil || s.Config.Web.LLMS.IncludeEndpoints {
		output.WriteString("## Endpoints\n\n")
		for _, op := range s.operations() {
			if op.AdminOnly || strings.Contains(op.Path, "/server/metrics") || strings.Contains(op.Path, "/internal/") {
				continue
			}
			path := strings.TrimPrefix(normalizeRoute(op.Path), s.APIPath)
			description := strings.TrimPrefix(op.Description, op.Method+" "+op.Path+" ")
			fmt.Fprintf(&output, "- %s %s - %s\n", op.Method, path, description)
			if s.Config != nil && s.Config.Web.LLMS.IncludeSchemas && len(op.PathParams) > 0 {
				fmt.Fprintf(&output, "  Request path parameters: %s.\n", strings.Join(op.PathParams, ", "))
			}
		}
		output.WriteString("- POST /server/mcp - MCP endpoint for the same API tools\n\n")
	}
	output.WriteString("## Capabilities\n\n")
	capabilities := []struct{ route, label string }{
		{"weather", "Current weather and forecasts"},
		{"severe-weather", "Severe weather alerts"},
		{"earthquakes", "Earthquake data"},
		{"hurricanes", "Hurricane tracking"},
		{"moon", "Moon phase and lunar data"},
		{"locations", "Location search and lookup"},
	}
	available := make(map[string]bool, len(capabilities))
	for _, op := range s.operations() {
		if op.AdminOnly {
			continue
		}
		for _, capability := range capabilities {
			if strings.Contains(op.Path, "/"+capability.route) {
				available[capability.route] = true
			}
		}
	}
	for _, capability := range capabilities {
		if available[capability.route] {
			fmt.Fprintf(&output, "- %s\n", capability.label)
		}
	}
	var apiContact, securityContact string
	if s.DB != nil {
		apiContact = settings.GetString("web.api_contact", "")
		securityContact = settings.GetString("security.contact", "")
	}
	if apiContact != "" || securityContact != "" {
		output.WriteString("\n## Contact\n\n")
		if apiContact != "" {
			fmt.Fprintf(&output, "API issues: %s\n", apiContact)
		}
		if securityContact != "" {
			fmt.Fprintf(&output, "Security: %s\n", securityContact)
		}
	}
	if s.Config != nil {
		for _, section := range s.Config.Web.LLMS.CustomSections {
			if strings.TrimSpace(section) != "" {
				output.WriteString("\n")
				output.WriteString(strings.TrimSpace(section))
				output.WriteString("\n")
			}
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, output.String())
}

func (s *Server) operator(r *http.Request) bool {
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return false
	}
	typeOfToken := strings.TrimSpace(parts[1])
	if !strings.HasPrefix(typeOfToken, "adm_") {
		return false
	}
	_, err := (&model.AdminModel{DB: s.DB}).GetByAPIToken(typeOfToken)
	return err == nil
}

func (s *Server) user(r *http.Request) bool {
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" || !strings.HasPrefix(parts[1], "usr_") {
		return false
	}
	usersDB := s.UsersDB
	if usersDB == nil {
		usersDB = s.DB
	}
	token, err := (&model.TokenModel{DB: usersDB}).GetByToken(strings.TrimSpace(parts[1]))
	if err != nil {
		return false
	}
	user, err := (&model.UserModel{DB: usersDB}).GetByID(int64(token.UserID))
	return err == nil && user.Role != "admin"
}

func (s *Server) callTool(r *http.Request, raw json.RawMessage) interface{} {
	var call struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil || call.Name == "" {
		return map[string]interface{}{"content": []interface{}{map[string]string{"type": "text", "text": "invalid tool call"}}, "isError": true}
	}
	var selected *operation
	for _, op := range s.operations() {
		if op.Name == call.Name {
			copy := op
			selected = &copy
			break
		}
	}
	if selected == nil || selected.AdminOnly && !s.operator(r) || selected.UserOnly && !s.user(r) {
		return map[string]interface{}{"content": []interface{}{map[string]string{"type": "text", "text": "tool not found"}}, "isError": true}
	}
	args := call.Arguments
	path := selected.Path
	for _, key := range selected.PathParams {
		value, ok := args[key]
		if !ok {
			return toolError("missing path parameter: " + key)
		}
		path = substitutePathParam(path, key, url.PathEscape(fmt.Sprint(value)))
	}
	query, _ := args["query"].(map[string]interface{})
	values := url.Values{}
	for key, value := range query {
		values.Set(key, fmt.Sprint(value))
	}
	if len(values) > 0 {
		path += "?" + values.Encode()
	}
	var body io.Reader
	if value, ok := args["body"]; ok && value != nil {
		encoded, err := json.Marshal(value)
		if err != nil {
			return toolError("invalid request body")
		}
		body = bytes.NewReader(encoded)
	}
	request := r.Clone(r.Context())
	request.Method = selected.Method
	request.URL, _ = url.Parse(path)
	request.RequestURI = path
	if body == nil {
		request.Body = http.NoBody
	} else {
		request.Body = io.NopCloser(body)
	}
	request.Header = r.Header.Clone()
	request.Header.Del("Cookie")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request = request.WithContext(reqctx.SetValue(request.Context(), "request_source", "mcp"))
	request.RemoteAddr = r.RemoteAddr
	w := newResponseCapture()
	s.Router.ServeHTTP(w, request)
	textBody := w.body.String()
	return map[string]interface{}{"content": []interface{}{map[string]string{"type": "text", "text": textBody}}, "isError": w.status >= 400}
}

type responseCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newResponseCapture() *responseCapture     { return &responseCapture{header: make(http.Header)} }
func (w *responseCapture) Header() http.Header { return w.header }
func (w *responseCapture) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *responseCapture) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func toolError(message string) interface{} {
	return map[string]interface{}{"content": []interface{}{map[string]string{"type": "text", "text": message}}, "isError": true}
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// ToolInfo is the public name and description of one MCP tool.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PublicTools lists the tools an anonymous caller sees (the same registry that backs tools/list),
// so the MCP help page never keeps a second hand-written list.
func (s *Server) PublicTools() []ToolInfo {
	anonymous := &http.Request{Header: make(http.Header)}
	var out []ToolInfo
	for _, tool := range s.tools(anonymous) {
		name, _ := tool["name"].(string)
		description, _ := tool["description"].(string)
		out = append(out, ToolInfo{Name: name, Description: description})
	}
	return out
}
