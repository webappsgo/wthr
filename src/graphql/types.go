package graphql

import (
	"time"
)

// =============================================================================
// 2FA/TOTP TYPES
// =============================================================================

type TOTPStatus struct {
	Enabled           bool `json:"enabled"`
	RecoveryKeysCount int  `json:"recoveryKeysCount"`
}

type TOTPSetup struct {
	Secret    string `json:"secret"`
	QrCode    string `json:"qrCode"`
	ManualURL string `json:"manualUrl"`
	Account   string `json:"account"`
	Issuer    string `json:"issuer"`
}

type TOTPRecoveryKeys struct {
	Message      string   `json:"message"`
	RecoveryKeys []string `json:"recoveryKeys"`
}

// =============================================================================
// PASSKEY (WebAuthn / FIDO2) TYPES
// =============================================================================

type UserPasskey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type PasskeyRegistrationOptions struct {
	CeremonyToken string      `json:"ceremonyToken"`
	Options       interface{} `json:"options"`
}

type PasskeyChallengeOptions struct {
	CeremonyToken string      `json:"ceremonyToken"`
	Options       interface{} `json:"options"`
}

type PasskeyRegistrationResult struct {
	Message      string       `json:"message"`
	Passkey      *UserPasskey `json:"passkey"`
	RecoveryKeys []string     `json:"recoveryKeys,omitempty"`
}

type AuthUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type AuthResult struct {
	RequiresTwoFactor    bool       `json:"requiresTwoFactor"`
	VerificationRequired bool       `json:"verificationRequired"`
	SessionToken         *string    `json:"sessionToken,omitempty"`
	Token                *string    `json:"token,omitempty"`
	User                 *AuthUser  `json:"user,omitempty"`
	ExpiresAt            *time.Time `json:"expiresAt,omitempty"`
	RemainingKeys        *int       `json:"remainingKeys,omitempty"`
}

type UserInviteValidation struct {
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type ServerInviteValidation struct {
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type UserInviteCompletion struct {
	Message *string   `json:"message,omitempty"`
	Token   *string   `json:"token,omitempty"`
	User    *AuthUser `json:"user,omitempty"`
}

type InvitedServerAdmin struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type ServerInviteCompletion struct {
	Message string             `json:"message"`
	Admin   *InvitedServerAdmin `json:"admin"`
}

// =============================================================================
// USER PREFERENCE TYPES
// =============================================================================

type UserPreference struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type UserSubscription struct {
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	Enabled bool        `json:"enabled"`
	Config  interface{} `json:"config,omitempty"`
}

// =============================================================================
// DATABASE OPERATION TYPES
// =============================================================================

type DatabaseConnectionTest struct {
	Success bool    `json:"ok"`
	Latency *int    `json:"latency,omitempty"`
	Error   *string `json:"error,omitempty"`
}

type DatabaseOptimizeResult struct {
	Success         bool     `json:"ok"`
	TablesOptimized int      `json:"tablesOptimized"`
	Duration        *float64 `json:"duration,omitempty"`
}

// =============================================================================
// BACKUP TYPES
// =============================================================================

type Backup struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	Size      int       `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
	Type      string    `json:"type"`
}

type BackupDownload struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// =============================================================================
// EMAIL TEMPLATE TYPES
// =============================================================================

type EmailTemplate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Variables []string  `json:"variables"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type EmailTemplatePreview struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// =============================================================================
// NOTIFICATION METRICS TYPES
// =============================================================================

type NotificationMetricsSummary struct {
	Total               int      `json:"total"`
	Successful          int      `json:"successful"`
	Failed              int      `json:"failed"`
	Pending             int      `json:"pending"`
	AverageDeliveryTime *float64 `json:"averageDeliveryTime,omitempty"`
}

type ChannelMetrics struct {
	Type            string   `json:"type"`
	Sent            int      `json:"sent"`
	Failed          int      `json:"failed"`
	SuccessRate     float64  `json:"successRate"`
	AvgDeliveryTime *float64 `json:"avgDeliveryTime,omitempty"`
}

type NotificationError struct {
	Timestamp  time.Time `json:"timestamp"`
	Channel    string    `json:"channel"`
	Error      string    `json:"error"`
	RetryCount *int      `json:"retryCount,omitempty"`
}

type NotificationHealth struct {
	Healthy bool     `json:"healthy"`
	Issues  []string `json:"issues"`
}

// =============================================================================
// TOR MANAGEMENT TYPES
// =============================================================================

type TorStatus struct {
	Enabled      bool    `json:"enabled"`
	Running      bool    `json:"running"`
	OnionAddress *string `json:"onionAddress,omitempty"`
	Version      *string `json:"version,omitempty"`
}

type TorHealth struct {
	Healthy       bool    `json:"healthy"`
	Uptime        *string `json:"uptime,omitempty"`
	CircuitsBuilt *int    `json:"circuitsBuilt,omitempty"`
}

type VanityGenerationStatus struct {
	Running                bool     `json:"running"`
	Pattern                *string  `json:"pattern,omitempty"`
	Progress               *float64 `json:"progress,omitempty"`
	EstimatedTimeRemaining *int     `json:"estimatedTimeRemaining,omitempty"`
}

type TorKeys struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
}

// =============================================================================
// WEB SETTINGS TYPES
// =============================================================================

type WebSettings struct {
	RobotsTxt   string `json:"robotsTxt"`
	SecurityTxt string `json:"securityTxt"`
}

// =============================================================================
// LOGS MANAGEMENT TYPES
// =============================================================================

type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Source    *string                `json:"source,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type LogStats struct {
	Total    int                    `json:"total"`
	ByLevel  map[string]interface{} `json:"byLevel"`
	BySource map[string]interface{} `json:"bySource"`
	Size     int                    `json:"size"`
}

type ArchivedLog struct {
	Filename   string    `json:"filename"`
	Size       int       `json:"size"`
	CreatedAt  time.Time `json:"createdAt"`
	Compressed bool      `json:"compressed"`
}

// =============================================================================
// SSL/TLS TYPES
// =============================================================================

type SSLStatus struct {
	Enabled     bool            `json:"enabled"`
	AutoRenew   bool            `json:"autoRenew"`
	Certificate *SSLCertificate `json:"certificate,omitempty"`
}

type SSLCertificate struct {
	Domain        string    `json:"domain"`
	Issuer        string    `json:"issuer"`
	ValidFrom     time.Time `json:"validFrom"`
	ValidUntil    time.Time `json:"validUntil"`
	DaysRemaining int       `json:"daysRemaining"`
}

type DNSRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   *int   `json:"ttl,omitempty"`
}

type SSLTestResult struct {
	Valid  bool     `json:"valid"`
	Grade  *string  `json:"grade,omitempty"`
	Issues []string `json:"issues"`
}

// =============================================================================
// METRICS CONFIGURATION TYPES
// =============================================================================

type MetricsConfig struct {
	Enabled       bool    `json:"enabled"`
	Endpoint      string  `json:"endpoint"`
	IncludeSystem bool    `json:"includeSystem"`
	Token         *string `json:"token,omitempty"`
}

type MetricDefinition struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description *string `json:"description,omitempty"`
	Enabled     bool    `json:"enabled"`
}

// =============================================================================
// LOGGING CONFIGURATION TYPES
// =============================================================================

type LogFormat struct {
	Name    string `json:"name"`
	Format  string `json:"format"`
	Enabled bool   `json:"enabled"`
}

type LogFormatInput struct {
	Name    string `json:"name"`
	Format  string `json:"format"`
	Enabled bool   `json:"enabled"`
}

type Fail2banConfig struct {
	Enabled  bool `json:"enabled"`
	MaxRetry int  `json:"maxRetry"`
	FindTime int  `json:"findTime"`
	BanTime  int  `json:"banTime"`
}

type SyslogConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// =============================================================================
// REUSE GenericResponse FROM models IF IT EXISTS, OTHERWISE DEFINE IT
// =============================================================================

// GenericResponse is used across multiple resolvers. It binds the GraphQL
// `ok` field to the Go `Success` field name (matching the `BulkResponse` and
// `ContactSubmission` convention) so resolver code can use `&GenericResponse{
// Success: true, Message: "..."}` consistently.
type GenericResponse struct {
	Success bool   `json:"ok"`
	Message string `json:"message"`
}
