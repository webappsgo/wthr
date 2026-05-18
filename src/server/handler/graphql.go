package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
)

// InitGraphQL creates and returns the GraphQL schema
func InitGraphQL() (*handler.Handler, error) {
	// Define GraphQL types
	locationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Location",
		Fields: graphql.Fields{
			"name": &graphql.Field{
				Type: graphql.String,
			},
			"latitude": &graphql.Field{
				Type: graphql.Float,
			},
			"longitude": &graphql.Field{
				Type: graphql.Float,
			},
			"country": &graphql.Field{
				Type: graphql.String,
			},
		},
	})

	weatherType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Weather",
		Fields: graphql.Fields{
			"location": &graphql.Field{
				Type: locationType,
			},
			"temperature": &graphql.Field{
				Type: graphql.Float,
			},
			"humidity": &graphql.Field{
				Type: graphql.Float,
			},
			"wind_speed": &graphql.Field{
				Type: graphql.Float,
			},
			"description": &graphql.Field{
				Type: graphql.String,
			},
		},
	})

	healthType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Health",
		Fields: graphql.Fields{
			"status": &graphql.Field{
				Type: graphql.String,
			},
			"version": &graphql.Field{
				Type: graphql.String,
			},
			"uptime": &graphql.Field{
				Type: graphql.String,
			},
		},
	})

	// Define root query
	rootQuery := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"health": &graphql.Field{
				Type:        healthType,
				Description: "Get service health status",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return map[string]interface{}{
						"status":  "healthy",
						"version": "1.0.0",
						"uptime":  "running",
					}, nil
				},
			},
			"weather": &graphql.Field{
				Type:        weatherType,
				Description: "Get weather forecast for a location",
				Args: graphql.FieldConfigArgument{
					"location": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Location (city, coordinates, ZIP)",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					location, _ := p.Args["location"].(string)
					if location == "" {
						location = "New York, NY"
					}

					// Return mock data (in production, fetch from weather service)
					return map[string]interface{}{
						"location": map[string]interface{}{
							"name":      location,
							"latitude":  40.7128,
							"longitude": -74.0060,
							"country":   "US",
						},
						"temperature": 72.5,
						"humidity":    65.0,
						"wind_speed":  10.5,
						"description": "Clear sky",
					}, nil
				},
			},
		},
	})

	// Create schema
	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query: rootQuery,
	})
	if err != nil {
		return nil, err
	}

	// Create handler with GraphiQL enabled
	h := handler.New(&handler.Config{
		Schema:   &schema,
		Pretty:   true,
		GraphiQL: true,
	})

	return h, nil
}

// GraphQLHandler wraps the GraphQL handler for Gin
func GraphQLHandler(h *handler.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		// For GET requests, return GraphiQL UI
		if c.Request.Method == "GET" {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(200, graphiQLHTML)
			return
		}
		// For POST requests, use the handler
		h.ServeHTTP(c.Writer, c.Request)
	}
}

const graphiQLHTML = `<!DOCTYPE html>
<html>
<head>
    <title>GraphiQL - Weather API</title>
    <link rel="stylesheet" href="https://unpkg.com/graphiql@3.0.0/graphiql.min.css" />
    <style>
        body {
            height: 100%;
            margin: 0;
            width: 100%;
            overflow: hidden;
            background: #282a36;
        }
        #graphiql {
            height: 100vh;
        }
        .graphiql-container {
            --color-primary: 40, 165, 255;
            --color-secondary: 189, 147, 249;
            background: #282a36;
        }
    </style>
</head>
<body>
    <div id="graphiql">Loading...</div>
    <script crossorigin src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
    <script crossorigin src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
    <script src="https://unpkg.com/graphiql@3.0.0/graphiql.min.js"></script>
    <script>
        const fetcher = GraphiQL.createFetcher({ url: '/graphql' });
        const root = ReactDOM.createRoot(document.getElementById('graphiql'));
        root.render(
            React.createElement(GraphiQL, {
                fetcher: fetcher,
                defaultQuery: '# Welcome to GraphQL API for Weather\n# Try this example query:\n\n{\n  health {\n    status\n    version\n    uptime\n  }\n  weather(location: "London") {\n    location {\n      name\n      latitude\n      longitude\n      country\n    }\n    temperature\n    humidity\n    wind_speed\n  }\n}'
            })
        );
    </script>
</body>
</html>`
