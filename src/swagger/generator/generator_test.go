package generator

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestExtractRegisteredAPIRoutes(t *testing.T) {
	root := t.TempDir()
	writeGeneratorSource(t, root, `package main
func setup(r, api, v1, nested Router, dynamic string) {
	r.Mount("/api", api)
	r.Mount("/api", api)
	r.Mount(dynamic, api)
	r.sub.Mount("/api", api)
	unknown.Mount("/api", api)
	r.Mount("/api", getRouter())
	r.Handle("/api/mcp", handler)
	r.Get("/static", handler)
	r.Get()
	ordinary(handler)
	getRouter().Get("/api/not-registered", handler)
	api.Mount("/v1", v1)
	v1.Get("/items/{id:[0-9]+}/", handler)
	v1.Post("/items", handler)
	v1.Put("/items", handler)
	v1.Patch("/items", handler)
	v1.Delete("/items", handler)
	v1.Head("/items", handler)
	v1.Options("/items", handler)
	v1.Get("/items/{id}", handler)
	v1.Handle("/mcp", handler)
	v1.Mount("/nested", nested)
}
func setupNested(nested Router) { nested.Get("/item/{item_id}", handler) }
func registerHealthRoutes(r Router, apiPath string) {
	r.Get(apiPath + "/healthz", handler)
	r.Get(r.GetAPIPath() + "/health", handler)
	r.Use(middleware)
}
func registerMetricsRoutes(r Router, apiPath string) { r.Get(apiPath + "/metrics", handler) }
func registerGraphQLRoutes(r Router, apiPath string) { r.Post(apiPath + "/graphql", handler) }
func registerSwaggerRoutes(r Router, apiPath string) { r.Get(apiPath + "/swagger", handler) }
`)

	routes, err := ExtractRegisteredAPIRoutes(root)
	if err != nil {
		t.Fatalf("ExtractRegisteredAPIRoutes: %v", err)
	}
	want := []Route{
		{Method: "POST", Path: "/api/mcp"},
		{Method: "DELETE", Path: "/api/v1/items"},
		{Method: "HEAD", Path: "/api/v1/items"},
		{Method: "OPTIONS", Path: "/api/v1/items"},
		{Method: "PATCH", Path: "/api/v1/items"},
		{Method: "POST", Path: "/api/v1/items"},
		{Method: "PUT", Path: "/api/v1/items"},
		{Method: "GET", Path: "/api/v1/items/{id}"},
		{Method: "GET", Path: "/api/v1/nested/item/{item_id}"},
		{Method: "POST", Path: "/api/{api_version}/graphql"},
		{Method: "GET", Path: "/api/{api_version}/health"},
		{Method: "GET", Path: "/api/{api_version}/healthz"},
		{Method: "GET", Path: "/api/{api_version}/metrics"},
		{Method: "GET", Path: "/api/{api_version}/swagger"},
	}
	if !reflect.DeepEqual(routes, want) {
		t.Fatalf("routes = %#v, want %#v", routes, want)
	}
}

func TestExtractRegisteredAPIRoutesReportsUnmodeledAndParseErrors(t *testing.T) {
	root := t.TempDir()
	writeGeneratorSource(t, root, `package main
func setup(r, api Router, dynamic string) {
	r.Mount("/api", api)
	api.Get(dynamic, handler)
	api.Route(dynamic, func(child Router) { child.Get("/ok", handler) })
	api.Method("GET", "/items", handler)
	api.HandleFunc("/items", handler)
	api.Handle(dynamic, handler)
}
func registerHealthRoutes(r Router, apiPath string, dynamic string) { r.Get(dynamic, handler) }
`)
	if _, err := ExtractRegisteredAPIRoutes(root); err == nil || !strings.Contains(err.Error(), "unmodeled dynamic router declarations") {
		t.Fatalf("dynamic routes error = %v, want unmodeled router declaration error", err)
	}
	writeGeneratorSource(t, root, `package main
func setup(r Router) { r.Get("/public", handler) }
`)
	if routes, err := ExtractRegisteredAPIRoutes(root); err != nil || len(routes) != 0 {
		t.Fatalf("source without API routes = %#v, %v; want empty routes without error", routes, err)
	}
	writeGeneratorSource(t, root, `package main
func ( {`)
	if _, err := ExtractRegisteredAPIRoutes(root); err == nil || !strings.Contains(err.Error(), "parse router declarations") {
		t.Fatalf("parse error = %v, want wrapped parse error", err)
	}
}

func TestGenerateOpenAPIWritesDeterministicArtifact(t *testing.T) {
	root := t.TempDir()
	writeGeneratorSource(t, root, `package main
func setup(r Router) { r.Post("/api/v1/items/{item_id}", handler) }
`)
	if err := GenerateOpenAPI(root); err != nil {
		t.Fatalf("GenerateOpenAPI: %v", err)
	}
	path := filepath.Join(root, "src", "swagger", "openapi.json")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated OpenAPI: %v", err)
	}
	var document struct {
		OpenAPI string                                       `json:"openapi"`
		Paths   map[string]map[string]map[string]interface{} `json:"paths"`
	}
	if err := json.Unmarshal(first, &document); err != nil {
		t.Fatalf("decode generated OpenAPI: %v", err)
	}
	if document.OpenAPI != "3.0.3" || document.Paths["/api/v1/items/{item_id}"]["post"]["requestBody"] == nil {
		t.Fatalf("generated OpenAPI missing version or request body: %#v", document)
	}
	if err := GenerateOpenAPI(root); err != nil {
		t.Fatalf("second GenerateOpenAPI: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read second generated OpenAPI: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("OpenAPI generation was not deterministic")
	}
	if err := GenerateOpenAPI(t.TempDir()); err == nil {
		t.Fatal("GenerateOpenAPI accepted a root without src/main.go")
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove generated artifact: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("make artifact path a directory: %v", err)
	}
	if err := GenerateOpenAPI(root); err == nil {
		t.Fatal("GenerateOpenAPI ignored an artifact write failure")
	}
}

func TestGeneratorHelpers(t *testing.T) {
	if got, ok := routeReceiverName(astIdent("router")); !ok || got != "router" {
		t.Fatalf("identifier receiver = %q, %t", got, ok)
	}
	if got, ok := routeReceiverName(&ast.SelectorExpr{X: astIdent("router"), Sel: astIdent("Child")}); !ok || got != "router" {
		t.Fatalf("selector receiver = %q, %t", got, ok)
	}
	if got, ok := routeReceiverName(&ast.CallExpr{Fun: &ast.SelectorExpr{X: astIdent("router"), Sel: astIdent("Subrouter")}}); !ok || got != "router" {
		t.Fatalf("call receiver = %q, %t", got, ok)
	}
	if got, ok := routeReceiverName(&ast.CallExpr{Fun: astIdent("unknown")}); ok || got != "" {
		t.Fatalf("unsupported receiver = %q, %t", got, ok)
	}
	if got, ok := routeExpression(astString("/api"), nil); !ok || got != "/api" {
		t.Fatalf("string route expression = %q, %t", got, ok)
	}
	if got, ok := routeExpression(astIdent("apiPath"), map[string]string{"apiPath": "/api/v1"}); !ok || got != "/api/v1" {
		t.Fatalf("environment route expression = %q, %t", got, ok)
	}
	if got, ok := routeExpression(astCall("GetAdminAPIPath"), nil); !ok || got != "/api/{api_version}/server/{admin_path}" {
		t.Fatalf("admin API route expression = %q, %t", got, ok)
	}
	if got, ok := routeExpression(astCall("GetAdminPath"), nil); !ok || got != "{admin_path}" {
		t.Fatalf("admin route expression = %q, %t", got, ok)
	}
	if _, ok := routeExpression(astCall("UnknownPath"), nil); ok {
		t.Fatal("unknown path helper unexpectedly resolved")
	}
	if _, ok := routeExpression(&ast.BasicLit{Kind: token.INT, Value: "12"}, nil); ok {
		t.Fatal("numeric route expression unexpectedly resolved")
	}
	if got, ok := routeExpression(&ast.BinaryExpr{X: astString("/api"), Op: token.ADD, Y: astString("/v1")}, nil); !ok || got != "/api/v1" {
		t.Fatalf("binary route expression = %q, %t", got, ok)
	}
	if _, ok := routeExpression(astIdent("unknown"), nil); ok {
		t.Fatal("unknown route expression unexpectedly resolved")
	}
	if got := joinRoute("/api/", "//v1/"); got != "/api/v1" {
		t.Fatalf("joinRoute = %q", got)
	}
	if got := routePathParams("/api/{id}/items/{item_id}"); !reflect.DeepEqual(got, []string{"id", "item_id"}) {
		t.Fatalf("routePathParams = %#v", got)
	}
	if got := routePathParams("/api/{unfinished"); len(got) != 0 {
		t.Fatalf("routePathParams accepted an unfinished parameter: %#v", got)
	}
	if got := operationID("GET", "/api/v1/weather-alerts/{alert_id}"); !strings.HasPrefix(got, "getWeatherAlertsAlertId_") {
		t.Fatalf("operation ID = %q", got)
	}
}

func TestMarshalOpenAPIRejectsDuplicateOperationIDs(t *testing.T) {
	if _, err := marshalOpenAPI([]Route{{Method: "GET", Path: "/api/v1/items"}, {Method: "GET", Path: "/api/v1/items"}}); err == nil || !strings.Contains(err.Error(), "duplicate OpenAPI operationId") {
		t.Fatalf("duplicate operation ID error = %v", err)
	}
}

func TestMarshalOpenAPIDescribesQueuedVacuumOperation(t *testing.T) {
	data, err := marshalOpenAPI([]Route{{Method: "POST", Path: "/api/{api_version}/server/{admin_path}/database/vacuum"}})
	if err != nil {
		t.Fatalf("marshalOpenAPI: %v", err)
	}
	var document struct {
		Paths map[string]map[string]map[string]interface{} `json:"paths"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	operation := document.Paths["/api/{api_version}/server/{admin_path}/database/vacuum"]["post"]
	if _, exists := operation["requestBody"]; exists {
		t.Fatal("vacuum operation unexpectedly advertises a request body")
	}
	responses, ok := operation["responses"].(map[string]interface{})
	if !ok {
		t.Fatalf("responses = %#v", operation["responses"])
	}
	for _, status := range []string{"202", "401", "500", "501", "503"} {
		if _, exists := responses[status]; !exists {
			t.Errorf("vacuum operation is missing response %s", status)
		}
	}
	for _, status := range []string{"200", "400"} {
		if _, exists := responses[status]; exists {
			t.Errorf("vacuum operation unexpectedly advertises response %s", status)
		}
	}
}

func writeGeneratorSource(t *testing.T, root, source string) {
	t.Helper()
	mainDir := filepath.Join(root, "src")
	if err := os.MkdirAll(mainDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, "main.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mainDir, "swagger"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func astIdent(name string) *ast.Ident { return ast.NewIdent(name) }

func astString(value string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(value)}
}

func astCall(method string) *ast.CallExpr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: astIdent("config"), Sel: astIdent(method)}}
}
