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
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/scheduler"
	"github.com/webappsgo/wthr/src/server/handler"
	models "github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/service"
)

// Ok is the resolver for the GenericResponse.ok field. The Go struct uses
// `Success` (matching BulkResponse / ContactSubmission convention) but the
// schema exposes `ok`, so gqlgen requires this trampoline.
func (r *genericResponseResolver) Ok(ctx context.Context, obj *GenericResponse) (bool, error) {
	return obj.Success, nil
}

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
			Visibility:    settings.Privacy.Visibility,
			ShowEmail:     settings.Privacy.ShowEmail,
			ShowActivity:  settings.Privacy.ShowActivity,
			ShowOrgs:      settings.Privacy.ShowOrgs,
			Searchable:    settings.Privacy.Searchable,
			OrgVisibility: settings.Privacy.OrgVisibility,
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
func mapGraphQLRecoveryKeysResponse(response *handler.RecoveryKeysResponse) *TOTPRecoveryKeys {
	if response == nil {
		return nil
	}

	return &TOTPRecoveryKeys{
		Message:      response.Message,
		RecoveryKeys: append([]string(nil), response.RecoveryKeys...),
	}
}
func mapGraphQLAuthUser(user *handler.AuthUserSummary) *AuthUser {
	if user == nil {
		return nil
	}

	return &AuthUser{
		ID:       strconv.FormatInt(user.ID, 10),
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
	}
}
func mapGraphQLAuthLoginResponse(response *handler.AuthLoginResponse) *AuthResult {
	if response == nil {
		return nil
	}

	result := &AuthResult{
		RequiresTwoFactor: response.RequiresTwoFactor,
		User:              mapGraphQLAuthUser(response.User),
		ExpiresAt:         response.ExpiresAt,
		RemainingKeys:     response.RemainingKeys,
	}
	if trimmed := strings.TrimSpace(response.SessionToken); trimmed != "" {
		result.SessionToken = &trimmed
	}
	if trimmed := strings.TrimSpace(response.Token); trimmed != "" {
		result.Token = &trimmed
	}
	return result
}
func mapGraphQLAuthRegisterResponse(response *handler.AuthRegisterResponse) *AuthResult {
	if response == nil {
		return nil
	}

	result := &AuthResult{
		VerificationRequired: response.VerificationRequired,
		User:                 mapGraphQLAuthUser(response.User),
	}
	if trimmed := strings.TrimSpace(response.Token); trimmed != "" {
		result.Token = &trimmed
	}
	return result
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
func mapGraphQLUserInviteCompletion(response *handler.UserInviteCompletionResponse) *UserInviteCompletion {
	if response == nil {
		return nil
	}

	result := &UserInviteCompletion{
		User: mapGraphQLAuthUser(response.User),
	}
	if trimmed := strings.TrimSpace(response.Message); trimmed != "" {
		result.Message = &trimmed
	}
	if trimmed := strings.TrimSpace(response.Token); trimmed != "" {
		result.Token = &trimmed
	}
	return result
}
func mapGraphQLServerInviteCompletion(response *handler.ServerInviteCompletionResponse) *ServerInviteCompletion {
	if response == nil || response.Admin == nil {
		return nil
	}

	return &ServerInviteCompletion{
		Message: response.Message,
		Admin: &InvitedServerAdmin{
			ID:       strconv.FormatInt(response.Admin.ID, 10),
			Username: response.Admin.Username,
			Email:    response.Admin.Email,
		},
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
func loadGraphQLCurrentUserSession(ctx context.Context) (*models.Session, error) {
	sessionValue := ctx.Value("user_session")
	session, ok := sessionValue.(*models.Session)
	if !ok || session == nil {
		return nil, fmt.Errorf("session authentication required")
	}
	return session, nil
}
func graphQLRequestBaseURL(ctx context.Context) string {
	host, _ := ctx.Value("request_host").(string)
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}

	scheme, _ := ctx.Value("request_scheme").(string)
	scheme = strings.TrimSpace(scheme)
	if scheme == "" {
		scheme = "http"
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}
func getGraphQLCurrentAdminID(ctx context.Context) (int64, error) {
	value := ctx.Value("admin_id")
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
func loadGraphQLOnlineAdminUsernames(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
	SELECT DISTINCT sac.username
	FROM server_admin_credentials sac
	INNER JOIN server_admin_sessions sas ON sas.admin_id = sac.id
	WHERE sac.is_active = 1 AND sas.expires_at > CURRENT_TIMESTAMP
	ORDER BY sac.username ASC
`)
	if err != nil {
		return nil, fmt.Errorf("failed to query online admins: %w", err)
	}
	defer rows.Close()

	var usernames []string
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, fmt.Errorf("failed to scan online admin username: %w", err)
		}
		usernames = append(usernames, username)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate online admin usernames: %w", err)
	}

	return usernames, nil
}
func countGraphQLOtherActiveSuperAdmins(db *sql.DB, excludeID int64) (int, error) {
	var count int
	err := db.QueryRow(`
	SELECT COUNT(*)
	FROM server_admin_credentials
	WHERE is_super_admin = 1 AND is_active = 1 AND id != ?
`, excludeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active super admins: %w", err)
	}

	return count, nil
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
func graphQLServerInviteURL(ctx context.Context, token string) string {
	scheme, _ := ctx.Value("request_scheme").(string)
	if strings.TrimSpace(scheme) == "" {
		scheme = "http"
	}

	host, _ := ctx.Value("request_host").(string)
	if strings.TrimSpace(host) == "" {
		return ""
	}

	return fmt.Sprintf("%s://%s/auth/invite/server/%s", scheme, host, token)
}
func buildGraphQLServerAdminInvite(ctx context.Context, invite *models.AdminInvite, expiresIn string) *ServerAdminInvite {
	if invite == nil {
		return nil
	}

	return &ServerAdminInvite{
		Token:     invite.Token,
		Email:     invite.InvitedEmail,
		ExpiresAt: invite.ExpiresAt,
		ExpiresIn: expiresIn,
		InviteURL: graphQLServerInviteURL(ctx, invite.Token),
	}
}
func (r *mutationResolver) updateGraphQLScheduledTaskEnabled(name string, enabled bool) (*ScheduledTask, error) {
	_, err := r.ServerDB.Exec(`
	UPDATE server_scheduler_state
	SET enabled = ?, locked_by = NULL, locked_at = NULL
	WHERE task_name = ?
`, enabled, name)
	if err != nil {
		return nil, err
	}

	task, err := r.loadGraphQLScheduledTask(name)
	if err != nil {
		return nil, err
	}

	return task, nil
}
func (r *mutationResolver) loadGraphQLScheduledTask(name string) (*ScheduledTask, error) {
	return scanGraphQLScheduledTask(r.ServerDB.QueryRow(
		"SELECT task_name, schedule, enabled, last_run, next_run, run_count, fail_count FROM server_scheduler_state WHERE task_name = ?",
		name,
	).Scan)
}
func scanGraphQLScheduledTask(scan func(dest ...any) error) (*ScheduledTask, error) {
	var task ScheduledTask
	var lastRun sql.NullTime
	var nextRun sql.NullTime

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
		task.LastRun = &lastRun.Time
	}
	if nextRun.Valid {
		task.NextRun = &nextRun.Time
	}

	return &task, nil
}
func loadGraphQLNotificationChannel(db *sql.DB, channelType string) (*NotificationChannel, error) {
	return scanGraphQLNotificationChannel(db.QueryRow(
		"SELECT channel_type, enabled, config FROM notification_channels WHERE channel_type = ?",
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
func loadGraphQLSetting(db *sql.DB, key string) (*models.Setting, error) {
	return scanGraphQLSetting(db.QueryRow(
		"SELECT key, value, type, COALESCE(description, ''), updated_at FROM server_config WHERE key = ?",
		key,
	).Scan)
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
func loadGraphQLRequestStats(serverDB *sql.DB) (*RequestStats, error) {
	var totalToday int
	var errorsToday int
	var lastMinute int

	if err := serverDB.QueryRow(`
	SELECT COUNT(*) FROM server_audit_log
	WHERE timestamp >= date('now', 'start of day')
`).Scan(&totalToday); err != nil {
		return nil, err
	}
	if err := serverDB.QueryRow(`
	SELECT COUNT(*) FROM server_audit_log
	WHERE timestamp >= date('now', 'start of day') AND status = 'error'
`).Scan(&errorsToday); err != nil {
		return nil, err
	}
	if err := serverDB.QueryRow(`
	SELECT COUNT(*) FROM server_audit_log
	WHERE timestamp >= datetime('now', '-1 minute')
`).Scan(&lastMinute); err != nil {
		return nil, err
	}

	perSecond := float64(lastMinute) / 60.0
	return &RequestStats{
		Total:     totalToday,
		PerSecond: &perSecond,
		Errors:    &errorsToday,
	}, nil
}
func loadGraphQLDatabaseStats(serverDB, usersDB *sql.DB) (*DatabaseStats, error) {
	serverSize, serverTables, err := loadGraphQLSQLiteStats(serverDB)
	if err != nil {
		return nil, err
	}

	usersSize := 0.0
	usersTables := 0
	if usersDB != nil {
		usersSize, usersTables, err = loadGraphQLSQLiteStats(usersDB)
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
func loadGraphQLSQLiteStats(db *sql.DB) (float64, int, error) {
	if db == nil {
		return 0, 0, nil
	}

	var pageCount int64
	var pageSize int64
	var tables int

	if err := db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, 0, err
	}
	if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, 0, err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'").Scan(&tables); err != nil {
		return 0, 0, err
	}

	return float64(pageCount * pageSize), tables, nil
}
func ensureGraphQLContactSubmissionsTable(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("server database unavailable")
	}

	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS contact_submissions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		subject TEXT NOT NULL,
		message TEXT NOT NULL,
		ip_address TEXT,
		user_agent TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
	)
`)
	return err
}
func (r *mutationResolver) resolveAdminChannelTestRecipient(ctx context.Context, typeArg string, recipient *string) (string, error) {
	if recipient != nil {
		trimmed := strings.TrimSpace(*recipient)
		if trimmed != "" {
			return trimmed, nil
		}
	}

	if typeArg != "email" {
		return "", fmt.Errorf("channel %q testing requires a recipient", typeArg)
	}

	smtpService := service.NewSMTPService(r.ServerDB)
	if err := smtpService.LoadConfig(); err != nil {
		return "", fmt.Errorf("failed to load SMTP configuration: %w", err)
	}

	if config := smtpService.GetConfig(); config != nil {
		recipient := strings.TrimSpace(config.TestRecipient)
		if recipient != "" {
			return recipient, nil
		}
	}

	if adminID, ok := ctx.Value("admin_id").(int); ok && adminID > 0 {
		var adminEmail string
		err := r.ServerDB.QueryRow(`
		SELECT email
		FROM server_admin_credentials
		WHERE id = ? AND is_active = 1
	`, adminID).Scan(&adminEmail)
		if err != nil && err != sql.ErrNoRows {
			return "", fmt.Errorf("failed to load authenticated admin email: %w", err)
		}
		adminEmail = strings.TrimSpace(adminEmail)
		if adminEmail != "" {
			return adminEmail, nil
		}
	}

	if adminEmail, ok := ctx.Value("admin_email").(string); ok {
		adminEmail = strings.TrimSpace(adminEmail)
		if adminEmail != "" {
			return adminEmail, nil
		}
	}

	var fallbackEmail string
	err := r.ServerDB.QueryRow(`
	SELECT email
	FROM server_admin_credentials
	WHERE is_active = 1 AND email IS NOT NULL AND TRIM(email) != ''
	ORDER BY is_super_admin DESC, id ASC
	LIMIT 1
`).Scan(&fallbackEmail)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("email channel testing requires smtp.test_recipient or an active server admin email")
		}
		return "", fmt.Errorf("failed to load fallback admin email: %w", err)
	}

	return strings.TrimSpace(fallbackEmail), nil
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
	host, _ := ctx.Value("request_host").(string)
	if strings.TrimSpace(host) == "" {
		return ""
	}

	scheme, _ := ctx.Value("request_scheme").(string)
	if strings.TrimSpace(scheme) == "" {
		scheme = "http"
	}

	return fmt.Sprintf("%s://%s/auth/invite/user/%s", scheme, host, token)
}
