package graphql

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
)

func TestGraphQLSchemaRootFieldsExecuteThroughGeneratedExecutor(t *testing.T) {
	schema := NewExecutableSchema(Config{}).Schema()
	resolver := &coverageResolver{
		Resolver: &Resolver{},
		query:    coverageQueryResolver{},
		mutation: coverageMutationResolver{},
	}
	handler := GraphQLHandler(NewServer(resolver))

	for _, operation := range []struct {
		name string
		root *ast.Definition
	}{
		{name: "query", root: schema.Query},
		{name: "mutation", root: schema.Mutation},
	} {
		if operation.root == nil {
			continue
		}
		for _, field := range operation.root.Fields {
			field := field
			t.Run(operation.name+"/"+field.Name, func(t *testing.T) {
				args, ok := graphQLCoverageArguments(schema, field.Arguments)
				if !ok {
					t.Skip("required argument has no safe GraphQL literal")
				}
				selection, ok := graphQLCoverageSelection(schema, field.Type, 0, map[string]bool{})
				if !ok {
					t.Skip("root field has no selectable output type")
				}
				query := operation.name + " { " + field.Name + args + selection + " }"
				body, err := json.Marshal(map[string]string{"query": query})
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(string(body)))
				req.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, req)
				if response.Code != http.StatusOK {
					t.Fatalf("GraphQL status = %d, body=%s", response.Code, response.Body.String())
				}
				var result struct {
					Data map[string]json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatalf("decode GraphQL response: %v; body=%s", err, response.Body.String())
				}
				if _, ok := result.Data[field.Name]; !ok {
					t.Fatalf("GraphQL response omitted %q for query %s: %s", field.Name, query, response.Body.String())
				}
			})
		}
	}
}

func graphQLCoverageArguments(schema *ast.Schema, args ast.ArgumentDefinitionList) (string, bool) {
	values := make([]string, 0, len(args))
	for _, arg := range args {
		literal, ok := graphQLCoverageInputLiteral(schema, arg.Type, 0)
		if !ok {
			if arg.Type.NonNull && arg.DefaultValue == nil {
				return "", false
			}
			continue
		}
		values = append(values, arg.Name+":"+literal)
	}
	if len(values) == 0 {
		return "", true
	}
	return "(" + strings.Join(values, ",") + ")", true
}

func graphQLCoverageInputLiteral(schema *ast.Schema, typ *ast.Type, depth int) (string, bool) {
	if depth > 5 || typ == nil {
		return "", false
	}
	if typ.Elem != nil {
		return "[]", true
	}
	switch typ.NamedType {
	case "String", "ID":
		return strconv.Quote("value"), true
	case "Int":
		return "1", true
	case "Float":
		return "1.5", true
	case "Boolean":
		return "true", true
	case "Time":
		return strconv.Quote("2026-01-01T00:00:00Z"), true
	case "Any":
		return "{}", true
	case "Upload":
		return "", false
	}
	definition := schema.Types[typ.NamedType]
	if definition == nil {
		return "", false
	}
	switch definition.Kind {
	case ast.Enum:
		if len(definition.EnumValues) == 0 {
			return "", false
		}
		return definition.EnumValues[0].Name, true
	case ast.InputObject:
		values := make([]string, 0, len(definition.Fields))
		for _, field := range definition.Fields {
			if !field.Type.NonNull || field.DefaultValue != nil {
				continue
			}
			value, ok := graphQLCoverageInputLiteral(schema, field.Type, depth+1)
			if !ok {
				return "", false
			}
			values = append(values, field.Name+":"+value)
		}
		return "{" + strings.Join(values, ",") + "}", true
	default:
		return "", false
	}
}

func graphQLCoverageSelection(schema *ast.Schema, typ *ast.Type, depth int, seen map[string]bool) (string, bool) {
	if typ == nil || typ.Elem != nil {
		if typ == nil {
			return "", false
		}
		return graphQLCoverageSelection(schema, typ.Elem, depth, seen)
	}
	definition := schema.Types[typ.NamedType]
	if definition == nil || (definition.Kind != ast.Object && definition.Kind != ast.Interface) {
		return "", true
	}
	if depth >= 3 || seen[definition.Name] {
		return " { __typename }", true
	}
	seen[definition.Name] = true
	defer delete(seen, definition.Name)
	fields := []string{"__typename"}
	for _, field := range definition.Fields {
		args, ok := graphQLCoverageArguments(schema, field.Arguments)
		if !ok {
			continue
		}
		child, ok := graphQLCoverageSelection(schema, field.Type, depth+1, seen)
		if !ok {
			continue
		}
		fields = append(fields, field.Name+args+child)
	}
	return " { " + strings.Join(fields, " ") + " }", true
}
