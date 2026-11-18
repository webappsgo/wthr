package util

import (
	"strings"
	"testing"
)

const dnsTestKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestSealAndOpenDNSCredential(t *testing.T) {
	sealed, err := SealDNSCredential(dnsTestKey, "super-secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, DNSCredentialPrefix) || strings.Contains(sealed, "super-secret-token") {
		t.Fatalf("value is not sealed: %q", sealed)
	}
	again, err := SealDNSCredential(dnsTestKey, sealed)
	if err != nil || again != sealed {
		t.Error("a sealed value must be returned unchanged")
	}
	plain, usedPrevious, err := OpenDNSCredential(dnsTestKey, sealed)
	if err != nil || plain != "super-secret-token" || usedPrevious {
		t.Errorf("open = %q, %v, %v", plain, usedPrevious, err)
	}
	plain, _, err = OpenDNSCredential(dnsTestKey, "written-by-operator")
	if err != nil || plain != "written-by-operator" {
		t.Error("a value without the prefix is plaintext an operator wrote")
	}
}

func TestSealDNSCredentialsOnlyChangesPlaintext(t *testing.T) {
	already, _ := SealDNSCredential(dnsTestKey, "kept")
	credentials := map[string]string{"CF_DNS_API_TOKEN": "plain", "CF_API_EMAIL": already, "EMPTY": ""}
	changed, err := SealDNSCredentials(dnsTestKey, credentials)
	if err != nil || !changed {
		t.Fatalf("changed = %v, err = %v", changed, err)
	}
	if !strings.HasPrefix(credentials["CF_DNS_API_TOKEN"], DNSCredentialPrefix) {
		t.Error("the plaintext value was not sealed")
	}
	if credentials["CF_API_EMAIL"] != already {
		t.Error("a sealed value must not be sealed twice")
	}
	if credentials["EMPTY"] != "" {
		t.Error("an empty value stays empty")
	}
	if changed, _ := SealDNSCredentials(dnsTestKey, credentials); changed {
		t.Error("a second pass has nothing left to seal")
	}
}

func TestOpenDNSCredentialsFailsOnWrongKey(t *testing.T) {
	sealed, _ := SealDNSCredential(dnsTestKey, "x")
	otherKey := "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU="
	if _, err := OpenDNSCredentials(otherKey, map[string]string{"A_B": sealed}); err == nil {
		t.Error("a value sealed under another key must not open")
	} else if strings.Contains(err.Error(), "x") && strings.Contains(err.Error(), sealed) {
		t.Error("the error must not echo the value")
	}
}

func TestMaskDNSCredentialsHidesEveryValue(t *testing.T) {
	masked := MaskDNSCredentials(map[string]string{"CF_DNS_API_TOKEN": "enc:v1:abc"})
	if masked["CF_DNS_API_TOKEN"] != DNSCredentialMask {
		t.Errorf("masked = %v", masked)
	}
}

func TestRebaseDNSCredentials(t *testing.T) {
	newKey := "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU="
	oldSealed, _ := SealDNSCredential(dnsTestKey, "token")
	credentials := map[string]string{"CF_DNS_API_TOKEN": oldSealed, "PLAIN_ONE": "plain"}
	changed, err := RebaseDNSCredentials(dnsTestKey, newKey, credentials)
	if err != nil || !changed {
		t.Fatalf("changed = %v, err = %v", changed, err)
	}
	plain, _, err := OpenDNSCredential(newKey, credentials["CF_DNS_API_TOKEN"])
	if err != nil || plain != "token" {
		t.Errorf("rebased value = %q, %v", plain, err)
	}
	if credentials["PLAIN_ONE"] != "plain" {
		t.Error("a plaintext value is left for the sealing pass")
	}
	if changed, _ := RebaseDNSCredentials(dnsTestKey, newKey, credentials); changed {
		t.Error("a value already under the new key is skipped")
	}
}
