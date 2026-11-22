package generator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Route identifies one method and path declared by the HTTP router.
type Route struct {
	Method string
	Path   string
}

var routeMethods = map[string]bool{
	"Get": true, "Post": true, "Put": true, "Patch": true, "Delete": true, "Head": true, "Options": true,
}

var apiPathPattern = regexp.MustCompile(`^/api(?:/|$)`)

// ExtractRegisteredAPIRoutes reads the router declarations and returns the
// normalized API route set. The router is assembled in main.go; mounted
// subrouters are resolved through their Mount calls rather than by guessing
// from handler names.
func ExtractRegisteredAPIRoutes(root string) ([]Route, error) {
	filename := filepath.Join(root, "src", "main.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse router declarations: %w", err)
	}

	prefixes := map[string]string{"r": ""}
	for pass := 0; pass < 8; pass++ {
		changed := false
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Mount" {
				return true
			}
			parent, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			child, ok := call.Args[1].(*ast.Ident)
			if !ok {
				return true
			}
			parentPrefix, exists := prefixes[parent.Name]
			if !exists {
				return true
			}
			mount, ok := routeExpression(call.Args[0], nil)
			if !ok {
				return true
			}
			joined := joinRoute(parentPrefix, mount)
			if prefixes[child.Name] != joined {
				prefixes[child.Name] = joined
				changed = true
			}
			return true
		})
		if !changed {
			break
		}
	}

	seen := map[string]bool{}
	var unmodeled []string
	var routes []Route
	add := func(method, path string) {
		method = strings.ToUpper(method)
		path = normalizeAPIRoute(path)
		if path == "" || (!apiPathPattern.MatchString(path) && path != "/mcp") {
			return
		}
		key := method + " " + path
		if !seen[key] {
			seen[key] = true
			routes = append(routes, Route{Method: method, Path: path})
		}
	}

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver, ok := routeReceiverName(selector.X)
		if !ok {
			return true
		}
		prefix, exists := prefixes[receiver]
		if !exists || len(call.Args) == 0 {
			return true
		}
		if routeMethods[selector.Sel.Name] {
			path, ok := routeExpression(call.Args[0], nil)
			if ok {
				add(selector.Sel.Name, joinRoute(prefix, path))
			} else if apiPathPattern.MatchString(prefix) {
				position := fset.Position(call.Pos())
				unmodeled = append(unmodeled, fmt.Sprintf("%s:%d: %s route path expression", filepath.Base(filename), position.Line, selector.Sel.Name))
			}
		}
		if apiPathPattern.MatchString(prefix) && (selector.Sel.Name == "Route" || selector.Sel.Name == "Method" || selector.Sel.Name == "HandleFunc") {
			position := fset.Position(call.Pos())
			unmodeled = append(unmodeled, fmt.Sprintf("%s:%d: unsupported chi.Router.%s registration", filepath.Base(filename), position.Line, selector.Sel.Name))
		}
		if selector.Sel.Name == "Handle" {
			path, ok := routeExpression(call.Args[0], nil)
			if ok {
				full := joinRoute(prefix, path)
				if full == "/mcp" || full == "/api/mcp" || strings.HasSuffix(full, "/server/mcp") {
					add("POST", full)
				}
			} else if apiPathPattern.MatchString(prefix) {
				position := fset.Position(call.Pos())
				unmodeled = append(unmodeled, fmt.Sprintf("%s:%d: Handle route path expression", filepath.Base(filename), position.Line))
			}
		}
		return true
	})

	for _, functionName := range []string{"registerHealthRoutes", "registerMetricsRoutes", "registerGraphQLRoutes", "registerSwaggerRoutes"} {
		declaration := findFunction(file, functionName)
		if declaration == nil || declaration.Body == nil {
			continue
		}
		env := map[string]string{}
		if declaration.Type.Params != nil {
			for _, field := range declaration.Type.Params.List {
				for _, name := range field.Names {
					switch name.Name {
					case "apiPath":
						env[name.Name] = "/api/{api_version}"
					case "r":
						env[name.Name] = ""
					}
				}
			}
		}
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !routeMethods[selector.Sel.Name] {
				return true
			}
			path, ok := routeExpression(call.Args[0], env)
			if ok {
				add(selector.Sel.Name, path)
			} else {
				position := fset.Position(call.Pos())
				unmodeled = append(unmodeled, fmt.Sprintf("%s:%d: %s route path expression in %s", filepath.Base(filename), position.Line, selector.Sel.Name, functionName))
			}
			return true
		})
	}
	if len(unmodeled) > 0 {
		sort.Strings(unmodeled)
		return nil, fmt.Errorf("unmodeled dynamic router declarations: %s", strings.Join(unmodeled, "; "))
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes, nil
}

func routeReceiverName(expression ast.Expr) (string, bool) {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name, true
	case *ast.CallExpr:
		if selector, ok := value.Fun.(*ast.SelectorExpr); ok {
			return routeReceiverName(selector.X)
		}
	case *ast.SelectorExpr:
		return routeReceiverName(value.X)
	}
	return "", false
}

func findFunction(file *ast.File, name string) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == name {
			return function
		}
	}
	return nil
}

func routeExpression(expression ast.Expr, environment map[string]string) (string, bool) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "", false
		}
		decoded, err := strconv.Unquote(value.Value)
		return decoded, err == nil
	case *ast.Ident:
		resolved, ok := environment[value.Name]
		return resolved, ok
	case *ast.CallExpr:
		selector, ok := value.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "GetAPIPath" {
			return "/api/{api_version}", true
		}
		if ok && selector.Sel.Name == "GetAdminAPIPath" {
			return "/api/{api_version}/server/{admin_path}", true
		}
		if ok && selector.Sel.Name == "GetAdminPath" {
			return "{admin_path}", true
		}
	case *ast.BinaryExpr:
		left, okLeft := routeExpression(value.X, environment)
		right, okRight := routeExpression(value.Y, environment)
		if okLeft && okRight {
			return left + right, true
		}
	}
	return "", false
}

func joinRoute(prefix, path string) string {
	return normalizeAPIRoute(strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(path, "/"))
}

func normalizeAPIRoute(path string) string {
	path = regexp.MustCompile(`/+`).ReplaceAllString(path, "/")
	path = regexp.MustCompile(`\{([^}:]+):[^}]+\}`).ReplaceAllString(path, `{$1}`)
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

// GenerateOpenAPI writes the deterministic OpenAPI artifact used by the server.
func GenerateOpenAPI(root string) error {
	routes, err := ExtractRegisteredAPIRoutes(root)
	if err != nil {
		return err
	}
	data, err := marshalOpenAPI(routes)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "src", "swagger", "openapi.json"), data, 0644)
}

func marshalOpenAPI(routes []Route) ([]byte, error) {
	paths := make(map[string]map[string]interface{})
	operationIDs := make(map[string]string, len(routes))
	for _, route := range routes {
		pathItem := paths[route.Path]
		if pathItem == nil {
			pathItem = map[string]interface{}{}
			paths[route.Path] = pathItem
		}
		id := operationID(route.Method, route.Path)
		if previous, exists := operationIDs[id]; exists {
			return nil, fmt.Errorf("duplicate OpenAPI operationId %q for %s and %s", id, previous, route.Method+" "+route.Path)
		}
		operationIDs[id] = route.Method + " " + route.Path
		operation := map[string]interface{}{
			"operationId": id,
			"summary":     route.Method + " " + route.Path,
			"description": "API operation registered by the wthr server.",
			"responses": map[string]interface{}{
				"200": map[string]interface{}{"description": "Successful response", "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": map[string]interface{}{"$ref": "#/components/schemas/APIResponse"}}}},
				"400": map[string]interface{}{"description": "Invalid request", "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": map[string]interface{}{"$ref": "#/components/schemas/APIError"}}}},
				"401": map[string]interface{}{"description": "Authentication required", "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": map[string]interface{}{"$ref": "#/components/schemas/APIError"}}}},
				"500": map[string]interface{}{"description": "Server error", "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": map[string]interface{}{"$ref": "#/components/schemas/APIError"}}}},
			},
			"security": []map[string][]string{{}, {"BearerAuth": {}}},
		}
		parameters := []map[string]interface{}{}
		for _, name := range routePathParams(route.Path) {
			parameters = append(parameters, map[string]interface{}{"name": name, "in": "path", "required": true, "schema": map[string]string{"type": "string"}})
		}
		if len(parameters) > 0 {
			operation["parameters"] = parameters
		}
		if route.Method == "POST" || route.Method == "PUT" || route.Method == "PATCH" {
			operation["requestBody"] = map[string]interface{}{"required": false, "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": map[string]interface{}{"type": "object", "additionalProperties": true}}}}
		}
		pathItem[strings.ToLower(route.Method)] = operation
	}
	document := map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]string{
			"title":       "wthr API",
			"version":     "1.0.0",
			"description": "Weather forecasts, severe weather alerts, earthquakes, tropical storms, astronomy, locations and account operations.",
		},
		"servers": []map[string]string{{"url": "/", "description": "Current application origin"}},
		"paths":   paths,
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{"BearerAuth": map[string]string{"type": "http", "scheme": "bearer"}},
			"schemas": map[string]interface{}{
				"APIResponse": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"ok": map[string]string{"type": "boolean"}, "data": map[string]interface{}{}, "message": map[string]string{"type": "string"}}},
				"APIError":    map[string]interface{}{"type": "object", "required": []string{"ok", "error", "message"}, "properties": map[string]interface{}{"ok": map[string]interface{}{"type": "boolean", "enum": []bool{false}}, "error": map[string]string{"type": "string"}, "message": map[string]string{"type": "string"}, "details": map[string]interface{}{}}},
			},
		},
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func operationID(method, path string) string {
	var words []string
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '{' || r == '}' || r == '-' || r == '_' }) {
		if part == "api" || part == "v1" || part == "v2" {
			continue
		}
		words = append(words, strings.ToUpper(part[:1])+part[1:])
	}
	base := strings.ToLower(method) + strings.Join(words, "")
	digest := sha256.Sum256([]byte(method + " " + path))
	return base + "_" + hex.EncodeToString(digest[:4])
}

func routePathParams(path string) []string {
	var params []string
	for rest := path; ; {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			return params
		}
		rest = rest[start+1:]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return params
		}
		params = append(params, rest[:end])
		rest = rest[end+1:]
	}
}
