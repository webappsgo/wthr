package graphql

import (
	"database/sql"
	"github.com/webappsgo/wthr/src/server/handler"
	"github.com/webappsgo/wthr/src/server/service"
)

// This file will NOT be regenerated automatically by gqlgen.
// Resolver is the root GraphQL resolver.
// It holds references to all services and handlers needed for resolving queries and mutations.
type Resolver struct {
	// Database connections
	ServerDB *sql.DB
	UsersDB  *sql.DB

	// Services
	WeatherService *service.WeatherService

	// Handlers (we'll use their logic in resolvers)
	APIHandler          *handler.APIHandler
	AuthHandler         *handler.AuthHandler
	LocationHandler     *handler.LocationHandler
	NotificationHandler *handler.NotificationHandler
	AdminHandler        *handler.AdminHandler
	TorAdminHandler     *handler.TorAdminHandler
	SettingsHandler     *handler.AdminSettingsHandler
	SchedulerHandler    *handler.SchedulerHandler
	EarthquakeHandler   *handler.EarthquakeHandler
	HurricaneHandler    *handler.HurricaneHandler
	SevereWeatherHandler *handler.SevereWeatherHandler
	MoonHandler         *handler.MoonHandler
}

// NewResolver creates a new root resolver with all dependencies
func NewResolver(
	serverDB, usersDB *sql.DB,
	weatherService *service.WeatherService,
	apiHandler *handler.APIHandler,
	authHandler *handler.AuthHandler,
	locationHandler *handler.LocationHandler,
	notificationHandler *handler.NotificationHandler,
	adminHandler *handler.AdminHandler,
	torAdminHandler *handler.TorAdminHandler,
	settingsHandler *handler.AdminSettingsHandler,
	schedulerHandler *handler.SchedulerHandler,
	earthquakeHandler *handler.EarthquakeHandler,
	hurricaneHandler *handler.HurricaneHandler,
	severeWeatherHandler *handler.SevereWeatherHandler,
	moonHandler *handler.MoonHandler,
) *Resolver {
	return &Resolver{
		ServerDB:             serverDB,
		UsersDB:              usersDB,
		WeatherService:       weatherService,
		APIHandler:           apiHandler,
		AuthHandler:          authHandler,
		LocationHandler:      locationHandler,
		NotificationHandler:  notificationHandler,
		AdminHandler:         adminHandler,
		TorAdminHandler:      torAdminHandler,
		SettingsHandler:      settingsHandler,
		SchedulerHandler:     schedulerHandler,
		EarthquakeHandler:    earthquakeHandler,
		HurricaneHandler:     hurricaneHandler,
		SevereWeatherHandler: severeWeatherHandler,
		MoonHandler:          moonHandler,
	}
}
