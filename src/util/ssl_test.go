package util

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTestCert writes a self-signed certificate valid for names between notBefore and
// notAfter to certPath and keyPath.
func writeTestCert(t *testing.T, certPath, keyPath string, names []string, notBefore, notAfter time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: names[0]},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     names,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
}

// useSystemDir points the certbot directory at a temporary one for the test.
func useSystemDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	previous := systemLetsEncryptDir
	systemLetsEncryptDir = dir
	t.Cleanup(func() { systemLetsEncryptDir = previous })
	return dir
}

func validWindow() (time.Time, time.Time) {
	return time.Now().Add(-time.Hour), time.Now().Add(90 * 24 * time.Hour)
}

func TestCertDirNameNeutralizesHostileValues(t *testing.T) {
	cases := map[string]string{
		"api.example.com": "api.example.com",
		"*.example.com":   "_wildcard_.example.com",
		"../../etc":       "_invalid_",
		"a/b":             "a_b",
		"2001:db8::1":     "2001_db8__1",
		"":                "_invalid_",
	}
	for input, want := range cases {
		if got := CertDirName(input); got != want {
			t.Errorf("CertDirName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFindCertificateFollowsLookupOrder(t *testing.T) {
	system := useSystemDir(t)
	ssl := t.TempDir()
	notBefore, notAfter := validWindow()
	host := "api.example.com"

	if found, _ := FindCertificate(ssl, host, time.Now()); found != nil {
		t.Fatalf("nothing exists yet, got %+v", found)
	}

	writeTestCert(t, filepath.Join(ssl, "local", host, "cert.pem"), filepath.Join(ssl, "local", host, "key.pem"), []string{host}, notBefore, notAfter)
	found, _ := FindCertificate(ssl, host, time.Now())
	if found == nil || found.Owner != CertOwnerUser {
		t.Fatalf("only a local certificate exists, got %+v", found)
	}

	writeTestCert(t, filepath.Join(ssl, "letsencrypt", host, "fullchain.pem"), filepath.Join(ssl, "letsencrypt", host, "privkey.pem"), []string{host}, notBefore, notAfter)
	found, _ = FindCertificate(ssl, host, time.Now())
	if found == nil || found.Owner != CertOwnerApp {
		t.Fatalf("an app-managed certificate must beat a local one, got %+v", found)
	}

	writeTestCert(t, filepath.Join(system, host, "fullchain.pem"), filepath.Join(system, host, "privkey.pem"), []string{host}, notBefore, notAfter)
	found, _ = FindCertificate(ssl, host, time.Now())
	if found == nil || found.Owner != CertOwnerSystem || !strings.Contains(found.CertPath, filepath.Join(system, host)) {
		t.Fatalf("a certbot certificate named for the host must beat the app one, got %+v", found)
	}

	writeTestCert(t, filepath.Join(system, "domain", "fullchain.pem"), filepath.Join(system, "domain", "privkey.pem"), []string{host}, notBefore, notAfter)
	found, _ = FindCertificate(ssl, host, time.Now())
	if found == nil || !strings.Contains(found.CertPath, filepath.Join(system, "domain")) {
		t.Fatalf("the literal domain directory comes first, got %+v", found)
	}
}

func TestFindCertificateRejectsUnusableCertificates(t *testing.T) {
	system := useSystemDir(t)
	ssl := t.TempDir()
	host := "api.example.com"
	now := time.Now()

	writeTestCert(t, filepath.Join(system, "domain", "fullchain.pem"), filepath.Join(system, "domain", "privkey.pem"), []string{"other.example.org"}, now.Add(-time.Hour), now.Add(time.Hour))
	writeTestCert(t, filepath.Join(system, host, "fullchain.pem"), filepath.Join(system, host, "privkey.pem"), []string{host}, now.Add(-48*time.Hour), now.Add(-time.Hour))
	writeTestCert(t, filepath.Join(ssl, "letsencrypt", host, "fullchain.pem"), filepath.Join(ssl, "letsencrypt", host, "privkey.pem"), []string{host}, now.Add(time.Hour), now.Add(48*time.Hour))
	if err := os.MkdirAll(filepath.Join(ssl, "local", host), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ssl, "local", host, "cert.pem"), []byte("not a certificate"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, skipped := FindCertificate(ssl, host, now)
	if found != nil {
		t.Fatalf("every candidate is unusable, got %+v", found)
	}
	if len(skipped) != 4 {
		t.Fatalf("want 4 skip reasons, got %d: %v", len(skipped), skipped)
	}
	joined := strings.Join(skipped, "\n")
	for _, want := range []string{"does not match", "expired", "not valid before", "cannot read"} {
		if !strings.Contains(joined, want) {
			t.Errorf("skip reasons miss %q:\n%s", want, joined)
		}
	}
}

func TestParseTLSMinVersion(t *testing.T) {
	if ParseTLSMinVersion("TLS1.3") != tls.VersionTLS13 || ParseTLSMinVersion(" tls 1.3 ") != tls.VersionTLS13 {
		t.Error("TLS1.3 must map to TLS 1.3")
	}
	for _, value := range []string{"", "TLS1.2", "bogus", "SSL3"} {
		if ParseTLSMinVersion(value) != tls.VersionTLS12 {
			t.Errorf("%q must fall back to TLS 1.2", value)
		}
	}
}

func TestNewSSLManagerStartsDisabled(t *testing.T) {
	mgr := NewSSLManager(t.TempDir())
	if mgr.IsHTTPSEnabled() {
		t.Error("a fresh manager must not claim HTTPS")
	}
	if _, err := mgr.GetTLSConfig("TLS1.2"); err == nil {
		t.Error("GetTLSConfig on a disabled manager must fail")
	}
	if info := mgr.GetCertInfo(); info["enabled"] != false || info["status"] != "none" {
		t.Errorf("GetCertInfo = %v", info)
	}
	if mgr.CheckRenewal() {
		t.Error("nothing loaded, nothing to renew")
	}
	if err := mgr.Reload(); err == nil {
		t.Error("Reload with nothing loaded must fail")
	}
}

func TestCheckExistingCertsLoadsTheFoundCertificate(t *testing.T) {
	useSystemDir(t)
	ssl := t.TempDir()
	notBefore, notAfter := validWindow()
	host := "api.example.com"
	mgr := NewSSLManager(ssl)

	found, err := mgr.CheckExistingCerts(host)
	if err != nil || found {
		t.Fatalf("no certificate: found=%v err=%v", found, err)
	}

	writeTestCert(t, filepath.Join(ssl, "letsencrypt", host, "fullchain.pem"), filepath.Join(ssl, "letsencrypt", host, "privkey.pem"), []string{host}, notBefore, notAfter)
	found, err = mgr.CheckExistingCerts(host)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if !mgr.IsHTTPSEnabled() || mgr.Owner() != CertOwnerApp {
		t.Errorf("enabled=%v owner=%q", mgr.IsHTTPSEnabled(), mgr.Owner())
	}
	if mgr.CheckRenewal() {
		t.Error("a 90 day certificate is not due for renewal")
	}
}

func TestCheckRenewalOnlyForAppCertificatesWithinSevenDays(t *testing.T) {
	useSystemDir(t)
	ssl := t.TempDir()
	host := "api.example.com"
	soon := time.Now().Add(3 * 24 * time.Hour)
	before := time.Now().Add(-time.Hour)

	writeTestCert(t, filepath.Join(ssl, "letsencrypt", host, "fullchain.pem"), filepath.Join(ssl, "letsencrypt", host, "privkey.pem"), []string{host}, before, soon)
	app := NewSSLManager(ssl)
	if ok, err := app.CheckExistingCerts(host); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !app.CheckRenewal() {
		t.Error("an app certificate 3 days from expiry must be renewed")
	}

	other := t.TempDir()
	writeTestCert(t, filepath.Join(other, "local", host, "cert.pem"), filepath.Join(other, "local", host, "key.pem"), []string{host}, before, soon)
	user := NewSSLManager(other)
	if ok, err := user.CheckExistingCerts(host); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if user.CheckRenewal() {
		t.Error("a user certificate is never renewed by the app")
	}
}

func TestReloadPicksUpARenewedCertificate(t *testing.T) {
	useSystemDir(t)
	ssl := t.TempDir()
	host := "api.example.com"
	certPath := filepath.Join(ssl, "letsencrypt", host, "fullchain.pem")
	keyPath := filepath.Join(ssl, "letsencrypt", host, "privkey.pem")
	now := time.Now()

	writeTestCert(t, certPath, keyPath, []string{host}, now.Add(-time.Hour), now.Add(2*24*time.Hour))
	mgr := NewSSLManager(ssl)
	if ok, err := mgr.CheckExistingCerts(host); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !mgr.CheckRenewal() {
		t.Fatal("setup: the first certificate must be due")
	}

	writeTestCert(t, certPath, keyPath, []string{host}, now.Add(-time.Hour), now.Add(80*24*time.Hour))
	if err := mgr.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if mgr.CheckRenewal() {
		t.Error("after the reload the new 80 day certificate must be served")
	}
	cert, err := mgr.GetCertificate(&tls.ClientHelloInfo{ServerName: host})
	if err != nil || cert.Leaf.NotAfter.Before(now.Add(60*24*time.Hour)) {
		t.Errorf("GetCertificate must return the renewed certificate: %v", err)
	}
}

func TestGenerateSelfSignedCert(t *testing.T) {
	useSystemDir(t)
	ssl := t.TempDir()
	mgr := NewSSLManager(ssl)

	if err := mgr.GenerateSelfSignedCert("example.com"); err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}
	certPath, keyPath := mgr.GetCertPaths()
	if certPath != filepath.Join(ssl, "local", "example.com", "cert.pem") || keyPath != filepath.Join(ssl, "local", "example.com", "key.pem") {
		t.Errorf("paths = %q, %q", certPath, keyPath)
	}
	info, err := os.Stat(keyPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the private key must be 0600: %v %v", info, err)
	}
	if mgr.Owner() != CertOwnerUser || !mgr.IsHTTPSEnabled() {
		t.Errorf("owner=%q enabled=%v", mgr.Owner(), mgr.IsHTTPSEnabled())
	}

	status := mgr.GetCertInfo()
	if status["status"] != "valid" || status["enabled"] != true || status["issuer"] != "Self-Signed" {
		t.Errorf("GetCertInfo = %v", status)
	}
	if days, ok := status["days_remaining"].(int); !ok || days < 360 {
		t.Errorf("days_remaining = %v", status["days_remaining"])
	}

	again := NewSSLManager(ssl)
	if ok, err := again.CheckExistingCerts("example.com"); err != nil || !ok {
		t.Errorf("a generated certificate must be found on the next start: ok=%v err=%v", ok, err)
	}
}

func TestGenerateSelfSignedCertForAnIPAddress(t *testing.T) {
	useSystemDir(t)
	mgr := NewSSLManager(t.TempDir())
	if err := mgr.GenerateSelfSignedCert("203.0.113.9"); err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}
	if ok, err := mgr.CheckExistingCerts("203.0.113.9"); err != nil || !ok {
		t.Errorf("an IP host must validate against its IP SAN: ok=%v err=%v", ok, err)
	}
}

func TestGetTLSConfigServesTheCertificateAndHonorsMinVersion(t *testing.T) {
	useSystemDir(t)
	mgr := NewSSLManager(t.TempDir())
	if err := mgr.GenerateSelfSignedCert("example.com"); err != nil {
		t.Fatal(err)
	}
	config, err := mgr.GetTLSConfig("TLS1.3")
	if err != nil {
		t.Fatal(err)
	}
	if config.MinVersion != tls.VersionTLS13 || config.GetCertificate == nil {
		t.Errorf("config = %+v", config)
	}
	hasH2 := false
	for _, proto := range config.NextProtos {
		if proto == "h2" {
			hasH2 = true
		}
	}
	if !hasH2 {
		t.Errorf("h2 must be offered, NextProtos = %v", config.NextProtos)
	}
}

func TestGetCertificateAnswersTLSALPNChallenges(t *testing.T) {
	useSystemDir(t)
	mgr := NewSSLManager(t.TempDir())
	if err := mgr.GenerateSelfSignedCert("example.com"); err != nil {
		t.Fatal(err)
	}
	challenge := &tls.Certificate{Certificate: [][]byte{{1, 2, 3}}}

	hello := &tls.ClientHelloInfo{ServerName: "Example.com", SupportedProtos: []string{"acme-tls/1"}}
	if _, err := mgr.GetCertificate(hello); err == nil {
		t.Error("a validation handshake without a pending challenge must fail")
	}

	mgr.SetChallengeCertificate("example.com", challenge)
	got, err := mgr.GetCertificate(hello)
	if err != nil || got != challenge {
		t.Errorf("the challenge certificate must be served: %v", err)
	}

	normal, err := mgr.GetCertificate(&tls.ClientHelloInfo{ServerName: "example.com", SupportedProtos: []string{"h2"}})
	if err != nil || normal == challenge {
		t.Error("an ordinary handshake must never receive the challenge certificate")
	}

	mgr.ClearChallengeCertificate("example.com")
	if _, err := mgr.GetCertificate(hello); err == nil {
		t.Error("a cleared challenge must stop being served")
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present.txt")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !fileExists(present) {
		t.Error("fileExists() = false for a present file, want true")
	}
	if fileExists(filepath.Join(dir, "absent.txt")) {
		t.Error("fileExists() = true for an absent file, want false")
	}
}
