package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webappsgo/wthr/src/config"
)

const contactWebhookTimeout = 10 * time.Second

// ValidateContactWebhookURL checks a configured endpoint before it is saved.
func ValidateContactWebhookURL(endpoint string) error {
	if strings.TrimSpace(endpoint) == "" {
		return nil
	}
	_, err := parseContactWebhookURL(endpoint, false)
	return err
}

// ContactWebhookMessage is the event delivered to a configured contact webhook.
// WebhookID and Timestamp can be set by a retrying caller so retries remain
// idempotent and retain the original event time.
type ContactWebhookMessage struct {
	Role           string    `json:"role"`
	Event          string    `json:"event"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	Severity       string    `json:"severity"`
	Timestamp      time.Time `json:"timestamp"`
	ProjectName    string    `json:"project_name"`
	ProjectVersion string    `json:"project_version"`
	AppURL         string    `json:"app_url"`
	TrackingID     string    `json:"tracking_id,omitempty"`
	WebhookID      string    `json:"-"`
	FailureRateKey string    `json:"failure_rate_key,omitempty"`
}

type contactWebhookDependencies struct {
	readSecret func(string) (string, error)
	client     *http.Client
	now        func() time.Time
	newID      func() (uuid.UUID, error)
}

func defaultContactWebhookDependencies() contactWebhookDependencies {
	return contactWebhookDependencies{
		readSecret: config.ReadSecret,
		client:     &http.Client{Timeout: contactWebhookTimeout},
		now:        time.Now,
		newID:      uuid.NewV7,
	}
}

// DispatchContactWebhook sends one contact notification using the configured
// transport and contact-role fallback chain. Errors intentionally omit the
// configured URL and secret because both may contain credentials.
func DispatchContactWebhook(ctx context.Context, contact config.ContactConfig, role, transport string, message ContactWebhookMessage) error {
	return dispatchContactWebhook(ctx, contact, role, transport, message, defaultContactWebhookDependencies())
}

// DispatchContactWebhooks sends the notification once for each configured
// transport in the role's fallback chain. It returns the successful send count
// so callers can distinguish no configured webhooks from delivery success.
func DispatchContactWebhooks(ctx context.Context, contact config.ContactConfig, role string, message ContactWebhookMessage) (int, error) {
	return dispatchContactWebhooks(ctx, contact, role, message, defaultContactWebhookDependencies())
}

func dispatchContactWebhooks(ctx context.Context, contact config.ContactConfig, role string, message ContactWebhookMessage, deps contactWebhookDependencies) (int, error) {
	transports := contactWebhookTransports(contact, role)
	sent := 0
	var failures []error
	for _, transport := range transports {
		if err := dispatchContactWebhook(ctx, contact, role, transport, message, deps); err != nil {
			failures = append(failures, fmt.Errorf("%s webhook: %w", transport, err))
			continue
		}
		sent++
	}
	return sent, errors.Join(failures...)
}

func dispatchContactWebhook(ctx context.Context, contact config.ContactConfig, role, transport string, message ContactWebhookMessage, deps contactWebhookDependencies) error {
	if ctx == nil {
		return errors.New("webhook context is required")
	}
	if deps.readSecret == nil || deps.client == nil || deps.now == nil || deps.newID == nil {
		return errors.New("webhook dispatcher is not configured")
	}
	if strings.TrimSpace(message.Event) == "" {
		return errors.New("webhook event is required")
	}
	if !validWebhookHeader(message.Event) {
		return errors.New("webhook event is invalid")
	}
	transport = strings.ToLower(strings.TrimSpace(transport))
	message.Role = role
	if message.Timestamp.IsZero() {
		message.Timestamp = deps.now().UTC()
	} else {
		message.Timestamp = message.Timestamp.UTC()
	}
	if message.WebhookID == "" {
		id, err := deps.newID()
		if err != nil {
			return errors.New("contact webhook ID generation failed")
		}
		message.WebhookID = id.String()
	}
	id, err := uuid.Parse(message.WebhookID)
	if err != nil || id.Version() != 7 {
		return errors.New("contact webhook ID must be a UUID v7")
	}

	endpoint, sourceRole := resolveContactWebhook(contact, role, transport)
	if endpoint == "" {
		return errors.New("contact webhook is not configured")
	}
	secretKey := fmt.Sprintf("server.contact.%s.webhooks.%s_secret", sourceRole, transport)
	secretValue, err := deps.readSecret(secretKey)
	if err != nil || secretValue == "" {
		return errors.New("contact webhook secret is unavailable")
	}
	secret, err := base64.RawURLEncoding.DecodeString(secretValue)
	if err != nil || len(secret) != 32 {
		return errors.New("contact webhook secret is invalid")
	}

	requestBody, contentType, requestURL, err := buildContactWebhookRequest(endpoint, transport, role, message, deps.now, deps.newID)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(requestBody))
	if err != nil {
		return errors.New("contact webhook request is invalid")
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if err := setContactWebhookHeaders(req, secret, message, deps.now); err != nil {
		return err
	}
	client := boundedContactWebhookClient(deps.client)
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("contact webhook request failed")
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("contact webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func resolveContactWebhook(contact config.ContactConfig, role, transport string) (string, string) {
	for _, candidate := range contactRoleFallback(role) {
		webhooks := contactRoleWebhooks(contact, candidate)
		if endpoint := strings.TrimSpace(webhooks[transport]); endpoint != "" {
			return endpoint, candidate
		}
	}
	return "", ""
}

func contactWebhookTransports(contact config.ContactConfig, role string) []string {
	seen := make(map[string]struct{})
	var transports []string
	for _, candidate := range contactRoleFallback(role) {
		for transport, endpoint := range contactRoleWebhooks(contact, candidate) {
			transport = strings.ToLower(strings.TrimSpace(transport))
			if strings.TrimSpace(endpoint) == "" || transport == "" {
				continue
			}
			if _, ok := seen[transport]; ok {
				continue
			}
			seen[transport] = struct{}{}
			transports = append(transports, transport)
		}
	}
	sort.Strings(transports)
	return transports
}

func contactRoleFallback(role string) []string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return []string{"admin"}
	case "security":
		return []string{"security", "admin"}
	case "abuse":
		return []string{"abuse", "general", "admin"}
	case "general":
		return []string{"general", "admin"}
	default:
		return nil
	}
}

func contactRoleWebhooks(contact config.ContactConfig, role string) map[string]string {
	switch role {
	case "admin":
		return contact.Admin.Webhooks
	case "security":
		return contact.Security.Webhooks
	case "abuse":
		return contact.Abuse.Webhooks
	case "general":
		return contact.General.Webhooks
	default:
		return nil
	}
}

func buildContactWebhookRequest(endpoint, transport, role string, message ContactWebhookMessage, now func() time.Time, newID func() (uuid.UUID, error)) ([]byte, string, string, error) {
	transport = strings.ToLower(strings.TrimSpace(transport))
	if !supportedContactWebhookTransport(transport) {
		return nil, "", "", errors.New("contact webhook transport is unsupported")
	}
	parsed, err := parseContactWebhookURL(endpoint, transport == "generic")
	if err != nil {
		return nil, "", "", err
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = now().UTC()
	} else {
		message.Timestamp = message.Timestamp.UTC()
	}
	if message.WebhookID == "" {
		id, err := newID()
		if err != nil {
			return nil, "", "", errors.New("contact webhook ID generation failed")
		}
		message.WebhookID = id.String()
	}
	id, err := uuid.Parse(message.WebhookID)
	if err != nil || id.Version() != 7 {
		return nil, "", "", errors.New("contact webhook ID must be a UUID v7")
	}
	message.Role = role

	var body []byte
	contentType := "application/json"
	switch transport {
	case "telegram":
		query := parsed.Query()
		query.Set("text", contactWebhookText(message.Subject, message.Body))
		parsed.RawQuery = query.Encode()
		body = []byte{}
		contentType = ""
	case "discord":
		body, err = json.Marshal(struct {
			Content  string `json:"content"`
			Username string `json:"username"`
		}{contactWebhookText(message.Subject, message.Body), message.ProjectName})
	case "slack", "mattermost":
		body, err = json.Marshal(struct {
			Text string `json:"text"`
		}{contactWebhookText(message.Subject, message.Body)})
	case "pushover":
		query := parsed.Query()
		values := url.Values{}
		for _, key := range []string{"token", "user"} {
			if value := query.Get(key); value != "" {
				values.Set(key, value)
			}
			query.Del(key)
		}
		if values.Get("token") == "" || values.Get("user") == "" {
			return nil, "", "", errors.New("contact webhook endpoint is invalid")
		}
		if message.Subject != "" {
			values.Set("title", message.Subject)
		}
		values.Set("message", message.Body)
		parsed.RawQuery = query.Encode()
		body = []byte(values.Encode())
		contentType = "application/x-www-form-urlencoded"
	case "gotify":
		query := parsed.Query()
		if query.Get("token") == "" {
			return nil, "", "", errors.New("contact webhook endpoint is invalid")
		}
		path := strings.TrimRight(parsed.Path, "/")
		if !strings.HasSuffix(path, "/message") {
			path += "/message"
		}
		parsed.Path = path
		body, err = json.Marshal(struct {
			Title    string `json:"title"`
			Message  string `json:"message"`
			Priority int    `json:"priority"`
		}{message.Subject, message.Body, gotifyPriority(message.Severity)})
	case "generic":
		body, err = json.Marshal(struct {
			Role           string    `json:"role"`
			Event          string    `json:"event"`
			Subject        string    `json:"subject"`
			Body           string    `json:"body"`
			Severity       string    `json:"severity"`
			Timestamp      time.Time `json:"timestamp"`
			ProjectName    string    `json:"project_name"`
			ProjectVersion string    `json:"project_version"`
			AppURL         string    `json:"app_url"`
			TrackingID     string    `json:"tracking_id,omitempty"`
		}{message.Role, message.Event, message.Subject, message.Body, message.Severity, message.Timestamp, message.ProjectName, message.ProjectVersion, message.AppURL, message.TrackingID})
	}
	if err != nil {
		return nil, "", "", errors.New("contact webhook payload could not be encoded")
	}
	return body, contentType, parsed.String(), nil
}

func parseContactWebhookURL(endpoint string, requireHTTPS bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Host == "" || u.User != nil {
		return nil, errors.New("contact webhook endpoint is invalid")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
	case "http":
		if requireHTTPS || !isLoopbackWebhookHost(u.Hostname()) {
			return nil, errors.New("contact webhook endpoint must use HTTPS")
		}
	default:
		return nil, errors.New("contact webhook endpoint is invalid")
	}
	return u, nil
}

func isLoopbackWebhookHost(host string) bool {
	return strings.EqualFold(host, "localhost") || strings.HasPrefix(host, "127.") || host == "::1"
}

func supportedContactWebhookTransport(transport string) bool {
	switch transport {
	case "telegram", "discord", "slack", "mattermost", "pushover", "gotify", "generic":
		return true
	default:
		return false
	}
}

func contactWebhookText(subject, body string) string {
	if subject == "" {
		return body
	}
	if body == "" {
		return subject
	}
	return subject + "\n\n" + body
}

func gotifyPriority(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 10
	case "error":
		return 8
	case "warn", "warning":
		return 5
	default:
		return 1
	}
}

func setContactWebhookHeaders(req *http.Request, secret []byte, message ContactWebhookMessage, now func() time.Time) error {
	if !validWebhookHeader(message.Event) || !validWebhookHeader(message.WebhookID) {
		return errors.New("contact webhook headers are invalid")
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = now().UTC()
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(reqBodyBytes(req))
	req.Header.Set("X-Webhook-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", message.Timestamp.Unix()))
	req.Header.Set("X-Webhook-ID", message.WebhookID)
	req.Header.Set("X-Webhook-Event", message.Event)
	req.Header.Set("User-Agent", contactWebhookUserAgent(message))
	return nil
}

func reqBodyBytes(req *http.Request) []byte {
	if req.Body == nil || req.Body == http.NoBody {
		return nil
	}
	return reqBodySnapshot(req)
}

func reqBodySnapshot(req *http.Request) []byte {
	if req.GetBody == nil {
		return nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil
	}
	data, readErr := io.ReadAll(body)
	closeErr := body.Close()
	if readErr != nil || closeErr != nil {
		return nil
	}
	return data
}

func contactWebhookUserAgent(message ContactWebhookMessage) string {
	name := cleanHeaderValue(message.ProjectName)
	version := cleanHeaderValue(message.ProjectVersion)
	appURL := cleanHeaderValue(message.AppURL)
	return fmt.Sprintf("%s/%s (+%s)", name, version, appURL)
}

func cleanHeaderValue(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
}

func validWebhookHeader(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func boundedContactWebhookClient(client *http.Client) *http.Client {
	copy := *client
	if copy.Timeout == 0 || copy.Timeout > contactWebhookTimeout {
		copy.Timeout = contactWebhookTimeout
	}
	if copy.CheckRedirect == nil {
		copy.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return &copy
}
