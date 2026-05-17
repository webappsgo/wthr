package service

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/casapps/wthr/src/database"
)

// WeatherNotificationService handles weather-based notifications
type WeatherNotificationService struct {
	db             *sql.DB
	weatherService *WeatherService
	deliverySystem *DeliverySystem
	templateEngine *TemplateEngine
}

// WeatherAlert represents a weather alert
type WeatherAlert struct {
	LocationID   int
	LocationName string
	AlertType    string
	Severity     string
	Message      string
	IssuedAt     time.Time
	ExpiresAt    *time.Time
	Coordinates  struct {
		Latitude  float64
		Longitude float64
	}
}

// NewWeatherNotificationService creates a new weather notification service
func NewWeatherNotificationService(db *sql.DB, ws *WeatherService, ds *DeliverySystem, te *TemplateEngine) *WeatherNotificationService {
	return &WeatherNotificationService{
		db:             db,
		weatherService: ws,
		deliverySystem: ds,
		templateEngine: te,
	}
}

// CheckWeatherAlerts checks all user locations for weather alerts
func (wns *WeatherNotificationService) CheckWeatherAlerts() error {
	// Get all locations with alerts enabled
	rows, err := database.GetUsersDB().Query(`
		SELECT l.id, l.user_id, l.name, l.latitude, l.longitude
		FROM user_saved_locations l
		WHERE l.alerts_enabled = 1
	`)
	if err != nil {
		return fmt.Errorf("failed to query locations: %w", err)
	}
	defer rows.Close()

	alertsFound := 0
	for rows.Next() {
		var locationID, userID int
		var name string
		var lat, lon float64

		if err := rows.Scan(&locationID, &userID, &name, &lat, &lon); err != nil {
			continue
		}

		// Get weather data for this location
		weatherData, err := wns.weatherService.GetCurrentWeather(lat, lon, "imperial")
		if err != nil {
			continue
		}

		// Check for severe weather conditions
		alerts := wns.detectSevereWeather(weatherData, name, lat, lon)
		for _, alert := range alerts {
			alert.LocationID = locationID

			// Check if we've already sent this alert recently (within 6 hours)
			if wns.hasRecentAlert(userID, locationID, alert.AlertType) {
				continue
			}

			// Send notification to user
			if err := wns.sendWeatherAlert(userID, alert); err != nil {
				fmt.Printf("Failed to send weather alert: %v\n", err)
			} else {
				alertsFound++
				// Record that we sent this alert
				wns.recordAlertSent(userID, locationID, alert.AlertType)
			}
		}
	}

	if alertsFound > 0 {
		fmt.Printf("✅ Sent %d weather alerts\n", alertsFound)
	}

	return nil
}

// detectSevereWeather analyzes weather data and returns alerts
func (wns *WeatherNotificationService) detectSevereWeather(weatherData *CurrentWeather, location string, lat, lon float64) []WeatherAlert {
	var alerts []WeatherAlert

	if weatherData == nil {
		return alerts
	}

	// Get current conditions
	temp := weatherData.Temperature
	windSpeed := weatherData.WindSpeed
	weatherCode := float64(weatherData.WeatherCode)
	precipitation := weatherData.Precipitation

	now := time.Now()

	// Extreme temperature alerts
	// >= 100°F / 38°C
	if temp >= 100 {
		alerts = append(alerts, WeatherAlert{
			LocationName: location,
			AlertType:    "Extreme Heat",
			Severity:     "high",
			Message:      fmt.Sprintf("Extreme heat warning: Temperature is %.0f°F. Take precautions to stay cool.", temp),
			IssuedAt:     now,
			Coordinates:  struct{ Latitude, Longitude float64 }{lat, lon},
		})
	// <= 0°F / -18°C
	} else if temp <= 0 {
		alerts = append(alerts, WeatherAlert{
			LocationName: location,
			AlertType:    "Extreme Cold",
			Severity:     "high",
			Message:      fmt.Sprintf("Extreme cold warning: Temperature is %.0f°F. Frostbite danger.", temp),
			IssuedAt:     now,
			Coordinates:  struct{ Latitude, Longitude float64 }{lat, lon},
		})
	}

	// High wind alerts
	// >= 40 mph / 64 km/h
	if windSpeed >= 40 {
		severity := "medium"
		if windSpeed >= 60 {
			severity = "high"
		}
		alerts = append(alerts, WeatherAlert{
			LocationName: location,
			AlertType:    "High Winds",
			Severity:     severity,
			Message:      fmt.Sprintf("High wind warning: Wind speed is %.0f mph. Secure loose objects.", windSpeed),
			IssuedAt:     now,
			Coordinates:  struct{ Latitude, Longitude float64 }{lat, lon},
		})
	}

	// Severe weather based on weather codes
	// 95-99: Thunderstorm
	if weatherCode >= 95 && weatherCode <= 99 {
		alerts = append(alerts, WeatherAlert{
			LocationName: location,
			AlertType:    "Thunderstorm",
			Severity:     "high",
			Message:      "Severe thunderstorm warning: Seek shelter immediately.",
			IssuedAt:     now,
			Coordinates:  struct{ Latitude, Longitude float64 }{lat, lon},
		})
	}

	// 71-77, 85-86: Heavy snow
	if (weatherCode >= 71 && weatherCode <= 77) || (weatherCode >= 85 && weatherCode <= 86) {
		alerts = append(alerts, WeatherAlert{
			LocationName: location,
			AlertType:    "Heavy Snow",
			Severity:     "medium",
			Message:      "Heavy snow warning: Travel may be hazardous.",
			IssuedAt:     now,
			Coordinates:  struct{ Latitude, Longitude float64 }{lat, lon},
		})
	}

	// Heavy precipitation
	// >= 1 inch per hour
	if precipitation >= 1.0 {
		alerts = append(alerts, WeatherAlert{
			LocationName: location,
			AlertType:    "Heavy Rain",
			Severity:     "medium",
			Message:      fmt.Sprintf("Heavy rain warning: %.1f inches per hour. Flooding possible.", precipitation),
			IssuedAt:     now,
			Coordinates:  struct{ Latitude, Longitude float64 }{lat, lon},
		})
	}

	return alerts
}

// sendWeatherAlert sends a weather alert notification to a user
func (wns *WeatherNotificationService) sendWeatherAlert(userID int, alert WeatherAlert) error {
	// Get user's enabled notification preferences
	rows, err := database.GetUsersDB().Query(`
		SELECT unp.channel_type
		FROM user_notification_preferences unp
		JOIN notification_subscriptions ns ON ns.user_id = unp.user_id
		WHERE unp.user_id = ?
		  AND unp.enabled = 1
		  AND ns.subscription_type = 'weather_alerts'
		  AND ns.enabled = 1
		GROUP BY unp.channel_type
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to get user preferences: %w", err)
	}
	defer rows.Close()

	channelsSent := 0
	for rows.Next() {
		var channelType string
		if err := rows.Scan(&channelType); err != nil {
			continue
		}

		// Prepare template variables
		variables := map[string]interface{}{
			"Location":    alert.LocationName,
			"AlertType":   alert.AlertType,
			"Severity":    alert.Severity,
			"Message":     alert.Message,
			"IssuedAt":    alert.IssuedAt.Format("Jan 2, 2006 at 3:04 PM"),
			"Coordinates": fmt.Sprintf("%.4f, %.4f", alert.Coordinates.Latitude, alert.Coordinates.Longitude),
		}

		if alert.ExpiresAt != nil {
			variables["ExpiresAt"] = alert.ExpiresAt.Format("Jan 2, 2006 at 3:04 PM")
		}

		// Render template
		subject, body, err := wns.templateEngine.RenderTemplate(channelType, "weather_alert", variables)
		if err != nil {
			// Fallback to default template
			subject = fmt.Sprintf("⚠️ Weather Alert: %s", alert.AlertType)
			body = alert.Message
		}

		// Priority based on severity
		// normal
		priority := 2
		if alert.Severity == "high" {
			// high
			priority = 3
		} else if alert.Severity == "critical" {
			// critical
			priority = 4
		}

		// Enqueue notification
		_, err = wns.deliverySystem.Enqueue(&userID, channelType, subject, body, priority, variables)
		if err != nil {
			fmt.Printf("Failed to enqueue notification for channel %s: %v\n", channelType, err)
		} else {
			channelsSent++
		}
	}

	if channelsSent > 0 {
		fmt.Printf("📢 Weather alert sent to user %d via %d channels: %s - %s\n",
			userID, channelsSent, alert.LocationName, alert.AlertType)
	}

	return nil
}

// hasRecentAlert checks if we've sent this alert type recently
func (wns *WeatherNotificationService) hasRecentAlert(userID, locationID int, alertType string) bool {
	var count int
	err := database.GetUsersDB().QueryRow(`
		SELECT COUNT(*)
		FROM user_weather_alert_history
		WHERE user_id = ?
		  AND location_id = ?
		  AND alert_type = ?
		  AND sent_at > datetime('now', '-6 hours')
	`, userID, locationID, alertType).Scan(&count)

	return err == nil && count > 0
}

// recordAlertSent records that we sent an alert
func (wns *WeatherNotificationService) recordAlertSent(userID, locationID int, alertType string) {
	_, _ = database.GetUsersDB().Exec(`
		INSERT INTO user_weather_alert_history (user_id, location_id, alert_type, sent_at)
		VALUES (?, ?, ?, datetime('now'))
	`, userID, locationID, alertType)
}

// SendDailyForecast sends daily forecast to subscribed users
func (wns *WeatherNotificationService) SendDailyForecast() error {
	// Get all users subscribed to daily forecast
	rows, err := database.GetUsersDB().Query(`
		SELECT DISTINCT ns.user_id, l.id, l.name, l.latitude, l.longitude
		FROM notification_subscriptions ns
		JOIN user_saved_locations l ON l.user_id = ns.user_id
		WHERE ns.subscription_type = 'daily_forecast'
		  AND ns.enabled = 1
	`)
	if err != nil {
		return fmt.Errorf("failed to query subscriptions: %w", err)
	}
	defer rows.Close()

	forecastsSent := 0
	for rows.Next() {
		var userID, locationID int
		var name string
		var lat, lon float64

		if err := rows.Scan(&userID, &locationID, &name, &lat, &lon); err != nil {
			continue
		}

		// Get weather data
		weatherData, err := wns.weatherService.GetCurrentWeather(lat, lon, "imperial")
		if err != nil {
			continue
		}

		if err := wns.sendDailyForecast(userID, name, weatherData); err != nil {
			fmt.Printf("Failed to send daily forecast: %v\n", err)
		} else {
			forecastsSent++
		}
	}

	if forecastsSent > 0 {
		fmt.Printf("✅ Sent %d daily forecasts\n", forecastsSent)
	}

	return nil
}

// sendDailyForecast sends a daily forecast to a user
func (wns *WeatherNotificationService) sendDailyForecast(userID int, location string, weatherData *CurrentWeather) error {
	// Get user's enabled channels
	rows, err := database.GetUsersDB().Query(`
		SELECT channel_type
		FROM user_notification_preferences
		WHERE user_id = ? AND enabled = 1
	`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	condition := wns.weatherService.GetWeatherDescription(weatherData.WeatherCode)
	emoji := wns.weatherService.GetWeatherIcon(weatherData.WeatherCode, weatherData.IsDay == 1)

	variables := map[string]interface{}{
		"Location":    location,
		"Temperature": fmt.Sprintf("%.0f°F", weatherData.Temperature),
		"Condition":   condition,
		"Emoji":       emoji,
		"Date":        time.Now().Format("Monday, January 2"),
	}

	for rows.Next() {
		var channelType string
		if err := rows.Scan(&channelType); err != nil {
			continue
		}

		subject := fmt.Sprintf("🌤️ Daily Forecast for %s", location)
		body := fmt.Sprintf("%s %s - %.0f°F\n\n%s", emoji, condition, weatherData.Temperature, "Have a great day!")

		_, _ = wns.deliverySystem.Enqueue(&userID, channelType, subject, body, 1, variables)
	}

	return nil
}

// SendSystemHealthAlert sends system health alerts to admins
func (wns *WeatherNotificationService) SendSystemHealthAlert(component, message string, severity string) error {
	// Get all admin users
	rows, err := database.GetUsersDB().Query(`
		SELECT u.id
		FROM user_accounts u
		JOIN notification_subscriptions ns ON ns.user_id = u.id
		WHERE u.role = 'admin'
		  AND ns.subscription_type = 'system_notifications'
		  AND ns.enabled = 1
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err != nil {
			continue
		}

		// Get admin's enabled channels
		channels, err := database.GetUsersDB().Query(`
			SELECT channel_type
			FROM user_notification_preferences
			WHERE user_id = ? AND enabled = 1
		`, userID)
		if err != nil {
			continue
		}

		for channels.Next() {
			var channelType string
			if err := channels.Scan(&channelType); err != nil {
				continue
			}

			variables := map[string]interface{}{
				"Component": component,
				"Message":   message,
				"Priority":  severity,
				"Time":      time.Now().Format("2006-01-02 15:04:05"),
			}

			priority := 2
			if severity == "critical" {
				priority = 4
			} else if severity == "high" {
				priority = 3
			}

			subject := fmt.Sprintf("[%s] System Alert: %s", severity, component)
			body := message

			_, _ = wns.deliverySystem.Enqueue(&userID, channelType, subject, body, priority, variables)
		}
		channels.Close()
	}

	return nil
}
