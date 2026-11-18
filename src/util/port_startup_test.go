package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePortSpec(t *testing.T) {
	cases := []struct {
		in          string
		http, https int
		ok          bool
	}{
		{"8080", 8080, 0, true},
		{" 80 , 443 ", 80, 443, true},
		{"64580", 64580, 0, true},
		{"", 0, 0, false},
		{"0", 0, 0, false},
		{"65536", 0, 0, false},
		{"abc", 0, 0, false},
		{"80,80", 0, 0, false},
		{"80,443,8443", 0, 0, false},
		{"80,", 0, 0, false},
	}
	for _, c := range cases {
		httpPort, httpsPort, err := ParsePortSpec(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ParsePortSpec(%q) err = %v, want ok=%v", c.in, err, c.ok)
			continue
		}
		if c.ok && (httpPort != c.http || httpsPort != c.https) {
			t.Errorf("ParsePortSpec(%q) = %d,%d want %d,%d", c.in, httpPort, httpsPort, c.http, c.https)
		}
	}
}

func TestResolveStartupPortsOrder(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("PORT", "")
	random, err := ResolveStartupPorts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if random.HTTP < MinPort || random.HTTP > MaxPort || random.FromConfig {
		t.Errorf("a first run must pick a random port in %d-%d, got %+v", MinPort, MaxPort, random)
	}

	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("server:\n  port: 8123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fromFile, err := ResolveStartupPorts(dir)
	if err != nil || fromFile.HTTP != 8123 || !fromFile.FromConfig {
		t.Errorf("server.yml port must win over random, got %+v, %v", fromFile, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("server:\n  port: \"8080,8443\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dual, err := ResolveStartupPorts(dir)
	if err != nil || dual.HTTP != 8080 || dual.HTTPS != 8443 {
		t.Errorf("a dual-port string must parse, got %+v, %v", dual, err)
	}

	t.Setenv("PORT", "9001")
	fromEnv, err := ResolveStartupPorts(dir)
	if err != nil || fromEnv.HTTP != 9001 || fromEnv.FromConfig {
		t.Errorf("PORT must win over server.yml, got %+v, %v", fromEnv, err)
	}
}

func TestResolveStartupPortsInvalid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORT", "nope")
	if _, err := ResolveStartupPorts(dir); err == nil {
		t.Error("an invalid PORT must be an error")
	}

	t.Setenv("PORT", "")
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("server:\n  port: 99999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveStartupPorts(dir); err == nil {
		t.Error("an out-of-range server.port must be an error")
	}
}

func TestResolveStartupPortsIgnoresUnreadableConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORT", "")
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte("server: [unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveStartupPorts(dir)
	if err != nil || got.FromConfig {
		t.Errorf("an unparsable file must fall back to a random port, got %+v, %v", got, err)
	}
}
