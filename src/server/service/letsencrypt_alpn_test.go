package service

import (
	"crypto/tls"
	"strings"
	"testing"
)

// recordingSink remembers the challenge certificates it was given.
type recordingSink struct {
	certs map[string]*tls.Certificate
}

func (s *recordingSink) SetChallengeCertificate(domain string, cert *tls.Certificate) {
	s.certs[domain] = cert
}

func (s *recordingSink) ClearChallengeCertificate(domain string) {
	delete(s.certs, domain)
}

func TestTLSALPN01ProviderPresentsAndCleansUp(t *testing.T) {
	sink := &recordingSink{certs: map[string]*tls.Certificate{}}
	provider := &TLSALPN01Provider{sink: sink}

	if err := provider.Present("example.com", "token", "token.thumbprint"); err != nil {
		t.Fatalf("Present: %v", err)
	}
	cert := sink.certs["example.com"]
	if cert == nil || len(cert.Certificate) == 0 {
		t.Fatal("Present must hand a certificate to the listener")
	}

	if err := provider.CleanUp("example.com", "token", "token.thumbprint"); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if _, left := sink.certs["example.com"]; left {
		t.Error("CleanUp must remove the challenge certificate")
	}
}

func TestSetupChallengesTLSALPNNeedsTheListener(t *testing.T) {
	service := &LetsEncryptService{}
	err := service.SetupChallenges("tls-alpn-01")
	if err == nil || !strings.Contains(err.Error(), "HTTPS listener") {
		t.Fatalf("without a sink TLS-ALPN-01 must explain it needs HTTPS, got %v", err)
	}
}
