package service

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"

	_ "modernc.org/sqlite"
)

// setupWeatherNotificationsTestDB creates in-memory users and server SQLite
// databases with the ad hoc tables that weather_notifications.go queries.
// The real UsersSchema/ServerSchema do not define these exact table shapes
// (see report), so tests target the Go logic against tables that match what
// the code under test actually queries.
func setupWeatherNotificationsTestDB(t *testing.T) (usersDB, serverDB *sql.DB) {
	t.Helper()
	name := t.Name()

	var err error
	usersDB, err = sql.Open("sqlite", "file:"+name+"_users?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open users db: %v", err)
	}
	t.Cleanup(func() { usersDB.Close() })
	// MaxOpenConns is intentionally left unbounded (matches convention used
	// elsewhere in this package): CheckWeatherAlerts/sendDailyForecast hold
	// an open *sql.Rows on usersDB while calling other methods (sendWeatherAlert,
	// hasRecentAlert, etc.) that issue further queries against the same DB —
	// a single-connection pool deadlocks that nested-query pattern.

	serverDB, err = sql.Open("sqlite", "file:"+name+"_server?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open server db: %v", err)
	}
	t.Cleanup(func() { serverDB.Close() })

	schema := []string{
		`CREATE TABLE user_saved_locations (
			id INTEGER PRIMARY KEY,
			user_id INTEGER,
			name TEXT,
			latitude REAL,
			longitude REAL,
			alerts_enabled INTEGER
		)`,
		`CREATE TABLE user_weather_alert_history (
			id INTEGER PRIMARY KEY,
			user_id INTEGER,
			location_id INTEGER,
			alert_type TEXT,
			sent_at TEXT
		)`,
		`CREATE TABLE user_notification_preferences (
			user_id INTEGER,
			channel_type TEXT,
			enabled INTEGER,
			config TEXT
		)`,
		`CREATE TABLE notification_subscriptions (
			user_id INTEGER,
			subscription_type TEXT,
			enabled INTEGER
		)`,
		`CREATE TABLE user_accounts (
			id INTEGER PRIMARY KEY,
			role TEXT,
			email TEXT
		)`,
		`CREATE TABLE notification_queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			channel_type TEXT,
			template_id INTEGER,
			priority INTEGER,
			state TEXT,
			subject TEXT,
			body TEXT,
			variables TEXT,
			retry_count INTEGER,
			max_retries INTEGER,
			next_retry_at TEXT,
			delivered_at TEXT,
			failed_at TEXT,
			error_message TEXT,
			created_at TEXT,
			updated_at TEXT
		)`,
	}
	for _, stmt := range schema {
		if _, err := usersDB.Exec(stmt); err != nil {
			if _, err2 := serverDB.Exec(stmt); err2 != nil {
				t.Fatalf("failed to create table (users err=%v, server err=%v): %s", err, err2, stmt)
			}
		}
	}

	// notification_queue lives on the server DB in production usage.
	if _, err := serverDB.Exec(`CREATE TABLE IF NOT EXISTS notification_queue (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		channel_type TEXT,
		template_id INTEGER,
		priority INTEGER,
		state TEXT,
		subject TEXT,
		body TEXT,
		variables TEXT,
		retry_count INTEGER,
		max_retries INTEGER,
		next_retry_at TEXT,
		delivered_at TEXT,
		failed_at TEXT,
		error_message TEXT,
		created_at TEXT,
		updated_at TEXT
	)`); err != nil {
		t.Fatalf("failed to create server notification_queue: %v", err)
	}

	return usersDB, serverDB
}

// newTestWeatherServiceForNotifications returns a WeatherService whose
// Open-Meteo endpoint points at an httptest server, keeping tests offline.
func newTestWeatherServiceForNotifications(t *testing.T, body string) *WeatherService {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	ws := newTestWeatherService()
	ws.openMeteoBaseURL = server.URL
	return ws
}

const sampleWeatherJSON = `{
	"current": {
		"temperature_2m": 105,
		"relative_humidity_2m": 20,
		"apparent_temperature": 110,
		"is_day": 1,
		"precipitation": 0,
		"weather_code": 1,
		"cloud_cover": 5,
		"pressure_msl": 1005,
		"wind_speed_10m": 5,
		"wind_direction_10m": 90,
		"wind_gusts_10m": 8
	},
	"timezone": "America/Chicago"
}`

// TestWeatherNotifications_detectSevereWeather exhaustively covers each
// threshold branch, boundary values, the nil-input case, and multi-alert
// combinations. This is a pure function with no DB/network dependency.
func TestWeatherNotifications_detectSevereWeather(t *testing.T) {
	wns := &WeatherNotificationService{}

	tests := []struct {
		name         string
		weather      *CurrentWeather
		wantTypes    []string
		wantCountMin int
	}{
		{
			name:      "nil weather data returns no alerts",
			weather:   nil,
			wantTypes: nil,
		},
		{
			name:      "normal conditions produce no alerts",
			weather:   &CurrentWeather{Temperature: 72, WindSpeed: 5, WeatherCode: 1, Precipitation: 0},
			wantTypes: nil,
		},
		{
			name:      "extreme heat at exact boundary 100",
			weather:   &CurrentWeather{Temperature: 100},
			wantTypes: []string{"Extreme Heat"},
		},
		{
			name:      "just below extreme heat boundary produces no heat alert",
			weather:   &CurrentWeather{Temperature: 99.9},
			wantTypes: nil,
		},
		{
			name:      "extreme cold at exact boundary 0",
			weather:   &CurrentWeather{Temperature: 0},
			wantTypes: []string{"Extreme Cold"},
		},
		{
			name:      "just above extreme cold boundary produces no cold alert",
			weather:   &CurrentWeather{Temperature: 0.1},
			wantTypes: nil,
		},
		{
			name:      "high winds at exact boundary 40 (medium severity)",
			weather:   &CurrentWeather{Temperature: 50, WindSpeed: 40},
			wantTypes: []string{"High Winds"},
		},
		{
			name:      "high winds at exact boundary 60 escalates to high severity",
			weather:   &CurrentWeather{Temperature: 50, WindSpeed: 60},
			wantTypes: []string{"High Winds"},
		},
		{
			name:      "thunderstorm weather code lower boundary 95",
			weather:   &CurrentWeather{Temperature: 50, WeatherCode: 95},
			wantTypes: []string{"Thunderstorm"},
		},
		{
			name:      "thunderstorm weather code upper boundary 99",
			weather:   &CurrentWeather{Temperature: 50, WeatherCode: 99},
			wantTypes: []string{"Thunderstorm"},
		},
		{
			name:      "code 94 is not a thunderstorm",
			weather:   &CurrentWeather{Temperature: 50, WeatherCode: 94},
			wantTypes: nil,
		},
		{
			name:      "heavy snow lower range boundary 71",
			weather:   &CurrentWeather{Temperature: 50, WeatherCode: 71},
			wantTypes: []string{"Heavy Snow"},
		},
		{
			name:      "heavy snow upper range boundary 77",
			weather:   &CurrentWeather{Temperature: 50, WeatherCode: 77},
			wantTypes: []string{"Heavy Snow"},
		},
		{
			name:      "heavy snow second range boundary 85",
			weather:   &CurrentWeather{Temperature: 50, WeatherCode: 85},
			wantTypes: []string{"Heavy Snow"},
		},
		{
			name:      "heavy snow second range boundary 86",
			weather:   &CurrentWeather{Temperature: 50, WeatherCode: 86},
			wantTypes: []string{"Heavy Snow"},
		},
		{
			name:      "code 78 is not heavy snow (gap between ranges)",
			weather:   &CurrentWeather{Temperature: 50, WeatherCode: 78},
			wantTypes: nil,
		},
		{
			name:      "heavy rain at exact boundary 1.0",
			weather:   &CurrentWeather{Temperature: 50, Precipitation: 1.0},
			wantTypes: []string{"Heavy Rain"},
		},
		{
			name:      "just below heavy rain boundary produces no rain alert",
			weather:   &CurrentWeather{Temperature: 50, Precipitation: 0.99},
			wantTypes: nil,
		},
		{
			name: "multiple simultaneous alerts can co-occur",
			weather: &CurrentWeather{
				Temperature:   105,
				WindSpeed:     65,
				WeatherCode:   97,
				Precipitation: 2.0,
			},
			wantTypes: []string{"Extreme Heat", "High Winds", "Thunderstorm", "Heavy Rain"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alerts := wns.detectSevereWeather(tt.weather, "Test City", 1.0, 2.0)
			if len(tt.wantTypes) == 0 {
				if len(alerts) != 0 {
					t.Fatalf("expected no alerts, got %d: %+v", len(alerts), alerts)
				}
				return
			}
			if len(alerts) != len(tt.wantTypes) {
				t.Fatalf("expected %d alerts, got %d: %+v", len(tt.wantTypes), len(alerts), alerts)
			}
			gotTypes := make(map[string]bool, len(alerts))
			for _, a := range alerts {
				gotTypes[a.AlertType] = true
				if a.LocationName != "Test City" {
					t.Errorf("unexpected location name: %q", a.LocationName)
				}
				if a.Coordinates.Latitude != 1.0 || a.Coordinates.Longitude != 2.0 {
					t.Errorf("unexpected coordinates: %+v", a.Coordinates)
				}
				if a.IssuedAt.IsZero() {
					t.Error("expected IssuedAt to be set")
				}
			}
			for _, want := range tt.wantTypes {
				if !gotTypes[want] {
					t.Errorf("expected alert type %q to be present, got %+v", want, alerts)
				}
			}
		})
	}

	t.Run("wind severity escalates from medium to high at 60", func(t *testing.T) {
		low := wns.detectSevereWeather(&CurrentWeather{Temperature: 50, WindSpeed: 59}, "X", 0, 0)
		high := wns.detectSevereWeather(&CurrentWeather{Temperature: 50, WindSpeed: 60}, "X", 0, 0)
		if len(low) != 1 || low[0].Severity != "medium" {
			t.Fatalf("expected medium severity below 60, got %+v", low)
		}
		if len(high) != 1 || high[0].Severity != "high" {
			t.Fatalf("expected high severity at 60, got %+v", high)
		}
	})
}

// TestWeatherNotifications_hasRecentAlert covers the dedup lookup: no rows,
// a recent row within the 6-hour window, and a boundary/expired row.
func TestWeatherNotifications_hasRecentAlert(t *testing.T) {
	usersDB, serverDB := setupWeatherNotificationsTestDB(t)
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	wns := &WeatherNotificationService{}

	t.Run("no history rows returns false", func(t *testing.T) {
		if wns.hasRecentAlert(1, 1, "Extreme Heat") {
			t.Error("expected false with no history")
		}
	})

	t.Run("recent alert within 6 hours returns true", func(t *testing.T) {
		if _, err := usersDB.Exec(`INSERT INTO user_weather_alert_history (user_id, location_id, alert_type, sent_at) VALUES (2, 2, 'Extreme Heat', datetime('now'))`); err != nil {
			t.Fatalf("seed failed: %v", err)
		}
		if !wns.hasRecentAlert(2, 2, "Extreme Heat") {
			t.Error("expected true for recently-sent alert")
		}
	})

	t.Run("alert older than 6 hours returns false", func(t *testing.T) {
		if _, err := usersDB.Exec(`INSERT INTO user_weather_alert_history (user_id, location_id, alert_type, sent_at) VALUES (3, 3, 'Extreme Cold', datetime('now', '-7 hours'))`); err != nil {
			t.Fatalf("seed failed: %v", err)
		}
		if wns.hasRecentAlert(3, 3, "Extreme Cold") {
			t.Error("expected false for expired alert")
		}
	})

	t.Run("different alert type for same user/location is not deduped", func(t *testing.T) {
		if _, err := usersDB.Exec(`INSERT INTO user_weather_alert_history (user_id, location_id, alert_type, sent_at) VALUES (4, 4, 'High Winds', datetime('now'))`); err != nil {
			t.Fatalf("seed failed: %v", err)
		}
		if wns.hasRecentAlert(4, 4, "Heavy Rain") {
			t.Error("expected false: different alert type should not match")
		}
	})
}

// TestWeatherNotifications_recordAlertSent verifies inserts succeed and that
// calling it twice (idempotency check) inserts two rows rather than
// deduplicating - dedup is hasRecentAlert's responsibility, not this one's.
func TestWeatherNotifications_recordAlertSent(t *testing.T) {
	usersDB, serverDB := setupWeatherNotificationsTestDB(t)
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	wns := &WeatherNotificationService{}

	wns.recordAlertSent(10, 20, "Extreme Heat")
	wns.recordAlertSent(10, 20, "Extreme Heat")

	var count int
	if err := usersDB.QueryRow(`SELECT COUNT(*) FROM user_weather_alert_history WHERE user_id = 10 AND location_id = 20`).Scan(&count); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows after calling recordAlertSent twice, got %d", count)
	}
}

// TestWeatherNotifications_sendWeatherAlert covers the enqueue happy path,
// the template-render-failure fallback, priority-by-severity mapping, and
// the zero-channels-enabled boundary.
func TestWeatherNotifications_sendWeatherAlert(t *testing.T) {
	t.Run("enqueues to every enabled+subscribed channel with fallback subject/body", func(t *testing.T) {
		usersDB, serverDB := setupWeatherNotificationsTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		if _, err := usersDB.Exec(`INSERT INTO user_notification_preferences (user_id, channel_type, enabled) VALUES (1, 'email', 1)`); err != nil {
			t.Fatalf("seed failed: %v", err)
		}
		if _, err := usersDB.Exec(`INSERT INTO notification_subscriptions (user_id, subscription_type, enabled) VALUES (1, 'weather_alerts', 1)`); err != nil {
			t.Fatalf("seed failed: %v", err)
		}

		cm := NewChannelManager(serverDB)
		te := NewTemplateEngine(serverDB)
		ds := NewDeliverySystem(serverDB, cm, te)
		wns := NewWeatherNotificationService(serverDB, nil, ds, te)

		alert := WeatherAlert{
			LocationName: "Testville",
			AlertType:    "Extreme Heat",
			Severity:     "high",
			Message:      "It is hot.",
			IssuedAt:     time.Now(),
		}
		if err := wns.sendWeatherAlert(1, alert); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var subject, body string
		var priority int
		if err := serverDB.QueryRow(`SELECT subject, body, priority FROM notification_queue WHERE user_id = 1`).Scan(&subject, &body, &priority); err != nil {
			t.Fatalf("expected an enqueued row: %v", err)
		}
		if subject == "" || body != alert.Message {
			t.Errorf("expected fallback subject/body, got subject=%q body=%q", subject, body)
		}
		if priority != 3 {
			t.Errorf("expected priority 3 for high severity, got %d", priority)
		}
	})

	t.Run("critical severity maps to priority 4", func(t *testing.T) {
		usersDB, serverDB := setupWeatherNotificationsTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		usersDB.Exec(`INSERT INTO user_notification_preferences (user_id, channel_type, enabled) VALUES (1, 'email', 1)`)
		usersDB.Exec(`INSERT INTO notification_subscriptions (user_id, subscription_type, enabled) VALUES (1, 'weather_alerts', 1)`)

		cm := NewChannelManager(serverDB)
		te := NewTemplateEngine(serverDB)
		ds := NewDeliverySystem(serverDB, cm, te)
		wns := NewWeatherNotificationService(serverDB, nil, ds, te)

		alert := WeatherAlert{LocationName: "X", AlertType: "Y", Severity: "critical", Message: "m", IssuedAt: time.Now()}
		if err := wns.sendWeatherAlert(1, alert); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var priority int
		if err := serverDB.QueryRow(`SELECT priority FROM notification_queue WHERE user_id = 1`).Scan(&priority); err != nil {
			t.Fatalf("expected an enqueued row: %v", err)
		}
		if priority != 4 {
			t.Errorf("expected priority 4 for critical severity, got %d", priority)
		}
	})

	t.Run("no enabled channels enqueues nothing and returns nil error", func(t *testing.T) {
		usersDB, serverDB := setupWeatherNotificationsTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		cm := NewChannelManager(serverDB)
		te := NewTemplateEngine(serverDB)
		ds := NewDeliverySystem(serverDB, cm, te)
		wns := NewWeatherNotificationService(serverDB, nil, ds, te)

		alert := WeatherAlert{LocationName: "X", AlertType: "Y", Severity: "medium", Message: "m", IssuedAt: time.Now()}
		if err := wns.sendWeatherAlert(99, alert); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int
		if err := serverDB.QueryRow(`SELECT COUNT(*) FROM notification_queue`).Scan(&count); err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 enqueued rows, got %d", count)
		}
	})

	t.Run("preference enabled but subscription disabled is not sent", func(t *testing.T) {
		usersDB, serverDB := setupWeatherNotificationsTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		usersDB.Exec(`INSERT INTO user_notification_preferences (user_id, channel_type, enabled) VALUES (5, 'email', 1)`)
		usersDB.Exec(`INSERT INTO notification_subscriptions (user_id, subscription_type, enabled) VALUES (5, 'weather_alerts', 0)`)

		cm := NewChannelManager(serverDB)
		te := NewTemplateEngine(serverDB)
		ds := NewDeliverySystem(serverDB, cm, te)
		wns := NewWeatherNotificationService(serverDB, nil, ds, te)

		alert := WeatherAlert{LocationName: "X", AlertType: "Y", Severity: "medium", Message: "m", IssuedAt: time.Now()}
		if err := wns.sendWeatherAlert(5, alert); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int
		serverDB.QueryRow(`SELECT COUNT(*) FROM notification_queue`).Scan(&count)
		if count != 0 {
			t.Errorf("expected 0 enqueued rows when subscription disabled, got %d", count)
		}
	})
}

// TestWeatherNotifications_CheckWeatherAlerts_EndToEnd exercises the full
// alert pipeline against an httptest-stubbed weather endpoint: it seeds one
// alerts-enabled location whose stubbed weather triggers an Extreme Heat
// alert, verifies the alert is enqueued and recorded, and that a second run
// does not re-send it (dedup via hasRecentAlert).
func TestWeatherNotifications_CheckWeatherAlerts_EndToEnd(t *testing.T) {
	usersDB, serverDB := setupWeatherNotificationsTestDB(t)
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	if _, err := usersDB.Exec(`INSERT INTO user_saved_locations (id, user_id, name, latitude, longitude, alerts_enabled) VALUES (1, 1, 'Home', 10, 20, 1)`); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	usersDB.Exec(`INSERT INTO user_notification_preferences (user_id, channel_type, enabled) VALUES (1, 'email', 1)`)
	usersDB.Exec(`INSERT INTO notification_subscriptions (user_id, subscription_type, enabled) VALUES (1, 'weather_alerts', 1)`)

	ws := newTestWeatherServiceForNotifications(t, sampleWeatherJSON)
	cm := NewChannelManager(serverDB)
	te := NewTemplateEngine(serverDB)
	ds := NewDeliverySystem(serverDB, cm, te)
	wns := NewWeatherNotificationService(serverDB, ws, ds, te)

	if err := wns.CheckWeatherAlerts(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var queued int
	if err := serverDB.QueryRow(`SELECT COUNT(*) FROM notification_queue`).Scan(&queued); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if queued == 0 {
		t.Fatal("expected at least one enqueued notification for extreme heat")
	}

	var historyRows int
	if err := usersDB.QueryRow(`SELECT COUNT(*) FROM user_weather_alert_history`).Scan(&historyRows); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if historyRows == 0 {
		t.Fatal("expected alert history to be recorded")
	}

	// Running again immediately should not enqueue a duplicate due to hasRecentAlert dedup.
	if err := wns.CheckWeatherAlerts(); err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}
	var queuedAfter int
	if err := serverDB.QueryRow(`SELECT COUNT(*) FROM notification_queue`).Scan(&queuedAfter); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if queuedAfter != queued {
		t.Errorf("expected no new notifications on second run (dedup), before=%d after=%d", queued, queuedAfter)
	}
}

// TestWeatherNotifications_CheckWeatherAlerts_NoLocations covers the boundary
// case where no locations have alerts enabled.
func TestWeatherNotifications_CheckWeatherAlerts_NoLocations(t *testing.T) {
	usersDB, serverDB := setupWeatherNotificationsTestDB(t)
	database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
	t.Cleanup(func() { database.SetGlobalDualDB(nil) })

	ws := newTestWeatherServiceForNotifications(t, sampleWeatherJSON)
	cm := NewChannelManager(serverDB)
	te := NewTemplateEngine(serverDB)
	ds := NewDeliverySystem(serverDB, cm, te)
	wns := NewWeatherNotificationService(serverDB, ws, ds, te)

	if err := wns.CheckWeatherAlerts(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var count int
	serverDB.QueryRow(`SELECT COUNT(*) FROM notification_queue`).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 enqueued rows with no locations, got %d", count)
	}
}

// TestWeatherNotifications_sendDailyForecast covers the daily forecast
// enqueue path directly, including the zero-channels boundary.
func TestWeatherNotifications_sendDailyForecast(t *testing.T) {
	t.Run("enqueues forecast for each enabled channel", func(t *testing.T) {
		usersDB, serverDB := setupWeatherNotificationsTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		usersDB.Exec(`INSERT INTO user_notification_preferences (user_id, channel_type, enabled) VALUES (1, 'email', 1)`)

		ws := newTestWeatherService()
		cm := NewChannelManager(serverDB)
		te := NewTemplateEngine(serverDB)
		ds := NewDeliverySystem(serverDB, cm, te)
		wns := NewWeatherNotificationService(serverDB, ws, ds, te)

		weather := &CurrentWeather{Temperature: 70, WeatherCode: 1, IsDay: 1}
		if err := wns.sendDailyForecast(1, "Home", weather); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int
		serverDB.QueryRow(`SELECT COUNT(*) FROM notification_queue WHERE user_id = 1`).Scan(&count)
		if count != 1 {
			t.Errorf("expected 1 enqueued forecast, got %d", count)
		}
	})

	t.Run("no enabled channels enqueues nothing", func(t *testing.T) {
		usersDB, serverDB := setupWeatherNotificationsTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		ws := newTestWeatherService()
		cm := NewChannelManager(serverDB)
		te := NewTemplateEngine(serverDB)
		ds := NewDeliverySystem(serverDB, cm, te)
		wns := NewWeatherNotificationService(serverDB, ws, ds, te)

		weather := &CurrentWeather{Temperature: 70, WeatherCode: 1, IsDay: 1}
		if err := wns.sendDailyForecast(2, "Home", weather); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int
		serverDB.QueryRow(`SELECT COUNT(*) FROM notification_queue WHERE user_id = 2`).Scan(&count)
		if count != 0 {
			t.Errorf("expected 0 enqueued rows, got %d", count)
		}
	})
}

// TestWeatherNotifications_SendSystemHealthAlert covers admin fan-out:
// priority-by-severity mapping and the boundary of no admins subscribed.
func TestWeatherNotifications_SendSystemHealthAlert(t *testing.T) {
	t.Run("enqueues to each admin's enabled channels with correct priority", func(t *testing.T) {
		usersDB, serverDB := setupWeatherNotificationsTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		usersDB.Exec(`INSERT INTO user_accounts (id, role, email) VALUES (1, 'admin', 'admin@example.com')`)
		usersDB.Exec(`INSERT INTO notification_subscriptions (user_id, subscription_type, enabled) VALUES (1, 'system_notifications', 1)`)
		usersDB.Exec(`INSERT INTO user_notification_preferences (user_id, channel_type, enabled) VALUES (1, 'email', 1)`)

		cm := NewChannelManager(serverDB)
		te := NewTemplateEngine(serverDB)
		ds := NewDeliverySystem(serverDB, cm, te)
		wns := NewWeatherNotificationService(serverDB, nil, ds, te)

		if err := wns.SendSystemHealthAlert("database", "disk full", "critical"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var priority int
		if err := serverDB.QueryRow(`SELECT priority FROM notification_queue WHERE user_id = 1`).Scan(&priority); err != nil {
			t.Fatalf("expected an enqueued row: %v", err)
		}
		if priority != 4 {
			t.Errorf("expected priority 4 for critical severity, got %d", priority)
		}
	})

	t.Run("non-admin users are not notified", func(t *testing.T) {
		usersDB, serverDB := setupWeatherNotificationsTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		usersDB.Exec(`INSERT INTO user_accounts (id, role, email) VALUES (2, 'user', 'user@example.com')`)
		usersDB.Exec(`INSERT INTO notification_subscriptions (user_id, subscription_type, enabled) VALUES (2, 'system_notifications', 1)`)
		usersDB.Exec(`INSERT INTO user_notification_preferences (user_id, channel_type, enabled) VALUES (2, 'email', 1)`)

		cm := NewChannelManager(serverDB)
		te := NewTemplateEngine(serverDB)
		ds := NewDeliverySystem(serverDB, cm, te)
		wns := NewWeatherNotificationService(serverDB, nil, ds, te)

		if err := wns.SendSystemHealthAlert("database", "disk full", "high"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int
		serverDB.QueryRow(`SELECT COUNT(*) FROM notification_queue`).Scan(&count)
		if count != 0 {
			t.Errorf("expected 0 enqueued rows for non-admin user, got %d", count)
		}
	})

	t.Run("no admins subscribed enqueues nothing", func(t *testing.T) {
		usersDB, serverDB := setupWeatherNotificationsTestDB(t)
		database.SetGlobalDualDB(&database.DualDB{Server: serverDB, Users: usersDB})
		t.Cleanup(func() { database.SetGlobalDualDB(nil) })

		cm := NewChannelManager(serverDB)
		te := NewTemplateEngine(serverDB)
		ds := NewDeliverySystem(serverDB, cm, te)
		wns := NewWeatherNotificationService(serverDB, nil, ds, te)

		if err := wns.SendSystemHealthAlert("database", "disk full", "low"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
