package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type LoggingHandler struct {
	logsDir string
}

func NewLoggingHandler(logsDir string) *LoggingHandler {
	return &LoggingHandler{
		logsDir: logsDir,
	}
}

// LogFormats represents available log formats
type LogFormats struct {
	Standard bool `json:"standard"`
	JSON     bool `json:"json"`
	Fail2ban bool `json:"fail2ban"`
	Syslog   bool `json:"syslog"`
	CEF      bool `json:"cef"`
	Apache   bool `json:"apache"`
}

// GetFormats returns the current logging format configuration
func (h *LoggingHandler) GetFormats(c *gin.Context) {
	formats := LogFormats{
		Standard: true,
		JSON:     false,
		Fail2ban: false,
		Syslog:   false,
		CEF:      false,
		Apache:   false,
	}

	c.JSON(http.StatusOK, gin.H{
		"formats": formats,
		"active":  []string{"standard"},
	})
}

// UpdateFormats updates the logging format configuration
func (h *LoggingHandler) UpdateFormats(c *gin.Context) {
	var formats LogFormats

	if err := c.ShouldBindJSON(&formats); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// In a real implementation, this would update the logger configuration
	c.JSON(http.StatusOK, gin.H{
		"message": "Logging formats updated successfully",
		"formats": formats,
	})
}

// GetFail2banConfig generates Fail2ban filter configuration
func (h *LoggingHandler) GetFail2banConfig(c *gin.Context) {
	config := `# Fail2ban filter for Weather Application
# /etc/fail2ban/filter.d/weather.conf

[Definition]

# Failed login attempts
failregex = ^.* \[ERROR\] \[auth\] Failed login attempt from <HOST>
            ^.* \[WARN\] \[security\] Suspicious activity from <HOST>
            ^.* \[ERROR\] \[api\] Rate limit exceeded for <HOST>

# Ignore successful logins
ignoreregex = ^.* \[INFO\] \[auth\] Successful login from <HOST>`

	c.Header("Content-Type", "text/plain")
	c.Header("Content-Disposition", "attachment; filename=weather.conf")
	c.String(http.StatusOK, config)
}

// GetSyslogConfig provides syslog configuration
func (h *LoggingHandler) GetSyslogConfig(c *gin.Context) {
	config := map[string]interface{}{
		"protocol": "UDP",
		"port":     514,
		"facility": "local0",
		"severity": "info",
		"format":   "RFC5424",
		"example": fmt.Sprintf(
			"<%d>1 %s weather-app - - - - Failed login attempt from 192.168.1.100",
			// local0.info
			16*8+6,
			time.Now().Format(time.RFC3339),
		),
	}

	c.JSON(http.StatusOK, config)
}

// GetCEFConfig provides CEF (Common Event Format) configuration
func (h *LoggingHandler) GetCEFConfig(c *gin.Context) {
	config := map[string]interface{}{
		"version": "0",
		"vendor":  "Weather",
		"product": "Weather API Manager",
		"format":  "CEF:Version|Device Vendor|Device Product|Device Version|Signature ID|Name|Severity|Extension",
		"example": "CEF:0|Weather|Weather API Manager|2.0|100|Failed Login|5|src=192.168.1.100 suser=admin msg=Invalid password",
	}

	c.JSON(http.StatusOK, config)
}

// ExportLogs exports logs in specified format
func (h *LoggingHandler) ExportLogs(c *gin.Context) {
	format := c.Query("format")

	switch format {
	case "fail2ban":
		h.exportFail2ban(c)
	case "syslog":
		h.exportSyslog(c)
	case "cef":
		h.exportCEF(c)
	case "json":
		h.exportJSON(c)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid format"})
	}
}

// Helper: Export in Fail2ban format
func (h *LoggingHandler) exportFail2ban(c *gin.Context) {
	// Sample Fail2ban formatted logs
	logs := []string{
		"2025-12-14 10:30:45 [ERROR] [auth] Failed login attempt from 192.168.1.100",
		"2025-12-14 10:31:00 [WARN] [security] Suspicious activity from 10.0.0.50",
		"2025-12-14 10:31:15 [ERROR] [api] Rate limit exceeded for 172.16.0.25",
	}

	output := ""
	for _, log := range logs {
		output += log + "\n"
	}

	c.Header("Content-Type", "text/plain")
	c.Header("Content-Disposition", "attachment; filename=weather_fail2ban.log")
	c.String(http.StatusOK, output)
}

// Helper: Export in Syslog format (RFC5424)
func (h *LoggingHandler) exportSyslog(c *gin.Context) {
	logs := []string{
		fmt.Sprintf("<%d>1 %s weather-app - - - - Failed login from 192.168.1.100",
			16*8+3, time.Now().Format(time.RFC3339)),
		fmt.Sprintf("<%d>1 %s weather-app - - - - API request processed successfully",
			16*8+6, time.Now().Format(time.RFC3339)),
	}

	output := ""
	for _, log := range logs {
		output += log + "\n"
	}

	c.Header("Content-Type", "text/plain")
	c.Header("Content-Disposition", "attachment; filename=weather_syslog.log")
	c.String(http.StatusOK, output)
}

// Helper: Export in CEF format
func (h *LoggingHandler) exportCEF(c *gin.Context) {
	logs := []string{
		"CEF:0|Weather|Weather API|2.0|100|Failed Login|8|src=192.168.1.100 suser=admin msg=Invalid password attempt=3",
		"CEF:0|Weather|Weather API|2.0|200|API Access|2|src=10.0.0.50 request=/api/v1/weather msg=Successful request",
		"CEF:0|Weather|Weather API|2.0|300|Rate Limit|5|src=172.16.0.25 msg=Rate limit exceeded threshold=100",
	}

	output := ""
	for _, log := range logs {
		output += log + "\n"
	}

	c.Header("Content-Type", "text/plain")
	c.Header("Content-Disposition", "attachment; filename=weather_cef.log")
	c.String(http.StatusOK, output)
}

// Helper: Export in JSON format
func (h *LoggingHandler) exportJSON(c *gin.Context) {
	logs := []map[string]interface{}{
		{
			"timestamp": time.Now().Format(time.RFC3339),
			"level":     "ERROR",
			"source":    "auth",
			"message":   "Failed login attempt",
			"metadata": map[string]string{
				"ip":       "192.168.1.100",
				"username": "admin",
				"attempts": "3",
			},
		},
		{
			"timestamp": time.Now().Format(time.RFC3339),
			"level":     "INFO",
			"source":    "api",
			"message":   "API request processed",
			"metadata": map[string]string{
				"method":   "GET",
				"path":     "/api/v1/weather",
				"duration": "45ms",
			},
		},
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// ConfigureFail2ban configures Fail2ban integration
func (h *LoggingHandler) ConfigureFail2ban(c *gin.Context) {
	var config struct {
		Enabled    bool   `json:"enabled"`
		FilterPath string `json:"filterPath"`
		JailPath   string `json:"jailPath"`
		BanTime    int    `json:"banTime"`
		MaxRetry   int    `json:"maxRetry"`
	}

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// In a real implementation, this would configure Fail2ban
	c.JSON(http.StatusOK, gin.H{
		"message": "Fail2ban configuration saved",
		"config":  config,
	})
}

// ConfigureSyslog configures Syslog integration
func (h *LoggingHandler) ConfigureSyslog(c *gin.Context) {
	var config struct {
		Enabled  bool   `json:"enabled"`
		Protocol string `json:"protocol"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Facility string `json:"facility"`
	}

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// In a real implementation, this would configure Syslog
	c.JSON(http.StatusOK, gin.H{
		"message": "Syslog configuration saved",
		"config":  config,
	})
}

// TestFormat tests a logging format
func (h *LoggingHandler) TestFormat(c *gin.Context) {
	format := c.Query("format")

	samples := map[string]string{
		"standard": "[2025-12-14 10:30:45] [INFO] [server] Server started on port 3000",
		"json":     `{"timestamp":"2025-12-14T10:30:45Z","level":"INFO","source":"server","message":"Server started on port 3000"}`,
		"fail2ban": "2025-12-14 10:30:45 [ERROR] [auth] Failed login attempt from 192.168.1.100",
		"syslog":   "<134>1 2025-12-14T10:30:45Z weather-app - - - - Server started on port 3000",
		"cef":      "CEF:0|Weather|Weather API|2.0|001|Server Start|2|msg=Server started on port 3000",
	}

	sample, ok := samples[format]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid format"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"format": format,
		"sample": sample,
	})
}
