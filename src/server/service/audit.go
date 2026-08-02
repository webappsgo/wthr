package service

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// EventType represents the type of audit event
type EventType string

// Audit event types (180+ types organized by category)
const (
	// User Authentication Events (1-20)
	EventUserLogin          EventType = "user.login"
	EventUserLoginFailed    EventType = "user.login.failed"
	EventUserLogout         EventType = "user.logout"
	EventUserRegister       EventType = "user.register"
	EventUserRegisterFailed EventType = "user.register.failed"
	EventUserPasswordChange EventType = "user.password.change"
	EventUserPasswordReset  EventType = "user.password.reset"
	EventUserEmailVerify    EventType = "user.email.verify"
	EventUserEmailChange    EventType = "user.email.change"
	EventUserAccountLock    EventType = "user.account.lock"
	EventUserAccountUnlock  EventType = "user.account.unlock"
	EventUserAccountDelete  EventType = "user.account.delete"
	EventUserSessionCreate  EventType = "user.session.create"
	EventUserSessionDestroy EventType = "user.session.destroy"
	EventUserSessionExpire  EventType = "user.session.expire"
	EventUserProfileUpdate  EventType = "user.profile.update"
	EventUserAvatarChange   EventType = "user.avatar.change"
	EventUserPrefsUpdate    EventType = "user.preferences.update"
	EventUser2FAEnable      EventType = "user.2fa.enable"
	EventUser2FADisable     EventType = "user.2fa.disable"

	// User Security Events (21-40)
	EventUserRecoveryKeyGenerate EventType = "user.recovery.generate"
	EventUserRecoveryKeyUse      EventType = "user.recovery.use"
	EventUserAPIKeyCreate        EventType = "user.apikey.create"
	EventUserAPIKeyRevoke        EventType = "user.apikey.revoke"
	EventUserAPIKeyUse           EventType = "user.apikey.use"
	EventUserOAuthLink           EventType = "user.oauth.link"
	EventUserOAuthUnlink         EventType = "user.oauth.unlink"
	EventUserLocationAdd         EventType = "user.location.add"
	EventUserLocationUpdate      EventType = "user.location.update"
	EventUserLocationDelete      EventType = "user.location.delete"
	EventUserNotificationEnable  EventType = "user.notification.enable"
	EventUserNotificationDisable EventType = "user.notification.disable"
	EventUserDataExport          EventType = "user.data.export"
	EventUserDataImport          EventType = "user.data.import"
	EventUserConsentGrant        EventType = "user.consent.grant"
	EventUserConsentRevoke       EventType = "user.consent.revoke"
	EventUserDeviceAdd           EventType = "user.device.add"
	EventUserDeviceRemove        EventType = "user.device.remove"
	EventUserSecurityAlert       EventType = "user.security.alert"
	EventUserSuspiciousActivity  EventType = "user.suspicious.activity"

	// Admin Actions (41-80)
	EventAdminLogin             EventType = "admin.login"
	EventAdminLogout            EventType = "admin.logout"
	EventAdminSettingsChange    EventType = "admin.settings.change"
	EventAdminUserCreate        EventType = "admin.user.create"
	EventAdminUserUpdate        EventType = "admin.user.update"
	EventAdminUserDelete        EventType = "admin.user.delete"
	EventAdminUserImpersonate   EventType = "admin.user.impersonate"
	EventAdminRoleCreate        EventType = "admin.role.create"
	EventAdminRoleUpdate        EventType = "admin.role.update"
	EventAdminRoleDelete        EventType = "admin.role.delete"
	EventAdminRoleAssign        EventType = "admin.role.assign"
	EventAdminDatabaseBackup    EventType = "admin.database.backup"
	EventAdminDatabaseRestore   EventType = "admin.database.restore"
	EventAdminDatabaseOptimize  EventType = "admin.database.optimize"
	EventAdminDatabaseVacuum    EventType = "admin.database.vacuum"
	EventAdminCacheClear        EventType = "admin.cache.clear"
	EventAdminCacheFlush        EventType = "admin.cache.flush"
	EventAdminLogsClear         EventType = "admin.logs.clear"
	EventAdminLogsRotate        EventType = "admin.logs.rotate"
	EventAdminLogsDownload      EventType = "admin.logs.download"
	EventAdminEmailTemplateEdit EventType = "admin.email.template.edit"
	EventAdminEmailTest         EventType = "admin.email.test"
	EventAdminNotificationSend  EventType = "admin.notification.send"
	EventAdminSystemRestart     EventType = "admin.system.restart"
	EventAdminSystemShutdown    EventType = "admin.system.shutdown"
	EventAdminConfigReload      EventType = "admin.config.reload"
	EventAdminConfigExport      EventType = "admin.config.export"
	EventAdminConfigImport      EventType = "admin.config.import"
	EventAdminCertObtain        EventType = "admin.cert.obtain"
	EventAdminCertRenew         EventType = "admin.cert.renew"
	EventAdminCertRevoke        EventType = "admin.cert.revoke"
	EventAdminPluginInstall     EventType = "admin.plugin.install"
	EventAdminPluginUninstall   EventType = "admin.plugin.uninstall"
	EventAdminPluginEnable      EventType = "admin.plugin.enable"
	EventAdminPluginDisable     EventType = "admin.plugin.disable"
	EventAdminTaskEnable        EventType = "admin.task.enable"
	EventAdminTaskDisable       EventType = "admin.task.disable"
	EventAdminTaskTrigger       EventType = "admin.task.trigger"
	EventAdminWebRobotsUpdate   EventType = "admin.web.robots.update"
	EventAdminWebSecurityUpdate EventType = "admin.web.security.update"

	// System Events (81-120)
	EventSystemStartup       EventType = "system.startup"
	EventSystemShutdown      EventType = "system.shutdown"
	EventSystemRestart       EventType = "system.restart"
	EventSystemError         EventType = "system.error"
	EventSystemWarning       EventType = "system.warning"
	EventSystemPanic         EventType = "system.panic"
	EventSystemConfigChange  EventType = "system.config.change"
	EventSystemUpdate        EventType = "system.update"
	EventSystemMaintenance   EventType = "system.maintenance"
	EventSystemBackupStart   EventType = "system.backup.start"
	EventSystemBackupFinish  EventType = "system.backup.finish"
	EventSystemBackupFail    EventType = "system.backup.fail"
	EventSystemRestoreStart  EventType = "system.restore.start"
	EventSystemRestoreFinish EventType = "system.restore.finish"
	EventSystemRestoreFail   EventType = "system.restore.fail"
	EventSystemTaskStart     EventType = "system.task.start"
	EventSystemTaskFinish    EventType = "system.task.finish"
	EventSystemTaskFail      EventType = "system.task.fail"
	EventSystemDiskLow       EventType = "system.disk.low"
	EventSystemMemoryHigh    EventType = "system.memory.high"
	EventSystemCPUHigh       EventType = "system.cpu.high"
	EventSystemDBConnect     EventType = "system.db.connect"
	EventSystemDBDisconnect  EventType = "system.db.disconnect"
	EventSystemDBError       EventType = "system.db.error"
	EventSystemCacheConnect  EventType = "system.cache.connect"
	EventSystemCacheError    EventType = "system.cache.error"
	EventSystemSMTPConnect   EventType = "system.smtp.connect"
	EventSystemSMTPError     EventType = "system.smtp.error"
	EventSystemAPIError      EventType = "system.api.error"
	EventSystemRateLimit     EventType = "system.ratelimit.trigger"
	EventSystemFirewall      EventType = "system.firewall.block"
	EventSystemSSLError      EventType = "system.ssl.error"
	EventSystemSSLExpiring   EventType = "system.ssl.expiring"
	EventSystemSSLRenew      EventType = "system.ssl.renew"
	EventSystemTorStart      EventType = "system.tor.start"
	EventSystemTorStop       EventType = "system.tor.stop"
	EventSystemTorRegenerate EventType = "system.tor.regenerate"
	EventSystemTorError      EventType = "system.tor.error"
	EventSystemGeoIPUpdate   EventType = "system.geoip.update"
	EventSystemGeoIPError    EventType = "system.geoip.error"

	// API Events (121-150)
	EventAPIRequest         EventType = "api.request"
	EventAPIRequestFailed   EventType = "api.request.failed"
	EventAPIAuthSuccess     EventType = "api.auth.success"
	EventAPIAuthFailed      EventType = "api.auth.failed"
	EventAPIRateLimitHit    EventType = "api.ratelimit.hit"
	EventAPIKeyInvalid      EventType = "api.key.invalid"
	EventAPIKeyExpired      EventType = "api.key.expired"
	EventAPIWeatherFetch    EventType = "api.weather.fetch"
	EventAPIEarthquakeFetch EventType = "api.earthquake.fetch"
	EventAPIHurricaneFetch  EventType = "api.hurricane.fetch"
	EventAPIAlertsFetch     EventType = "api.alerts.fetch"
	EventAPIMoonFetch       EventType = "api.moon.fetch"
	EventAPIHistoryFetch    EventType = "api.history.fetch"
	EventAPILocationSearch  EventType = "api.location.search"
	EventAPIGeocodeLookup   EventType = "api.geocode.lookup"
	EventAPIIPLookup        EventType = "api.ip.lookup"
	EventAPIWebhookCreate   EventType = "api.webhook.create"
	EventAPIWebhookUpdate   EventType = "api.webhook.update"
	EventAPIWebhookDelete   EventType = "api.webhook.delete"
	EventAPIWebhookTrigger  EventType = "api.webhook.trigger"
	EventAPIWebhookFail     EventType = "api.webhook.fail"
	EventAPITokenCreate     EventType = "api.token.create"
	EventAPITokenRevoke     EventType = "api.token.revoke"
	EventAPITokenRefresh    EventType = "api.token.refresh"
	EventAPIBatchRequest    EventType = "api.batch.request"
	EventAPIExportData      EventType = "api.export.data"
	EventAPIImportData      EventType = "api.import.data"
	EventAPIInvalidRequest  EventType = "api.request.invalid"
	EventAPIServerError     EventType = "api.server.error"
	EventAPITimeout         EventType = "api.timeout"

	// Security Events (151-180)
	EventSecurityBruteForce    EventType = "security.bruteforce.detected"
	EventSecurityIPBlocked     EventType = "security.ip.blocked"
	EventSecurityIPUnblocked   EventType = "security.ip.unblocked"
	EventSecuritySQLInjection  EventType = "security.sql.injection"
	EventSecurityXSSAttempt    EventType = "security.xss.attempt"
	EventSecurityCSRFDetected  EventType = "security.csrf.detected"
	EventSecurityFileUpload    EventType = "security.file.upload"
	EventSecurityFileRejected  EventType = "security.file.rejected"
	EventSecurityPathTraversal EventType = "security.path.traversal"
	EventSecurityCommandInject EventType = "security.command.injection"
	EventSecurityPrivEscalate  EventType = "security.privilege.escalation"
	EventSecurityDataLeak      EventType = "security.data.leak"
	EventSecurityUnauthorized  EventType = "security.unauthorized.access"
	EventSecuritySessionHijack EventType = "security.session.hijack"
	EventSecurityTokenSteal    EventType = "security.token.steal"
	EventSecurityCORSViolation EventType = "security.cors.violation"
	EventSecurityCSPViolation  EventType = "security.csp.violation"
	EventSecurityDDoSAttempt   EventType = "security.ddos.attempt"
	EventSecurityBotDetected   EventType = "security.bot.detected"
	EventSecurityScanDetected  EventType = "security.scan.detected"
	EventSecurityPhishing      EventType = "security.phishing.attempt"
	EventSecurityMalware       EventType = "security.malware.detected"
	EventSecurityRansomware    EventType = "security.ransomware.detected"
	EventSecurityBackdoor      EventType = "security.backdoor.detected"
	EventSecurityCrypto        EventType = "security.crypto.miner"
	EventSecurityReverseTunnel EventType = "security.reverse.tunnel"
	EventSecurityDataExfil     EventType = "security.data.exfiltration"
	EventSecurityZeroDay       EventType = "security.zeroday.attempt"
	EventSecurityCompliance    EventType = "security.compliance.violation"
	EventSecurityAuditFail     EventType = "security.audit.fail"
)

// Actor represents who performed an action
type Actor struct {
	// admin, user, system, api
	Type string `json:"type"`
	// Admin username or user ID
	ID string `json:"id"`
	// IP address
	IP string `json:"ip"`
	// Browser/client info
	UserAgent string `json:"user_agent"`
}

// Target represents what was acted upon
type Target struct {
	// session, user, config, etc.
	Type string `json:"type"`
	// Target identifier
	ID string `json:"id"`
}

// AuditEvent represents a single audit log entry (TEMPLATE.md spec)
type AuditEvent struct {
	// ULID format
	ID string `json:"id"`
	// X-Request-ID header value (UUID v4) for request tracing
	RequestID string `json:"request_id,omitempty"`
	// ISO 8601, UTC
	Time time.Time `json:"time"`
	// e.g., "admin.login"
	Event string `json:"event"`
	// authentication, configuration, security
	Category string `json:"category"`
	// info, warn, error, critical
	Severity string `json:"severity"`
	// Who performed the action
	Actor   Actor                  `json:"actor"`
	Target  *Target                `json:"target,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
	// success or failure
	Result string `json:"result"`
	// For cluster mode
	NodeID string `json:"node_id,omitempty"`
	// Optional reason
	Reason string `json:"reason,omitempty"`

	// Legacy fields for backwards compatibility
	// Internal use only
	UserID int64 `json:"-"`
	// Internal use only
	Username string `json:"-"`
	// Use Actor.IP instead
	IP string `json:"-"`
	// Use Actor.UserAgent instead
	UserAgent string `json:"-"`
	// Use Result instead
	Success bool `json:"-"`
	// Use Details instead
	Error string `json:"-"`
	// Use Event instead
	EventType EventType `json:"-"`
}

// AuditLogger handles audit logging
type AuditLogger struct {
	logFile string
	mu      sync.Mutex
	file    *os.File
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logDir string) (*AuditLogger, error) {
	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logFile := filepath.Join(logDir, "audit.log")

	// Open log file in append mode
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}

	return &AuditLogger{
		logFile: logFile,
		file:    file,
	}, nil
}

// Log writes an audit event to the log file
func (al *AuditLogger) Log(event AuditEvent) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	// Set timestamp if not already set
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}

	// Generate ID if not set (use ULID per AI.md PART 18 line 18898)
	if event.ID == "" {
		event.ID = ulid.MustNew(ulid.Timestamp(event.Time), rand.Reader).String()
	}

	// Set default severity if not provided
	if event.Severity == "" {
		if event.Result == "failure" {
			event.Severity = "warn"
		} else {
			event.Severity = "info"
		}
	}

	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	// Write to file with newline
	if _, err := al.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write audit log: %w", err)
	}

	return nil
}

// LogSuccess logs a successful event
func (al *AuditLogger) LogSuccess(event string, category string, actorType string, actorID string, ip string, details map[string]interface{}) error {
	return al.Log(AuditEvent{
		Event:    event,
		Category: category,
		Severity: "info",
		Actor: Actor{
			Type: actorType,
			ID:   actorID,
			IP:   ip,
		},
		Details: details,
		Result:  "success",
	})
}

// LogFailure logs a failed event
func (al *AuditLogger) LogFailure(event string, category string, actorType string, actorID string, ip string, errorMsg string, details map[string]interface{}) error {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["error"] = errorMsg

	return al.Log(AuditEvent{
		Event:    event,
		Category: category,
		Severity: "warn",
		Actor: Actor{
			Type: actorType,
			ID:   actorID,
			IP:   ip,
		},
		Details: details,
		Result:  "failure",
	})
}

// Close closes the audit log file
func (al *AuditLogger) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()

	if al.file != nil {
		return al.file.Close()
	}

	return nil
}

// Rotate rotates the audit log file
func (al *AuditLogger) Rotate() error {
	al.mu.Lock()
	defer al.mu.Unlock()

	// Close current file
	if al.file != nil {
		al.file.Close()
	}

	// Rename current file with timestamp
	timestamp := time.Now().Format("20060102-150405")
	archiveFile := fmt.Sprintf("%s.%s", al.logFile, timestamp)

	if err := os.Rename(al.logFile, archiveFile); err != nil {
		return fmt.Errorf("failed to rotate audit log: %w", err)
	}

	// Open new file
	file, err := os.OpenFile(al.logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open new audit log: %w", err)
	}

	al.file = file

	return nil
}

// GetEventTypes returns all available event types
func GetEventTypes() []EventType {
	return []EventType{
		// User Authentication Events
		EventUserLogin, EventUserLoginFailed, EventUserLogout, EventUserRegister,
		EventUserRegisterFailed, EventUserPasswordChange, EventUserPasswordReset,
		EventUserEmailVerify, EventUserEmailChange, EventUserAccountLock,
		EventUserAccountUnlock, EventUserAccountDelete, EventUserSessionCreate,
		EventUserSessionDestroy, EventUserSessionExpire, EventUserProfileUpdate,
		EventUserAvatarChange, EventUserPrefsUpdate, EventUser2FAEnable, EventUser2FADisable,

		// User Security Events
		EventUserRecoveryKeyGenerate, EventUserRecoveryKeyUse, EventUserAPIKeyCreate,
		EventUserAPIKeyRevoke, EventUserAPIKeyUse, EventUserOAuthLink, EventUserOAuthUnlink,
		EventUserLocationAdd, EventUserLocationUpdate, EventUserLocationDelete,
		EventUserNotificationEnable, EventUserNotificationDisable, EventUserDataExport,
		EventUserDataImport, EventUserConsentGrant, EventUserConsentRevoke,
		EventUserDeviceAdd, EventUserDeviceRemove, EventUserSecurityAlert,
		EventUserSuspiciousActivity,

		// Admin Actions
		EventAdminLogin, EventAdminLogout, EventAdminSettingsChange, EventAdminUserCreate,
		EventAdminUserUpdate, EventAdminUserDelete, EventAdminUserImpersonate,
		EventAdminRoleCreate, EventAdminRoleUpdate, EventAdminRoleDelete, EventAdminRoleAssign,
		EventAdminDatabaseBackup, EventAdminDatabaseRestore, EventAdminDatabaseOptimize,
		EventAdminDatabaseVacuum, EventAdminCacheClear, EventAdminCacheFlush,
		EventAdminLogsClear, EventAdminLogsRotate, EventAdminLogsDownload,
		EventAdminEmailTemplateEdit, EventAdminEmailTest, EventAdminNotificationSend,
		EventAdminSystemRestart, EventAdminSystemShutdown, EventAdminConfigReload,
		EventAdminConfigExport, EventAdminConfigImport, EventAdminCertObtain,
		EventAdminCertRenew, EventAdminCertRevoke, EventAdminPluginInstall,
		EventAdminPluginUninstall, EventAdminPluginEnable, EventAdminPluginDisable,
		EventAdminTaskEnable, EventAdminTaskDisable, EventAdminTaskTrigger,
		EventAdminWebRobotsUpdate, EventAdminWebSecurityUpdate,

		// System Events
		EventSystemStartup, EventSystemShutdown, EventSystemRestart, EventSystemError,
		EventSystemWarning, EventSystemPanic, EventSystemConfigChange, EventSystemUpdate,
		EventSystemMaintenance, EventSystemBackupStart, EventSystemBackupFinish,
		EventSystemBackupFail, EventSystemRestoreStart, EventSystemRestoreFinish,
		EventSystemRestoreFail, EventSystemTaskStart, EventSystemTaskFinish,
		EventSystemTaskFail, EventSystemDiskLow, EventSystemMemoryHigh, EventSystemCPUHigh,
		EventSystemDBConnect, EventSystemDBDisconnect, EventSystemDBError,
		EventSystemCacheConnect, EventSystemCacheError, EventSystemSMTPConnect,
		EventSystemSMTPError, EventSystemAPIError, EventSystemRateLimit, EventSystemFirewall,
		EventSystemSSLError, EventSystemSSLExpiring, EventSystemSSLRenew,
		EventSystemTorStart, EventSystemTorStop, EventSystemTorRegenerate,
		EventSystemTorError, EventSystemGeoIPUpdate, EventSystemGeoIPError,

		// API Events
		EventAPIRequest, EventAPIRequestFailed, EventAPIAuthSuccess, EventAPIAuthFailed,
		EventAPIRateLimitHit, EventAPIKeyInvalid, EventAPIKeyExpired, EventAPIWeatherFetch,
		EventAPIEarthquakeFetch, EventAPIHurricaneFetch, EventAPIAlertsFetch, EventAPIMoonFetch,
		EventAPIHistoryFetch, EventAPILocationSearch, EventAPIGeocodeLookup, EventAPIIPLookup,
		EventAPIWebhookCreate, EventAPIWebhookUpdate, EventAPIWebhookDelete,
		EventAPIWebhookTrigger, EventAPIWebhookFail, EventAPITokenCreate, EventAPITokenRevoke,
		EventAPITokenRefresh, EventAPIBatchRequest, EventAPIExportData, EventAPIImportData,
		EventAPIInvalidRequest, EventAPIServerError, EventAPITimeout,

		// Security Events
		EventSecurityBruteForce, EventSecurityIPBlocked, EventSecurityIPUnblocked,
		EventSecuritySQLInjection, EventSecurityXSSAttempt, EventSecurityCSRFDetected,
		EventSecurityFileUpload, EventSecurityFileRejected, EventSecurityPathTraversal,
		EventSecurityCommandInject, EventSecurityPrivEscalate, EventSecurityDataLeak,
		EventSecurityUnauthorized, EventSecuritySessionHijack, EventSecurityTokenSteal,
		EventSecurityCORSViolation, EventSecurityCSPViolation, EventSecurityDDoSAttempt,
		EventSecurityBotDetected, EventSecurityScanDetected, EventSecurityPhishing,
		EventSecurityMalware, EventSecurityRansomware, EventSecurityBackdoor,
		EventSecurityCrypto, EventSecurityReverseTunnel, EventSecurityDataExfil,
		EventSecurityZeroDay, EventSecurityCompliance, EventSecurityAuditFail,
	}
}
