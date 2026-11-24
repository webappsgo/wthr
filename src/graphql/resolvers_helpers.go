package graphql

// This file holds resolver helpers + per-type resolver methods that gqlgen
// previously stored at the bottom of schema.resolvers.go (and would otherwise
// shove into a /* ... */ "preserve" block on every regen). Keeping them in a
// separately-named file keeps gqlgen out of them — the regen logic only
// touches files named after schema sources (e.g. schema.resolvers.go).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/scheduler"
	"github.com/webappsgo/wthr/src/server/handler"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

func (r *aPITokenResolver) Token(ctx context.Context, obj *models.APIToken) (*string, error) {
	if obj.Token == "" {
		return nil, nil
	}
	return &obj.Token, nil
}
func (r *aPITokenResolver) Name(ctx context.Context, obj *models.APIToken) (*string, error) {
	if obj.Name == "" {
		return nil, nil
	}
	return &obj.Name, nil
}
func (r *notificationResolver) ReadAt(ctx context.Context, obj *models.Notification) (*time.Time, error) {
	return obj.ReadAt, nil
}
func mapGraphQLEarthquakes(items []service.Earthquake) []*Earthquake {
	earthquakes := make([]*Earthquake, 0, len(items))
	for _, item := range items {
		earthquakes = append(earthquakes, mapGraphQLEarthquake(item))
	}
	return earthquakes
}
func mapGraphQLEarthquake(item service.Earthquake) *Earthquake {
	var depth *float64
	if item.Depth != 0 {
		depth = &item.Depth
	}

	var tsunami *bool
	if item.Tsunami == 0 || item.Tsunami == 1 {
		value := item.Tsunami == 1
		tsunami = &value
	}

	var url *string
	if item.URL != "" {
		url = &item.URL
	}

	return &Earthquake{
		ID:        item.ID,
		Magnitude: item.Magnitude,
		Location:  item.Place,
		Depth:     depth,
		Time:      item.Time,
		Lat:       item.Latitude,
		Lon:       item.Longitude,
		Tsunami:   tsunami,
		URL:       url,
	}
}
func mapGraphQLHurricanes(storms []service.Storm) []*Hurricane {
	hurricanes := make([]*Hurricane, 0, len(storms))
	for _, storm := range storms {
		hurricanes = append(hurricanes, mapGraphQLHurricane(storm))
	}
	return hurricanes
}
func mapGraphQLHurricane(storm service.Storm) *Hurricane {
	var minPressure *float64
	if storm.Pressure > 0 {
		value := float64(storm.Pressure)
		minPressure = &value
	}

	var movement *HurricaneMovement
	if storm.MovementDir != "" || storm.MovementSpeed > 0 {
		movement = &HurricaneMovement{}
		if storm.MovementDir != "" {
			movement.Direction = &storm.MovementDir
		}
		if storm.MovementSpeed > 0 {
			speed := float64(storm.MovementSpeed)
			movement.Speed = &speed
		}
	}

	lastUpdated := parseGraphQLTime(storm.LastUpdate)

	return &Hurricane{
		ID:           storm.ID,
		Name:         storm.Name,
		Category:     hurricaneCategory(storm.WindSpeed),
		MaxWindSpeed: float64(storm.WindSpeed),
		MinPressure:  minPressure,
		Location: &Location{
			Name:    storm.Name,
			Country: storm.Basin,
			Lat:     storm.Latitude,
			Lon:     storm.Longitude,
		},
		Status:      storm.Classification,
		Movement:    movement,
		LastUpdated: lastUpdated,
	}
}
func mapGraphQLSevereWeather(data *service.SevereWeatherData) []*SevereWeather {
	if data == nil {
		return []*SevereWeather{}
	}

	alerts := make([]service.Alert, 0, len(data.TornadoWarnings)+len(data.SevereStorms)+len(data.WinterStorms)+len(data.FloodWarnings)+len(data.OtherAlerts))
	alerts = append(alerts, data.TornadoWarnings...)
	alerts = append(alerts, data.SevereStorms...)
	alerts = append(alerts, data.WinterStorms...)
	alerts = append(alerts, data.FloodWarnings...)
	alerts = append(alerts, data.OtherAlerts...)

	items := make([]*SevereWeather, 0, len(alerts))
	for _, alert := range alerts {
		items = append(items, mapGraphQLSevereWeatherAlert(alert))
	}

	return items
}
func mapGraphQLSevereWeatherAlert(alert service.Alert) *SevereWeather {
	description := strings.TrimSpace(alert.Description)
	if description == "" {
		description = strings.TrimSpace(alert.Headline)
	}

	locationName := strings.TrimSpace(alert.AreaDesc)
	if locationName == "" {
		locationName = "Unknown area"
	}

	var instruction *string
	if value := strings.TrimSpace(alert.Instruction); value != "" {
		instruction = &value
	}

	return &SevereWeather{
		ID:       alert.ID,
		Type:     alert.Event,
		Severity: alert.Severity,
		Location: &Location{
			Name:    locationName,
			Country: "Unknown",
			Lat:     0,
			Lon:     0,
		},
		Effective:   parseGraphQLTime(firstNonEmpty(alert.Effective, alert.Sent)),
		Expires:     parseGraphQLTime(firstNonEmpty(alert.Expires, alert.Effective, alert.Sent)),
		Description: description,
		Instruction: instruction,
	}
}
func hurricaneCategory(windSpeed int) int {
	switch {
	case windSpeed >= 157:
		return 5
	case windSpeed >= 130:
		return 4
	case windSpeed >= 111:
		return 3
	case windSpeed >= 96:
		return 2
	case windSpeed >= 74:
		return 1
	default:
		return 0
	}
}
func parseGraphQLTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}

	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"Mon, 02 Jan 2006 15:04:05 MST",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if parsed, err := time.Parse(format, value); err == nil {
			return parsed
		}
	}

	return time.Time{}
}
func buildGraphQLUserToken(id int64, name string, tokenPrefix string, scopes string, createdAt time.Time, expiresAt sql.NullTime, lastUsedAt sql.NullTime, token *string) *UserToken {
	result := &UserToken{
		ID:          strconv.FormatInt(id, 10),
		TokenPrefix: tokenPrefix,
		CreatedAt:   createdAt,
		Token:       token,
	}

	if trimmed := strings.TrimSpace(name); trimmed != "" {
		result.Name = &trimmed
	}
	if trimmed := strings.TrimSpace(scopes); trimmed != "" {
		result.Scopes = &trimmed
	}
	if expiresAt.Valid {
		result.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		result.LastUsedAt = &lastUsedAt.Time
	}

	return result
}
func mapGraphQLUserSettings(settings *handler.UserSettingsResponse) *UserSettings {
	if settings == nil {
		return nil
	}

	return &UserSettings{
		Account: &AccountSettings{
			DisplayName: settings.Account.DisplayName,
			Bio:         settings.Account.Bio,
			Location:    settings.Account.Location,
			Website:     settings.Account.Website,
			Timezone:    settings.Account.Timezone,
			Language:    settings.Account.Language,
			DateFormat:  settings.Account.DateFormat,
			TimeFormat:  settings.Account.TimeFormat,
		},
		Privacy: &PrivacySettings{
			Visibility:   settings.Privacy.Visibility,
			ShowEmail:    settings.Privacy.ShowEmail,
			ShowActivity: settings.Privacy.ShowActivity,
			Searchable:   settings.Privacy.Searchable,
		},
		Notifications: &NotificationSettings{
			EmailSecurity: settings.Notifications.EmailSecurity,
			EmailMentions: settings.Notifications.EmailMentions,
			EmailUpdates:  settings.Notifications.EmailUpdates,
			EmailDigest:   settings.Notifications.EmailDigest,
			PushEnabled:   settings.Notifications.PushEnabled,
			PushMentions:  settings.Notifications.PushMentions,
		},
		Appearance: &AppearanceSettings{
			Theme:        settings.Appearance.Theme,
			FontSize:     settings.Appearance.FontSize,
			ReduceMotion: settings.Appearance.ReduceMotion,
		},
	}
}
func mapGraphQLPublicUserProfile(profile *handler.PublicUserProfile) *PublicUserProfile {
	if profile == nil {
		return nil
	}

	result := &PublicUserProfile{
		Username:  profile.Username,
		Avatar:    &PublicAvatar{Type: profile.Avatar.Type, Urls: profile.Avatar.URLs},
		Verified:  profile.Verified,
		CreatedAt: profile.CreatedAt,
	}
	if trimmed := strings.TrimSpace(profile.DisplayName); trimmed != "" {
		result.DisplayName = &trimmed
	}
	if trimmed := strings.TrimSpace(profile.Bio); trimmed != "" {
		result.Bio = &trimmed
	}
	if trimmed := strings.TrimSpace(profile.Location); trimmed != "" {
		result.Location = &trimmed
	}
	if trimmed := strings.TrimSpace(profile.Website); trimmed != "" {
		result.Website = &trimmed
	}
	return result
}
func mapGraphQLTOTPStatus(status *handler.TwoFactorStatusResponse) *TOTPStatus {
	if status == nil {
		return nil
	}

	return &TOTPStatus{
		Enabled:           status.Enabled,
		RecoveryKeysCount: status.RecoveryKeysCount,
	}
}
func mapGraphQLTOTPSetup(setup *handler.TwoFactorSetupResponse) *TOTPSetup {
	if setup == nil {
		return nil
	}

	return &TOTPSetup{
		Secret:    setup.Secret,
		QrCode:    setup.QRCode,
		ManualURL: setup.ManualURL,
		Account:   setup.Account,
		Issuer:    setup.Issuer,
	}
}
func mapGraphQLUserInviteValidation(response *handler.UserInviteValidationResponse) *UserInviteValidation {
	if response == nil {
		return nil
	}

	return &UserInviteValidation{
		Username:  response.Username,
		Email:     response.Email,
		Role:      response.Role,
		ExpiresAt: response.ExpiresAt,
	}
}
func mapGraphQLServerInviteValidation(response *handler.ServerInviteValidationResponse) *ServerInviteValidation {
	if response == nil {
		return nil
	}

	return &ServerInviteValidation{
		Email:     response.Email,
		ExpiresAt: response.ExpiresAt,
	}
}
func loadGraphQLCurrentUserAuth(ctx context.Context, db *sql.DB) (*models.User, error) {
	userID := getUserIDFromContext(ctx)
	if userID == 0 {
		return nil, fmt.Errorf("unauthorized: user not authenticated")
	}

	userModel := &models.UserModel{DB: db}
	user, err := userModel.GetByID(int64(userID))
	if err != nil {
		return nil, fmt.Errorf("failed to load current user: %w", err)
	}
	return user, nil
}
func getGraphQLCurrentAdminID(ctx context.Context) (int64, error) {
	value := ctx.Value(ctxKeyAdminID)
	switch id := value.(type) {
	case int:
		if id > 0 {
			return int64(id), nil
		}
	case int64:
		if id > 0 {
			return id, nil
		}
	case string:
		parsed, err := strconv.ParseInt(id, 10, 64)
		if err == nil && parsed > 0 {
			return parsed, nil
		}
	}

	return 0, fmt.Errorf("unauthorized: admin session required")
}
func loadGraphQLCurrentAdmin(ctx context.Context, db *sql.DB) (*models.Admin, error) {
	adminID, err := getGraphQLCurrentAdminID(ctx)
	if err != nil {
		return nil, err
	}

	adminModel := &models.AdminModel{DB: db}
	admin, err := adminModel.GetByID(adminID)
	if err != nil {
		return nil, fmt.Errorf("failed to load admin: %w", err)
	}

	return admin, nil
}
func loadGraphQLOnlineAdminUsernames(ctx context.Context, sessionDB, accountsDB *sql.DB) ([]string, error) {
	// The unexpired test is not done in SQL: "expires_at > CURRENT_TIMESTAMP" is a
	// lexicographic TEXT comparison over a column holding two layouts (canonical
	// UTC text and the local-zone time.Time.String() form older builds wrote), so
	// each row's expires_at is parsed in Go instead and a NULL or unparseable
	// expiry never counts as online. admin_sessions lives in server.db and admins
	// in users.db (AI.md PART 12), so the sessions are read first and the active
	// admin accounts second; the result stays ordered ascending by username with
	// no duplicates.
	sessionRows, err := database.QueryContext(ctx, sessionDB, database.TimeoutComplexSelect, `
	SELECT admin_id, expires_at
	FROM admin_sessions
	WHERE expires_at IS NOT NULL
`)
	if err != nil {
		return nil, fmt.Errorf("failed to query online admins: %w", err)
	}
	defer func() { _ = sessionRows.Close() }()

	now := time.Now().UTC()
	onlineAdminIDs := make(map[int64]struct{})
	for sessionRows.Next() {
		var adminID int64
		var storedExpiresAt interface{}
		if err := sessionRows.Scan(&adminID, &storedExpiresAt); err != nil {
			return nil, fmt.Errorf("failed to scan online admin session: %w", err)
		}
		if dbtime.IsAfter(storedExpiresAt, now) {
			onlineAdminIDs[adminID] = struct{}{}
		}
	}
	if err := sessionRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate online admin sessions: %w", err)
	}
	if len(onlineAdminIDs) == 0 {
		return nil, nil
	}

	adminRows, err := database.QueryContext(ctx, accountsDB, database.TimeoutComplexSelect, `
	SELECT id, username
	FROM admins
	WHERE is_active = 1
	ORDER BY username ASC
`)
	if err != nil {
		return nil, fmt.Errorf("failed to query online admin usernames: %w", err)
	}
	defer func() { _ = adminRows.Close() }()

	var usernames []string
	for adminRows.Next() {
		var adminID int64
		var username string
		if err := adminRows.Scan(&adminID, &username); err != nil {
			return nil, fmt.Errorf("failed to scan online admin username: %w", err)
		}
		if _, online := onlineAdminIDs[adminID]; online {
			usernames = append(usernames, username)
		}
	}
	if err := adminRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate online admin usernames: %w", err)
	}

	return usernames, nil
}
func buildGraphQLServerAdmin(admin *models.Admin) *ServerAdmin {
	if admin == nil {
		return nil
	}

	return &ServerAdmin{
		ID:           strconv.FormatInt(admin.ID, 10),
		Username:     admin.Username,
		Email:        admin.Email,
		IsSuperAdmin: admin.IsSuperAdmin,
		IsActive:     admin.IsActive,
		CreatedAt:    admin.CreatedAt,
		UpdatedAt:    admin.UpdatedAt,
		LastLoginAt:  admin.LastLoginAt,
	}
}
func scanGraphQLScheduledTask(scan func(dest ...any) error) (*ScheduledTask, error) {
	var task ScheduledTask
	var lastRun sql.NullInt64
	var nextRun sql.NullInt64

	err := scan(
		&task.Name,
		&task.Schedule,
		&task.Enabled,
		&lastRun,
		&nextRun,
		&task.RunCount,
		&task.ErrorCount,
	)
	if err != nil {
		return nil, err
	}

	if lastRun.Valid {
		lastRunTime := time.Unix(lastRun.Int64, 0).UTC()
		task.LastRun = &lastRunTime
	}
	if nextRun.Valid {
		nextRunTime := time.Unix(nextRun.Int64, 0).UTC()
		task.NextRun = &nextRunTime
	}

	return &task, nil
}
func loadGraphQLNotificationChannel(ctx context.Context, db *sql.DB, channelType string) (*NotificationChannel, error) {
	return scanGraphQLNotificationChannel(database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect,
		"SELECT channel_type, enabled, config FROM server_notification_channels WHERE channel_type = ?",
		channelType,
	).Scan)
}
func scanGraphQLNotificationChannel(scan func(dest ...any) error) (*NotificationChannel, error) {
	var channel NotificationChannel
	var rawConfig sql.NullString

	err := scan(&channel.Type, &channel.Enabled, &rawConfig)
	if err != nil {
		return nil, err
	}

	if rawConfig.Valid && strings.TrimSpace(rawConfig.String) != "" {
		var decoded any
		if err := json.Unmarshal([]byte(rawConfig.String), &decoded); err == nil {
			channel.Config = decoded
		} else {
			channel.Config = rawConfig.String
		}
	}

	return &channel, nil
}
func loadGraphQLSetting(ctx context.Context, db *sql.DB, key string) (*models.Setting, error) {
	return (&models.SettingsModel{}).GetSetting(key)
}
func scanGraphQLSetting(scan func(dest ...any) error) (*models.Setting, error) {
	var setting models.Setting
	var updatedAt sql.NullTime
	var description sql.NullString

	err := scan(&setting.Key, &setting.Value, &setting.Type, &description, &updatedAt)
	if err != nil {
		return nil, err
	}

	if description.Valid {
		setting.Description = description.String
	}
	if updatedAt.Valid {
		setting.UpdatedAt = updatedAt.Time
	}

	return &setting, nil
}
func mapSchedulerTaskHistory(run scheduler.TaskRun) *TaskHistory {
	duration := float64(run.Duration)
	var completedAt *time.Time
	if !run.EndTime.IsZero() {
		completedAt = &run.EndTime
	}

	var errorText *string
	if strings.TrimSpace(run.Error) != "" {
		errorCopy := run.Error
		errorText = &errorCopy
	}

	return &TaskHistory{
		TaskName:    run.TaskName,
		StartedAt:   run.StartTime,
		CompletedAt: completedAt,
		Duration:    &duration,
		Ok:          run.Status == "success",
		Error:       errorText,
	}
}

// graphQLAuditLogSkewWindow widens the SQL prefilter in loadGraphQLRequestStats
// past any real UTC offset (the largest in use is +14:00), so a row written in
// the local-zone layout can never sort below the bound and be missed.
//
// This widens only the SCAN, never a reported window: every row the query
// returns is re-bounded against startOfDay/lastMinuteStart in Go after being
// resolved to a real instant, so the three totals are exact regardless of how
// far back the prefilter reaches. It stays in place even though every current
// producer of audit_log.timestamp now writes canonical UTC text -
// databases created before that are still on disk, and dropping the margin
// would silently exclude their rows from today's counts.
const graphQLAuditLogSkewWindow = 15 * time.Hour

func loadGraphQLRequestStats(ctx context.Context, serverDB *sql.DB) (*RequestStats, error) {
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	lastMinuteStart := now.Add(-time.Minute)

	// The three counts were COUNT(*) queries bounded by date('now', 'start of
	// day') and datetime('now', '-1 minute'). audit_log.timestamp holds
	// two layouts - canonical UTC text and the local-zone
	// time.Time.String() form written elsewhere - so those lexicographic
	// comparisons counted rows from the wrong window, and any datetime() wrapper
	// would have returned NULL for the local-zone rows and dropped them
	// entirely.
	//
	// SQL now only narrows the scan to a superset of the widest window needed
	// (start of the UTC day, pushed back by the maximum possible zone skew;
	// lastMinuteStart is always inside that range). Each row's timestamp is then
	// resolved to a real instant in Go and counted against the exact bounds, so
	// the three totals are the ones the original queries intended.
	rows, err := database.QueryContext(ctx, serverDB, database.TimeoutComplexSelect, `
	SELECT timestamp, status
	FROM audit_log
	WHERE timestamp >= ?
`, dbtime.FormatSQLTimestamp(startOfDay.Add(-graphQLAuditLogSkewWindow)))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("WARNING: failed to close GraphQL audit metric rows: %v", err)
		}
	}()

	var totalToday int
	var errorsToday int
	var lastMinute int

	for rows.Next() {
		var storedTimestamp interface{}
		var status sql.NullString
		if err := rows.Scan(&storedTimestamp, &status); err != nil {
			return nil, err
		}

		// An entry whose timestamp cannot be resolved counts toward no window.
		recordedAt, ok := dbtime.ParseStoredTimestamp(storedTimestamp)
		if !ok {
			continue
		}

		if !recordedAt.Before(startOfDay) {
			totalToday++
			if status.Valid && status.String == "error" {
				errorsToday++
			}
		}

		if !recordedAt.Before(lastMinuteStart) {
			lastMinute++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	perSecond := float64(lastMinute) / 60.0
	return &RequestStats{
		Total:     totalToday,
		PerSecond: &perSecond,
		Errors:    &errorsToday,
	}, nil
}
func loadGraphQLDatabaseStats(ctx context.Context, serverDB, usersDB *sql.DB) (*DatabaseStats, error) {
	serverSize, serverTables, err := loadGraphQLSQLiteStats(ctx, serverDB)
	if err != nil {
		return nil, err
	}

	usersSize := 0.0
	usersTables := 0
	if usersDB != nil {
		usersSize, usersTables, err = loadGraphQLSQLiteStats(ctx, usersDB)
		if err != nil {
			return nil, err
		}
	}

	tables := serverTables + usersTables
	connections := 0
	if serverDB != nil {
		connections++
	}
	if usersDB != nil && usersDB != serverDB {
		connections++
	}

	return &DatabaseStats{
		Size:        serverSize + usersSize,
		Tables:      &tables,
		Connections: &connections,
	}, nil
}
func loadGraphQLSQLiteStats(ctx context.Context, db *sql.DB) (float64, int, error) {
	if db == nil {
		return 0, 0, nil
	}
	if database.IsRemoteDatabase(db) {
		var size float64
		var tables int
		var query string
		if database.RemoteDatabaseDialect(db) == "mysql" || database.RemoteDatabaseDialect(db) == "mariadb" {
			query = `SELECT COALESCE(SUM(data_length + index_length), 0), COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()`
		} else {
			query = `SELECT COALESCE(SUM(pg_total_relation_size(quote_ident(schemaname) || '.' || quote_ident(tablename))), 0), COUNT(*) FROM pg_tables WHERE schemaname = current_schema()`
		}
		if err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect, query).Scan(&size, &tables); err != nil {
			return 0, 0, err
		}
		return size, tables, nil
	}

	var pageCount int64
	var pageSize int64
	var tables int

	if err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect, "PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, 0, err
	}
	if err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, 0, err
	}
	if err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'").Scan(&tables); err != nil {
		return 0, 0, err
	}

	return float64(pageCount * pageSize), tables, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}
func (r *settingResolver) UpdatedAt(ctx context.Context, obj *models.Setting) (*time.Time, error) {
	return &obj.UpdatedAt, nil
}

func buildGraphQLUserInvite(invite *models.UserInvite, ctx context.Context) *UserInvite {
	if invite == nil {
		return nil
	}

	return &UserInvite{
		ID:        strconv.FormatInt(invite.ID, 10),
		Token:     invite.Token,
		Username:  invite.Username,
		Email:     invite.Email,
		Role:      invite.Role,
		CreatedAt: invite.CreatedAt,
		ExpiresAt: invite.ExpiresAt,
		MaxUses:   invite.MaxUses,
		UseCount:  invite.UseCount,
		UsedAt:    invite.UsedAt,
		Status:    graphQLUserInviteStatus(invite),
		InviteURL: graphQLUserInviteURL(ctx, invite.Token),
	}
}
func graphQLUserInviteStatus(invite *models.UserInvite) string {
	if invite == nil {
		return "pending"
	}
	if invite.UsedAt != nil || (invite.MaxUses > 0 && invite.UseCount >= invite.MaxUses) {
		return "used"
	}
	if time.Now().After(invite.ExpiresAt) {
		return "expired"
	}
	return "pending"
}
func graphQLUserInviteURL(ctx context.Context, token string) string {
	host, _ := ctx.Value(ctxKeyRequestHost).(string)
	if strings.TrimSpace(host) == "" {
		return ""
	}

	scheme, _ := ctx.Value(ctxKeyRequestScheme).(string)
	if strings.TrimSpace(scheme) == "" {
		scheme = "http"
	}

	return fmt.Sprintf("%s://%s/server/auth/invite/user/%s", scheme, host, token)
}
