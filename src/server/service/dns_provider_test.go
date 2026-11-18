package service

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestDNSProviderNamesAreKnownToLego(t *testing.T) {
	if len(dnsProviderNames) < 100 {
		t.Fatalf("only %d providers listed", len(dnsProviderNames))
	}
	for _, name := range dnsProviderNames {
		_, err := NewDNSProvider(name, nil)
		var providerErr *DNSProviderError
		if err != nil && !errors.As(err, &providerErr) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNewDNSProviderNamesTheMissingVariableOnly(t *testing.T) {
	_, err := NewDNSProvider("cloudflare", nil)
	var providerErr *DNSProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected a DNSProviderError, got %v", err)
	}
	if !strings.Contains(strings.Join(providerErr.Variables, ","), "CLOUDFLARE_DNS_API_TOKEN") {
		t.Errorf("variables = %v", providerErr.Variables)
	}
}

func TestNewDNSProviderNeverEchoesAValue(t *testing.T) {
	secret := "SECRET_VALUE_THAT_MUST_NOT_LEAK"
	_, err := NewDNSProvider("cloudflare", map[string]string{"CF_API_EMAIL": secret})
	if err == nil {
		t.Fatal("an email without a key is incomplete")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("the error leaks a value: %v", err)
	}
}

func TestNewDNSProviderBuildsFromCredentialsAndRestoresTheEnvironment(t *testing.T) {
	t.Setenv("CF_DNS_API_TOKEN", "previous")
	if _, err := NewDNSProvider("cloudflare", map[string]string{"CF_DNS_API_TOKEN": "token-value"}); err != nil {
		t.Fatalf("a complete credential map builds the provider: %v", err)
	}
	if got := os.Getenv("CF_DNS_API_TOKEN"); got != "previous" {
		t.Errorf("the environment was not restored: %q", got)
	}

	if _, err := NewDNSProvider("digitalocean", map[string]string{"DO_AUTH_TOKEN": "t"}); err != nil {
		t.Fatal(err)
	}
	if _, set := os.LookupEnv("DO_AUTH_TOKEN"); set {
		t.Error("a variable that was not set before must be unset again")
	}
}

func TestNewDNSProviderRefusesUnsafeInput(t *testing.T) {
	if _, err := NewDNSProvider("manual", nil); err == nil {
		t.Error("manual waits on stdin and is not offered")
	}
	if _, err := NewDNSProvider("exec", nil); err == nil {
		t.Error("exec runs a program and is not offered")
	}
	for _, name := range []string{"PATH", "LD_PRELOAD", "lower_case", "BAD NAME", "HOME"} {
		if _, err := NewDNSProvider("cloudflare", map[string]string{name: "x"}); err == nil {
			t.Errorf("credential name %q must be refused", name)
		}
	}
}
