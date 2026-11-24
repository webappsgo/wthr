package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/webappsgo/wthr/src/config"
)

var contactWebhookTestID = uuid.MustParse("018f47a0-0000-7000-8000-000000000001")
var contactWebhookTestSecret = base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func contactWebhookTestDeps(client *http.Client, readSecret func(string) (string, error)) contactWebhookDependencies {
	return contactWebhookDependencies{
		readSecret: readSecret,
		client:     client,
		now:        func() time.Time { return time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC) },
		newID:      func() (uuid.UUID, error) { return contactWebhookTestID, nil },
	}
}

func TestDispatchContactWebhookAdaptersAndSignature(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		endpoint  func(string) string
		verify    func(*testing.T, *http.Request, []byte)
	}{
		{
			name: "telegram", transport: "telegram", endpoint: func(base string) string { return base + "?chat_id=123" },
			verify: func(t *testing.T, r *http.Request, body []byte) {
				if r.Method != http.MethodPost || r.URL.Query().Get("chat_id") != "123" || r.URL.Query().Get("text") != "Forecast\n\nRain expected" {
					t.Errorf("telegram request = %s %s body %q", r.Method, r.URL.String(), body)
				}
				if len(body) != 0 {
					t.Errorf("telegram body = %q, want empty body", body)
				}
			},
		},
		{
			name: "discord", transport: "discord", endpoint: func(base string) string { return base },
			verify: func(t *testing.T, _ *http.Request, body []byte) {
				var got map[string]string
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				if got["content"] != "Forecast\n\nRain expected" || got["username"] != "Weather" {
					t.Errorf("discord payload = %#v", got)
				}
			},
		},
		{
			name: "slack", transport: "slack", endpoint: func(base string) string { return base },
			verify: func(t *testing.T, _ *http.Request, body []byte) {
				var got map[string]string
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				if got["text"] != "Forecast\n\nRain expected" {
					t.Errorf("slack payload = %#v", got)
				}
			},
		},
		{
			name: "mattermost", transport: "mattermost", endpoint: func(base string) string { return base },
			verify: func(t *testing.T, _ *http.Request, body []byte) {
				var got map[string]string
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				if got["text"] != "Forecast\n\nRain expected" {
					t.Errorf("mattermost payload = %#v", got)
				}
			},
		},
		{
			name: "pushover", transport: "pushover", endpoint: func(base string) string { return base + "?token=app-token&user=user-key" },
			verify: func(t *testing.T, r *http.Request, body []byte) {
				if r.URL.Query().Has("token") || r.URL.Query().Has("user") {
					t.Errorf("Pushover credentials leaked into request URL: %s", r.URL)
				}
				form, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatal(err)
				}
				if form.Get("token") != "app-token" || form.Get("user") != "user-key" || form.Get("title") != "Forecast" || form.Get("message") != "Rain expected" {
					t.Errorf("pushover form = %#v", form)
				}
				if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Errorf("content type = %q", r.Header.Get("Content-Type"))
				}
				_ = body
			},
		},
		{
			name: "gotify", transport: "gotify", endpoint: func(base string) string { return base + "?token=gotify-token" },
			verify: func(t *testing.T, r *http.Request, body []byte) {
				if r.URL.Path != "/message" || r.URL.Query().Get("token") != "gotify-token" {
					t.Errorf("gotify endpoint = %s", r.URL)
				}
				var got map[string]any
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				if got["title"] != "Forecast" || got["message"] != "Rain expected" || got["priority"] != float64(8) {
					t.Errorf("gotify payload = %#v", got)
				}
			},
		},
		{
			name: "generic", transport: "generic", endpoint: func(base string) string { return base },
			verify: func(t *testing.T, _ *http.Request, body []byte) {
				var got map[string]any
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatal(err)
				}
				for key, want := range map[string]string{
					"role": "security", "event": "weather.alert", "subject": "Forecast",
					"body": "Rain expected", "severity": "error", "project_name": "Weather",
					"project_version": "1.2.3", "app_url": "https://weather.example", "tracking_id": "track-1",
				} {
					if got[key] != want {
						t.Errorf("generic %s = %#v, want %q", key, got[key], want)
					}
				}
				if got["timestamp"] != "2026-01-02T03:04:05Z" {
					t.Errorf("generic timestamp = %#v", got["timestamp"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured []byte
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				captured, err = io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				tt.verify(t, r, captured)
				if got := r.Header.Get("X-Webhook-Signature"); got != signatureForBody(contactWebhookTestSecret, captured) {
					t.Errorf("signature = %q, want signature for exact request body", got)
				}
				if got := r.Header.Get("X-Webhook-Timestamp"); got != "1767323045" {
					t.Errorf("timestamp header = %q", got)
				}
				if got := r.Header.Get("X-Webhook-ID"); got != contactWebhookTestID.String() {
					t.Errorf("webhook ID = %q", got)
				}
				if got := r.Header.Get("X-Webhook-Event"); got != "weather.alert" {
					t.Errorf("event header = %q", got)
				}
				if got := r.Header.Get("User-Agent"); got != "Weather/1.2.3 (+https://weather.example)" {
					t.Errorf("User-Agent = %q", got)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			contact := config.ContactConfig{Security: config.ContactRoleConfig{Webhooks: map[string]string{tt.transport: tt.endpoint(server.URL)}}}
			message := ContactWebhookMessage{
				Event: "weather.alert", Subject: "Forecast", Body: "Rain expected", Severity: "error",
				ProjectName: "Weather", ProjectVersion: "1.2.3", AppURL: "https://weather.example", TrackingID: "track-1",
			}
			secretsRead := 0
			deps := contactWebhookTestDeps(server.Client(), func(key string) (string, error) {
				secretsRead++
				if key != "server.contact.security.webhooks."+tt.transport+"_secret" {
					t.Errorf("secret key = %q", key)
				}
				return contactWebhookTestSecret, nil
			})
			if err := dispatchContactWebhook(context.Background(), contact, "security", tt.transport, message, deps); err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if secretsRead != 1 {
				t.Errorf("secret reads = %d, want 1", secretsRead)
			}
		})
	}
}

func signatureForBody(encodedSecret string, body []byte) string {
	secret, _ := base64.RawURLEncoding.DecodeString(encodedSecret)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestDispatchContactWebhookFallbackUsesSourceRoleSecret(t *testing.T) {
	var calls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	contact := config.ContactConfig{
		Security: config.ContactRoleConfig{Webhooks: map[string]string{}},
		Admin:    config.ContactRoleConfig{Webhooks: map[string]string{"slack": server.URL}},
	}
	var gotKey string
	deps := contactWebhookTestDeps(server.Client(), func(key string) (string, error) {
		gotKey = key
		return contactWebhookTestSecret, nil
	})
	if err := dispatchContactWebhook(context.Background(), contact, "security", "slack", ContactWebhookMessage{Event: "security.report_received"}, deps); err != nil {
		t.Fatalf("dispatch through fallback: %v", err)
	}
	if gotKey != "server.contact.admin.webhooks.slack_secret" {
		t.Errorf("secret key = %q, want fallback role secret", gotKey)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestDispatchContactWebhooksDiscoversAndDeduplicatesFallbackTransports(t *testing.T) {
	var got []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	contact := config.ContactConfig{
		Abuse:   config.ContactRoleConfig{Webhooks: map[string]string{"slack": server.URL + "/abuse", "generic": server.URL + "/generic-abuse"}},
		General: config.ContactRoleConfig{Webhooks: map[string]string{"slack": server.URL + "/general", "mattermost": server.URL + "/mattermost"}},
		Admin:   config.ContactRoleConfig{Webhooks: map[string]string{"gotify": server.URL + "/admin?token=x"}},
	}
	var secretKeys []string
	deps := contactWebhookTestDeps(server.Client(), func(key string) (string, error) {
		secretKeys = append(secretKeys, key)
		return contactWebhookTestSecret, nil
	})
	count, err := dispatchContactWebhooks(context.Background(), contact, "abuse", ContactWebhookMessage{Event: "abuse.report"}, deps)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if count != 4 {
		t.Fatalf("successful sends = %d, want 4", count)
	}
	if len(got) != 4 {
		t.Fatalf("requests = %v, want four transports", got)
	}
	if containsString(got, "/general") {
		t.Errorf("same transport should use the nearest role endpoint, got %v", got)
	}
	if !containsString(secretKeys, "server.contact.general.webhooks.mattermost_secret") || !containsString(secretKeys, "server.contact.admin.webhooks.gotify_secret") {
		t.Errorf("fallback secret keys = %v", secretKeys)
	}
}

func TestDispatchContactWebhooksNoConfigurationAndPartialFailure(t *testing.T) {
	count, err := dispatchContactWebhooks(context.Background(), config.ContactConfig{}, "security", ContactWebhookMessage{Event: "security.report"}, defaultContactWebhookDependencies())
	if count != 0 || err != nil {
		t.Fatalf("empty dispatch = (%d, %v), want (0, nil)", count, err)
	}

	good := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer good.Close()
	bad := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "receiver details", http.StatusInternalServerError)
	}))
	defer bad.Close()
	contact := config.ContactConfig{Security: config.ContactRoleConfig{Webhooks: map[string]string{"slack": good.URL, "generic": bad.URL}}}
	deps := contactWebhookTestDeps(good.Client(), func(string) (string, error) { return contactWebhookTestSecret, nil })
	deps.client = &http.Client{Transport: &roundTripperMux{
		defaultRoundTripper: good.Client().Transport,
		other:               bad.Client().Transport,
	}}
	count, err = dispatchContactWebhooks(context.Background(), contact, "security", ContactWebhookMessage{Event: "security.report"}, deps)
	if count != 1 || err == nil {
		t.Fatalf("partial dispatch = (%d, %v), want one success and an error", count, err)
	}
	for _, sensitive := range []string{good.URL, bad.URL, "receiver details", "token="} {
		if strings.Contains(err.Error(), sensitive) {
			t.Errorf("error contains sensitive endpoint/response data %q: %v", sensitive, err)
		}
	}
}

type roundTripperMux struct {
	defaultRoundTripper http.RoundTripper
	other               http.RoundTripper
}

func (rt *roundTripperMux) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, "generic") {
		return rt.other.RoundTrip(req)
	}
	return rt.defaultRoundTripper.RoundTrip(req)
}

func TestDispatchContactWebhookRedactsSecretAndNetworkErrors(t *testing.T) {
	contact := config.ContactConfig{Admin: config.ContactRoleConfig{Webhooks: map[string]string{"generic": "https://secret-host.example/hook?token=super-secret"}}}
	deps := contactWebhookTestDeps(&http.Client{Transport: rejectingRoundTripper{}}, func(key string) (string, error) {
		if !strings.Contains(key, "admin.webhooks.generic_secret") {
			t.Errorf("unexpected secret key %q", key)
		}
		return contactWebhookTestSecret, nil
	})
	err := dispatchContactWebhook(context.Background(), contact, "admin", "generic", ContactWebhookMessage{Event: "alert"}, deps)
	if err == nil || err.Error() != "contact webhook request failed" {
		t.Fatalf("network error = %v", err)
	}
	for _, sensitive := range []string{"secret-host", "super-secret"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Errorf("error leaked %q: %v", sensitive, err)
		}
	}
}

type rejectingRoundTripper struct{}

func (rejectingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("request failed at https://secret-host.example/hook?token=super-secret")
}

func TestContactWebhookValidationAndBoundedClient(t *testing.T) {
	for _, endpoint := range []string{"http://external.example/hook", "file:///tmp/hook", "https://user:pass@example.com/hook"} {
		if _, err := parseContactWebhookURL(endpoint, true); err == nil {
			t.Errorf("parseContactWebhookURL(%q) succeeded", endpoint)
		}
	}
	if _, _, _, err := buildContactWebhookRequest("https://example.com/hook", "unsupported", "admin", ContactWebhookMessage{Event: "alert"}, time.Now, uuid.NewV7); err == nil {
		t.Fatal("unsupported transport succeeded")
	}

	client := boundedContactWebhookClient(&http.Client{})
	if client.Timeout != contactWebhookTimeout {
		t.Errorf("default timeout = %s, want %s", client.Timeout, contactWebhookTimeout)
	}
	client = boundedContactWebhookClient(&http.Client{Timeout: time.Minute})
	if client.Timeout != contactWebhookTimeout {
		t.Errorf("long timeout = %s, want cap %s", client.Timeout, contactWebhookTimeout)
	}
	client = boundedContactWebhookClient(&http.Client{Timeout: time.Second})
	if client.Timeout != time.Second {
		t.Errorf("short timeout = %s, want preserved", client.Timeout)
	}

}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
