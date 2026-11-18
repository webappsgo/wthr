package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/util"
)

// dnsValidatedRow is the app_secrets row that holds when a certificate was last issued or renewed
// through the configured DNS provider (AI.md PART 15 "validated_at").
const dnsValidatedRow = "dns_validated_at"

// dnsEnvMutex serialises the process environment changes of one provider construction.
var dnsEnvMutex sync.Mutex

// dnsSavedEnvironment is one environment variable as it was before a provider construction.
type dnsSavedEnvironment struct {
	value string
	set   bool
}

// dnsVariableName is the shape of a lego credential variable.
var dnsVariableName = regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$`)

// dnsErrorVariable finds variable names inside a lego error message.
var dnsErrorVariable = regexp.MustCompile(`[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+`)

// forbiddenDNSVariable reports an environment name a credential map must never set, because the
// process or a child process reads it for another purpose.
func forbiddenDNSVariable(name string) bool {
	for _, prefix := range []string{"LD_", "DYLD_", "SSL_CERT", "XDG_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return slices.Contains([]string{
		"PATH", "HOME", "USER", "SHELL", "TMPDIR", "TZ", "LANG", "TERM", "DOMAIN", "MODE", "DEBUG", "PORT", "LISTEN",
		"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "GODEBUG", "GOFLAGS", "GOMAXPROCS", "GOGC", "GOMEMLIMIT", "GOROOT", "GOPATH", "GOPROXY",
	}, name)
}

// DNSProviderNames returns the lego DNS-01 providers the admin page offers, sorted.
func DNSProviderNames() []string {
	names := slices.Clone(dnsProviderNames)
	sort.Strings(names)
	return names
}

// IsDNSProviderName reports whether name is a provider the server accepts.
func IsDNSProviderName(name string) bool {
	return slices.Contains(dnsProviderNames, name)
}

// DNSProviderError is a provider construction failure. It carries the credential variable NAMES
// lego complained about and never a value.
type DNSProviderError struct {
	Provider  string
	Variables []string
}

// Error names the provider and the variables to check.
func (e *DNSProviderError) Error() string {
	if len(e.Variables) == 0 {
		return fmt.Sprintf("dns provider %s: the credentials were rejected", e.Provider)
	}
	return fmt.Sprintf("dns provider %s: check the variables %s", e.Provider, strings.Join(e.Variables, ", "))
}

// variablesNamedIn returns the credential variable names lego's message mentions, dropping any
// token that equals a supplied value so a secret can never be echoed back.
func variablesNamedIn(message string, credentials map[string]string) []string {
	values := make(map[string]bool, len(credentials))
	for _, value := range credentials {
		values[value] = true
	}
	seen := map[string]bool{}
	var found []string
	for _, token := range dnsErrorVariable.FindAllString(message, -1) {
		if values[token] || seen[token] {
			continue
		}
		seen[token] = true
		found = append(found, token)
	}
	sort.Strings(found)
	return found
}

// NewDNSProvider builds the lego DNS-01 solver for a provider from its credential map (plaintext).
// The map is the provider's environment for the one constructor call, under a mutex, and the
// previous environment is restored before returning, so nothing stays in the process environment
// (AI.md PART 15 "Field discovery" and "Passing to lego"). No DNS record is touched.
func NewDNSProvider(name string, credentials map[string]string) (challenge.Provider, error) {
	if !IsDNSProviderName(name) {
		return nil, fmt.Errorf("dns provider %q is not supported", name)
	}
	for variable := range credentials {
		if !dnsVariableName.MatchString(variable) || forbiddenDNSVariable(variable) {
			return nil, fmt.Errorf("dns credential name %q is not allowed", variable)
		}
	}

	dnsEnvMutex.Lock()
	defer dnsEnvMutex.Unlock()

	previous := make(map[string]dnsSavedEnvironment, len(credentials))
	for variable, value := range credentials {
		old, wasSet := os.LookupEnv(variable)
		previous[variable] = dnsSavedEnvironment{value: old, set: wasSet}
		if err := os.Setenv(variable, value); err != nil {
			restoreDNSEnvironment(previous)
			return nil, fmt.Errorf("dns credential %s could not be applied", variable)
		}
	}
	provider, err := dns.NewDNSChallengeProviderByName(name)
	restoreDNSEnvironment(previous)
	if err != nil {
		return nil, &DNSProviderError{Provider: name, Variables: variablesNamedIn(err.Error(), credentials)}
	}
	return provider, nil
}

// restoreDNSEnvironment puts back the environment values a construction replaced.
func restoreDNSEnvironment(previous map[string]dnsSavedEnvironment) {
	for variable, old := range previous {
		if old.set {
			_ = os.Setenv(variable, old.value)
		} else {
			_ = os.Unsetenv(variable)
		}
	}
}

// SetDNSSolver gives the ACME client the lego DNS provider for the dns-01 challenge. Without one
// the manual mode applies: the TXT records are listed for the operator to publish.
func (s *LetsEncryptService) SetDNSSolver(solver challenge.Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dnsSolver = solver
}

// DNSValidatedAt returns when a certificate was last issued through the DNS provider, or false.
func DNSValidatedAt(db *sql.DB) (time.Time, bool) {
	if db == nil {
		return time.Time{}, false
	}
	var text string
	err := database.QueryRowContext(context.Background(), db, database.TimeoutSimpleSelect,
		`SELECT value FROM app_secrets WHERE name = ?`, dnsValidatedRow).Scan(&text)
	if err != nil {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// MarkDNSValidated records a successful issuance or renewal through the DNS provider.
func MarkDNSValidated(db *sql.DB) error {
	if db == nil {
		return errors.New("no database")
	}
	_, err := database.ExecContext(context.Background(), db, database.TimeoutWrite,
		`INSERT INTO app_secrets (name, value, created_at, rotated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(name) DO UPDATE SET value = excluded.value, rotated_at = CURRENT_TIMESTAMP`,
		dnsValidatedRow, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ClearDNSValidated forgets the validation after the provider or a credential changed.
func ClearDNSValidated(db *sql.DB) error {
	if db == nil {
		return nil
	}
	_, err := database.ExecContext(context.Background(), db, database.TimeoutWrite,
		`DELETE FROM app_secrets WHERE name = ?`, dnsValidatedRow)
	return err
}

// ConfiguredDNSSolver builds the dns-01 solver from the configured provider and its stored
// credentials. It returns nil with no error when no provider is configured, which leaves the
// manual TXT-record mode in place. This is the check that runs at startup and before every
// certificate request; it touches no DNS record.
func ConfiguredDNSSolver(settings config.LetsEncryptConfig, encryptionKey string) (challenge.Provider, error) {
	name := strings.TrimSpace(settings.DNSProvider)
	if name == "" {
		return nil, nil
	}
	if encryptionKey == "" {
		return nil, errors.New("no encryption key is configured")
	}
	credentials, err := util.OpenDNSCredentials(encryptionKey, settings.DNSCredentials)
	if err != nil {
		return nil, err
	}
	return NewDNSProvider(name, credentials)
}
