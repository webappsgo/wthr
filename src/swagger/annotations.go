package swagger

// AI.md PART 19: Swagger/OpenAPI Annotations
// These annotations are used by swag to auto-generate OpenAPI specification
// Generate with: swag init -g src/main.go --output src/swagger/docs

// @title Weather Service API
// @version 1.0
// @description Professional weather tracking and forecasting service with real-time updates, severe weather alerts, earthquake monitoring, and moon phase tracking.
// @termsOfService https://github.com/casapps/wthr

// @contact.name Weather Service Support
// @contact.url https://github.com/casapps/wthr
// @contact.email support@example.com

// @license.name MIT
// @license.url https://github.com/casapps/wthr/blob/main/LICENSE.md

// @host localhost
// @BasePath /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

// @tag.name Weather
// @tag.description Weather forecast operations

// @tag.name Severe Weather
// @tag.description Severe weather alerts and tracking

// @tag.name Moon
// @tag.description Moon phase information

// @tag.name Earthquakes
// @tag.description Earthquake data and tracking

// @tag.name Hurricanes
// @tag.description Hurricane tracking and forecasts

// @tag.name User
// @tag.description User account management

// @tag.name Auth
// @tag.description Authentication and authorization

// @tag.name Locations
// @tag.description Saved locations management

// @tag.name Notifications
// @tag.description Weather notification settings

// @tag.name Admin
// @tag.description Administrative operations

// @tag.name System
// @tag.description System health and metrics
