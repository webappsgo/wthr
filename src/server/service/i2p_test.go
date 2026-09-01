package service

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/config"
	paths "github.com/webappsgo/wthr/src/path"
)

// testI2PConfig returns a valid baseline config that individual tests mutate.
func testI2PConfig() *config.I2PConfig {
	return &config.I2PConfig{
		Enabled:          true,
		Binary:           "",
		SAMAddress:       "127.0.0.1:7656",
		VirtualPort:      80,
		InboundLength:    3,
		OutboundLength:   3,
		InboundQuantity:  5,
		OutboundQuantity: 5,
		SignatureType:    7,
		BootstrapTimeout: 300,
	}
}

// TestI2P_B32Address covers the .b32.i2p derivation against known SHA-256
// vectors: unpadded base32 of the digest, lowercased, plus the suffix.
func TestI2P_B32Address(t *testing.T) {
	tests := []struct {
		name string
		dest []byte
		want string
	}{
		{
			name: "known vector for empty destination",
			dest: []byte(""),
			want: "4oymiquy7qobjgx36tejs35zeqt24qpemsnzgtfeswmrw6csxbkq.b32.i2p",
		},
		{
			name: "known vector for test destination",
			dest: []byte("test"),
			want: "t6dnbamijr6wlgrp5kqmkwwqcwr36ty3fmfyelgrlvwblmhqbiea.b32.i2p",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b32Address(tt.dest)
			if got != tt.want {
				t.Errorf("b32Address() = %q, want %q", got, tt.want)
			}
			if strings.ContainsAny(got, "=") {
				t.Error("b32Address() must not contain base32 padding")
			}
			if got != strings.ToLower(got) {
				t.Error("b32Address() must be lowercase")
			}
			// 52 base32 chars encode a 32-byte digest without padding.
			if len(strings.TrimSuffix(got, ".b32.i2p")) != 52 {
				t.Errorf("b32 label length = %d, want 52", len(strings.TrimSuffix(got, ".b32.i2p")))
			}
		})
	}

	t.Run("distinct destinations produce distinct addresses", func(t *testing.T) {
		if b32Address([]byte("a")) == b32Address([]byte("b")) {
			t.Error("different destinations must not collide")
		}
	})
}

// TestI2P_GetI2PTunnelsConf covers the i2pd tunnel rendering, asserting that
// every configurable field reaches the generated file.
func TestI2P_GetI2PTunnelsConf(t *testing.T) {
	cfg := testI2PConfig()
	cfg.InboundLength = 2
	cfg.OutboundLength = 4
	cfg.InboundQuantity = 6
	cfg.OutboundQuantity = 8
	cfg.SignatureType = 0

	keysPath := filepath.Join("/data", "i2p", "site", "site-keys.dat")
	got := getI2PTunnelsConf(cfg, keysPath, 64123)

	wantLines := []string{
		"[site]",
		"type = server",
		"host = 127.0.0.1",
		"port = 64123",
		"keys = " + keysPath,
		"inbound.length = 2",
		"outbound.length = 4",
		"inbound.quantity = 6",
		"outbound.quantity = 8",
		"signaturetype = 0",
	}
	for _, line := range wantLines {
		if !strings.Contains(got, line) {
			t.Errorf("tunnels.conf missing %q, got:\n%s", line, got)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("tunnels.conf must end with a newline")
	}
	if !strings.HasPrefix(got, "[site]\n") {
		t.Error("tunnels.conf must start with the [site] section header")
	}
}

// TestI2P_ProviderName covers the provider display names used by the health
// response, including the unknown/zero fallback.
func TestI2P_ProviderName(t *testing.T) {
	tests := []struct {
		name     string
		provider I2PProvider
		want     string
	}{
		{"none", I2PProviderNone, "none"},
		{"i2pd", I2PProviderI2PD, "i2pd"},
		{"sam", I2PProviderSAM, "sam"},
		{"unknown falls back to none", I2PProvider(99), "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := providerName(tt.provider); got != tt.want {
				t.Errorf("providerName(%v) = %q, want %q", tt.provider, got, tt.want)
			}
			svc := &I2PService{provider: tt.provider}
			if got := svc.ProviderName(); got != tt.want {
				t.Errorf("ProviderName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestI2P_ResolveI2PDBinary covers the explicit-override branches: a configured
// path that exists is returned verbatim, and a missing one is a hard error
// rather than a silent fall-through to the search paths.
func TestI2P_ResolveI2PDBinary(t *testing.T) {
	t.Run("configured binary not found is an error", func(t *testing.T) {
		cfg := testI2PConfig()
		cfg.Binary = filepath.Join(t.TempDir(), "definitely-missing-i2pd")

		got, err := resolveI2PDBinary(cfg)
		if err == nil {
			t.Fatalf("expected an error for a missing configured binary, got %q", got)
		}
		if got != "" {
			t.Errorf("path = %q, want empty on error", got)
		}
		if !strings.Contains(err.Error(), "configured i2pd binary not found") {
			t.Errorf("error = %v, want it to mention the configured binary", err)
		}
	})

	t.Run("configured binary that exists is used verbatim", func(t *testing.T) {
		fake := filepath.Join(t.TempDir(), "i2pd")
		if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0700); err != nil {
			t.Fatalf("failed to create fake binary: %v", err)
		}
		cfg := testI2PConfig()
		cfg.Binary = fake

		got, err := resolveI2PDBinary(cfg)
		if err != nil {
			t.Fatalf("resolveI2PDBinary() = %v, want nil error", err)
		}
		if got != fake {
			t.Errorf("path = %q, want %q", got, fake)
		}
	})
}

// TestI2P_SamReachable covers the SAM bridge probe against a live listener, a
// closed port, and an empty address.
func TestI2P_SamReachable(t *testing.T) {
	t.Run("reachable against a live listener", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		if !samReachable(ln.Addr().String()) {
			t.Errorf("samReachable(%q) = false, want true", ln.Addr().String())
		}
	})

	t.Run("unreachable against a closed port", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		addr := ln.Addr().String()
		ln.Close()

		if samReachable(addr) {
			t.Errorf("samReachable(%q) = true, want false for a closed port", addr)
		}
	})

	t.Run("empty address is never reachable", func(t *testing.T) {
		if samReachable("") {
			t.Error("samReachable(\"\") = true, want false")
		}
	})
}

// TestI2P_ReadSAMReply covers SAMv3 reply parsing: successful replies, quoted
// MESSAGE fields, non-OK RESULTs, wrong message types, and truncated input.
func TestI2P_ReadSAMReply(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expect     string
		wantErr    bool
		wantErrSub string
		wantFields map[string]string
	}{
		{
			name:       "hello reply ok",
			input:      "HELLO REPLY RESULT=OK VERSION=3.3\n",
			expect:     "HELLO REPLY",
			wantFields: map[string]string{"RESULT": "OK", "VERSION": "3.3"},
		},
		{
			name:       "dest reply carries keys",
			input:      "DEST REPLY PUB=abc PRIV=def\n",
			expect:     "DEST REPLY",
			wantFields: map[string]string{"PUB": "abc", "PRIV": "def"},
		},
		{
			name:       "session status carries destination",
			input:      "SESSION STATUS RESULT=OK DESTINATION=xyz\r\n",
			expect:     "SESSION STATUS",
			wantFields: map[string]string{"RESULT": "OK", "DESTINATION": "xyz"},
		},
		{
			name:       "non-ok result with quoted message is an error",
			input:      "SESSION STATUS RESULT=DUPLICATED_ID MESSAGE=\"session id already in use\"\n",
			expect:     "SESSION STATUS",
			wantErr:    true,
			wantErrSub: "session id already in use",
		},
		{
			name:       "non-ok result without message is an error",
			input:      "STREAM STATUS RESULT=CANT_REACH_PEER\n",
			expect:     "STREAM STATUS",
			wantErr:    true,
			wantErrSub: "CANT_REACH_PEER",
		},
		{
			name:       "unexpected message type is an error",
			input:      "HELLO REPLY RESULT=OK\n",
			expect:     "SESSION STATUS",
			wantErr:    true,
			wantErrSub: "unexpected SAM reply",
		},
		{
			name:       "truncated stream is an error",
			input:      "",
			expect:     "HELLO REPLY",
			wantErr:    true,
			wantErrSub: "read SAM HELLO REPLY",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(tt.input))
			fields, err := readSAMReply(r, tt.expect)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("readSAMReply() = nil error, want error")
				}
				if tt.wantErrSub != "" && !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("error = %v, want it to contain %q", err, tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("readSAMReply() = %v, want nil error", err)
			}
			for key, want := range tt.wantFields {
				if got := fields[key]; got != want {
					t.Errorf("field %s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

// TestI2P_ParseSAMFields covers the KEY=VALUE tokenizer directly, including
// quoted values containing spaces and malformed tokens without an '='.
func TestI2P_ParseSAMFields(t *testing.T) {
	fields := parseSAMFields(` RESULT=I2P_ERROR MESSAGE="router is not ready" VERSION=3.1 garbage`)
	if fields["RESULT"] != "I2P_ERROR" {
		t.Errorf("RESULT = %q, want I2P_ERROR", fields["RESULT"])
	}
	if fields["MESSAGE"] != "router is not ready" {
		t.Errorf("MESSAGE = %q, want %q", fields["MESSAGE"], "router is not ready")
	}
	if fields["VERSION"] != "3.1" {
		t.Errorf("VERSION = %q, want 3.1", fields["VERSION"])
	}
	if _, ok := fields["garbage"]; ok {
		t.Error("tokens without '=' must be ignored")
	}
}

// TestI2P_DestinationPrefix covers Destination parsing: a minimum-size
// zero-certificate destination, a certificate with a payload, and the two
// truncation guards.
func TestI2P_DestinationPrefix(t *testing.T) {
	t.Run("null certificate destination", func(t *testing.T) {
		data := make([]byte, i2pMinDestinationLen+16)
		got, err := i2pDestinationPrefix(data)
		if err != nil {
			t.Fatalf("i2pDestinationPrefix() = %v, want nil error", err)
		}
		if len(got) != i2pMinDestinationLen {
			t.Errorf("destination length = %d, want %d", len(got), i2pMinDestinationLen)
		}
	})

	t.Run("certificate payload extends the destination", func(t *testing.T) {
		data := make([]byte, i2pMinDestinationLen+4)
		data[i2pCertHeaderOffset] = 5
		data[i2pCertHeaderOffset+1] = 0
		data[i2pCertHeaderOffset+2] = 4
		got, err := i2pDestinationPrefix(data)
		if err != nil {
			t.Fatalf("i2pDestinationPrefix() = %v, want nil error", err)
		}
		if len(got) != i2pMinDestinationLen+4 {
			t.Errorf("destination length = %d, want %d", len(got), i2pMinDestinationLen+4)
		}
	})

	t.Run("too short is an error", func(t *testing.T) {
		if _, err := i2pDestinationPrefix(make([]byte, 10)); err == nil {
			t.Error("expected an error for a short destination")
		}
	})

	t.Run("truncated certificate payload is an error", func(t *testing.T) {
		data := make([]byte, i2pMinDestinationLen)
		data[i2pCertHeaderOffset+1] = 0
		data[i2pCertHeaderOffset+2] = 8
		if _, err := i2pDestinationPrefix(data); err == nil {
			t.Error("expected an error for a truncated certificate")
		}
	})
}

// TestI2P_UpdateI2PTunnels covers the atomic tunnels.conf write: content,
// 0600 permissions, directory creation, and clobbering an existing file.
func TestI2P_UpdateI2PTunnels(t *testing.T) {
	dir := t.TempDir()
	tunnelsPath := filepath.Join(dir, "i2p", "tunnels.conf")

	if err := updateI2PTunnels(tunnelsPath, []byte("first\n")); err != nil {
		t.Fatalf("updateI2PTunnels() = %v, want nil error", err)
	}
	data, err := os.ReadFile(tunnelsPath)
	if err != nil {
		t.Fatalf("failed to read tunnels.conf: %v", err)
	}
	if string(data) != "first\n" {
		t.Errorf("content = %q, want %q", string(data), "first\n")
	}
	info, err := os.Stat(tunnelsPath)
	if err != nil {
		t.Fatalf("failed to stat tunnels.conf: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("permissions = %o, want 600", perm)
	}

	// A regenerated config must replace the previous one, leaving no temp files.
	if err := updateI2PTunnels(tunnelsPath, []byte("second\n")); err != nil {
		t.Fatalf("second updateI2PTunnels() = %v, want nil error", err)
	}
	data, err = os.ReadFile(tunnelsPath)
	if err != nil {
		t.Fatalf("failed to re-read tunnels.conf: %v", err)
	}
	if string(data) != "second\n" {
		t.Errorf("content = %q, want %q", string(data), "second\n")
	}
	entries, err := os.ReadDir(filepath.Dir(tunnelsPath))
	if err != nil {
		t.Fatalf("failed to list tunnels dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("tunnels dir has %d entries, want exactly 1 (no temp leftovers)", len(entries))
	}
}

// TestI2P_EnsureI2PDirsAt covers directory creation with 0700 permissions for
// the config, data, and site paths, and that a re-run is idempotent.
func TestI2P_EnsureI2PDirsAt(t *testing.T) {
	configDir := t.TempDir()
	dataDir := t.TempDir()

	for i := 0; i < 2; i++ {
		if err := ensureI2PDirsAt(configDir, dataDir); err != nil {
			t.Fatalf("ensureI2PDirsAt() = %v, want nil error", err)
		}
	}

	for _, dir := range []string{
		filepath.Join(configDir, "i2p"),
		filepath.Join(dataDir, "i2p"),
		filepath.Join(dataDir, "i2p", "site"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
		if perm := info.Mode().Perm(); perm != 0700 {
			t.Errorf("%s permissions = %o, want 700", dir, perm)
		}
	}
}

// TestI2P_WaitForI2PDAddress covers the bootstrap wait: an already-written
// keyfile resolves immediately, a missing one times out, and a non-positive
// timeout is rejected outright.
func TestI2P_WaitForI2PDAddress(t *testing.T) {
	t.Run("resolves an existing keyfile", func(t *testing.T) {
		keysPath := filepath.Join(t.TempDir(), "site-keys.dat")
		dest := make([]byte, i2pMinDestinationLen)
		for i := range dest {
			dest[i] = byte(i % 251)
		}
		// A null certificate (type 0, zero-length payload) keeps the Destination
		// at exactly i2pMinDestinationLen bytes.
		dest[i2pCertHeaderOffset] = 0
		dest[i2pCertHeaderOffset+1] = 0
		dest[i2pCertHeaderOffset+2] = 0
		if err := os.WriteFile(keysPath, dest, 0600); err != nil {
			t.Fatalf("failed to write keyfile: %v", err)
		}

		got, err := waitForI2PDAddress(context.Background(), keysPath, 5*time.Second)
		if err != nil {
			t.Fatalf("waitForI2PDAddress() = %v, want nil error", err)
		}
		if want := b32Address(dest); got != want {
			t.Errorf("address = %q, want %q", got, want)
		}
	})

	t.Run("times out when the keyfile never appears", func(t *testing.T) {
		keysPath := filepath.Join(t.TempDir(), "missing-keys.dat")
		_, err := waitForI2PDAddress(context.Background(), keysPath, 50*time.Millisecond)
		if err == nil {
			t.Fatal("expected a timeout error")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("error = %v, want a timeout error", err)
		}
	})

	t.Run("rejects a non-positive timeout", func(t *testing.T) {
		if _, err := waitForI2PDAddress(context.Background(), "unused", 0); err == nil {
			t.Error("expected an error for a zero timeout")
		}
	})
}

// TestI2P_LoadSAMDestinationFromDisk covers reuse of a persisted SAM
// destination, so the .b32.i2p address survives a restart, plus the
// malformed-file guard.
func TestI2P_LoadSAMDestinationFromDisk(t *testing.T) {
	t.Run("persisted destination is reused", func(t *testing.T) {
		raw := make([]byte, i2pMinDestinationLen+32)
		for i := range raw {
			raw[i] = byte(i % 241)
		}
		// A null certificate keeps the public Destination at the minimum length,
		// so the trailing 32 bytes stay private-key material.
		raw[i2pCertHeaderOffset] = 0
		raw[i2pCertHeaderOffset+1] = 0
		raw[i2pCertHeaderOffset+2] = 0
		priv := i2pBase64.EncodeToString(raw)
		keysPath := filepath.Join(t.TempDir(), "site-keys.dat")
		if err := os.WriteFile(keysPath, []byte(priv+"\n"), 0600); err != nil {
			t.Fatalf("failed to write keyfile: %v", err)
		}

		dest, err := loadOrCreateSAMDestination(nil, nil, keysPath, 7)
		if err != nil {
			t.Fatalf("loadOrCreateSAMDestination() = %v, want nil error", err)
		}
		if dest.priv != priv {
			t.Error("private destination should be returned verbatim")
		}
		if want := b32Address(raw[:i2pMinDestinationLen]); b32Address(dest.pub) != want {
			t.Errorf("address = %q, want %q", b32Address(dest.pub), want)
		}
	})

	t.Run("malformed persisted destination is an error", func(t *testing.T) {
		keysPath := filepath.Join(t.TempDir(), "site-keys.dat")
		if err := os.WriteFile(keysPath, []byte("not*valid*base64\n"), 0600); err != nil {
			t.Fatalf("failed to write keyfile: %v", err)
		}
		if _, err := loadOrCreateSAMDestination(nil, nil, keysPath, 7); err == nil {
			t.Error("expected an error for an unparseable destination")
		}
	})
}

// TestI2P_ProviderHealthy covers the health check for both models plus the
// nil/none fallbacks, using a live loopback connection for the SAM branch.
func TestI2P_ProviderHealthy(t *testing.T) {
	t.Run("nil service is unhealthy", func(t *testing.T) {
		if i2pProviderHealthy(nil) {
			t.Error("nil service must be unhealthy")
		}
	})

	t.Run("no provider is unhealthy", func(t *testing.T) {
		if i2pProviderHealthy(&I2PService{provider: I2PProviderNone}) {
			t.Error("I2PProviderNone must be unhealthy")
		}
	})

	t.Run("i2pd without a process is unhealthy", func(t *testing.T) {
		if i2pProviderHealthy(&I2PService{provider: I2PProviderI2PD}) {
			t.Error("i2pd with no process must be unhealthy")
		}
	})

	t.Run("sam without a connection is unhealthy", func(t *testing.T) {
		if i2pProviderHealthy(&I2PService{provider: I2PProviderSAM}) {
			t.Error("sam with no connection must be unhealthy")
		}
	})

	t.Run("sam with an idle open connection is healthy", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		accepted := make(chan net.Conn, 1)
		go func() {
			c, aerr := ln.Accept()
			if aerr == nil {
				accepted <- c
			}
		}()

		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		server := <-accepted
		svc := &I2PService{provider: I2PProviderSAM, samConn: conn}
		if !i2pProviderHealthy(svc) {
			t.Error("an idle open SAM connection must be healthy")
		}

		// Once the router side hangs up, the probe must report unhealthy.
		server.Close()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if !i2pProviderHealthy(svc) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Error("a closed SAM connection must be reported unhealthy")
	})
}

// TestI2P_ManagerDisabledState covers the health-facing getters on a manager
// whose config is disabled or missing: no eepsite, no provider, no uptime.
func TestI2P_ManagerDisabledState(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.I2PConfig
	}{
		{"nil config", nil},
		{"disabled config", &config.I2PConfig{Enabled: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			im := NewI2PManager(context.Background(), tt.cfg)
			if got := im.Status(); got != "disabled" {
				t.Errorf("Status() = %q, want %q", got, "disabled")
			}
			if got := im.Provider(); got != "none" {
				t.Errorf("Provider() = %q, want %q", got, "none")
			}
			if got := im.EepsiteAddress(); got != "" {
				t.Errorf("EepsiteAddress() = %q, want empty", got)
			}
			if im.IsRunning() {
				t.Error("IsRunning() = true, want false")
			}
			if got := im.BinaryPath(); got != "" {
				t.Errorf("BinaryPath() = %q, want empty", got)
			}
			if got := im.SAMAddress(); got != "" {
				t.Errorf("SAMAddress() = %q, want empty", got)
			}
			if !im.StartedAt().IsZero() {
				t.Error("StartedAt() should be the zero time when not running")
			}
			if got := im.UptimeSeconds(); got != 0 {
				t.Errorf("UptimeSeconds() = %d, want 0", got)
			}
			if err := im.Close(); err != nil {
				t.Errorf("Close() = %v, want nil", err)
			}
		})
	}
}

// TestI2P_ManagerStartDisabled covers the opt-in gate: starting a disabled
// manager returns the documented error and leaves the status as "disabled",
// not an error state.
func TestI2P_ManagerStartDisabled(t *testing.T) {
	im := NewI2PManager(context.Background(), &config.I2PConfig{Enabled: false})
	err := im.Start()
	if err == nil {
		t.Fatal("Start() on a disabled config = nil, want an error")
	}
	if !strings.Contains(err.Error(), "i2p disabled") {
		t.Errorf("error = %v, want it to mention that i2p is disabled", err)
	}
	if got := im.Status(); got != "disabled" {
		t.Errorf("Status() = %q, want %q", got, "disabled")
	}
	if im.IsRunning() {
		t.Error("IsRunning() = true, want false")
	}
}

// TestI2P_ManagerUpdateConfigToDisabled covers switching a manager to a
// disabled config: it must not attempt a start and must clear any prior error.
func TestI2P_ManagerUpdateConfigToDisabled(t *testing.T) {
	im := NewI2PManager(context.Background(), testI2PConfig())
	if err := im.UpdateConfig(&config.I2PConfig{Enabled: false}); err != nil {
		t.Fatalf("UpdateConfig() = %v, want nil error when disabling", err)
	}
	if got := im.Status(); got != "disabled" {
		t.Errorf("Status() = %q, want %q", got, "disabled")
	}
	if im.lastErr != nil {
		t.Errorf("lastErr = %v, want nil after disabling", im.lastErr)
	}

	if err := im.UpdateConfig(nil); err != nil {
		t.Fatalf("UpdateConfig(nil) = %v, want nil error", err)
	}
	if got := im.Status(); got != "disabled" {
		t.Errorf("Status() = %q, want %q", got, "disabled")
	}
}

// TestI2P_ManagerStatusError covers the "error:{short message}" branch, which
// an enabled-but-unstarted manager must report rather than "healthy".
func TestI2P_ManagerStatusError(t *testing.T) {
	t.Run("enabled but never started", func(t *testing.T) {
		im := &I2PManager{config: testI2PConfig(), ctx: context.Background()}
		got := im.Status()
		if got != "error:not started" {
			t.Errorf("Status() = %q, want %q", got, "error:not started")
		}
	})

	t.Run("last error is surfaced", func(t *testing.T) {
		im := &I2PManager{
			config:  testI2PConfig(),
			ctx:     context.Background(),
			lastErr: errI2PTest,
		}
		got := im.Status()
		if !strings.HasPrefix(got, "error:") {
			t.Fatalf("Status() = %q, want an error: prefix", got)
		}
		if !strings.Contains(got, "no provider available") {
			t.Errorf("Status() = %q, want it to include the failure reason", got)
		}
	})
}

// TestI2P_ShortError covers error condensation for the status string: nil
// input, multi-line errors, and the length cap.
func TestI2P_ShortError(t *testing.T) {
	if got := shortI2PError(nil); got != "unknown" {
		t.Errorf("shortI2PError(nil) = %q, want %q", got, "unknown")
	}

	multi := &i2pTestError{msg: "first line\nsecond line"}
	if got := shortI2PError(multi); got != "first line" {
		t.Errorf("shortI2PError(multiline) = %q, want %q", got, "first line")
	}

	long := &i2pTestError{msg: strings.Repeat("x", i2pStatusErrorMaxLen+50)}
	if got := shortI2PError(long); len(got) != i2pStatusErrorMaxLen {
		t.Errorf("shortI2PError(long) length = %d, want %d", len(got), i2pStatusErrorMaxLen)
	}
}

// i2pTestError is a minimal error used to exercise message condensation.
type i2pTestError struct {
	msg string
}

// Error returns the wrapped message.
func (e *i2pTestError) Error() string { return e.msg }

// errI2PTest stands in for a provider-resolution failure in status tests.
var errI2PTest = &i2pTestError{msg: "i2p enabled but no provider available (no i2pd binary, SAM 127.0.0.1:7656 unreachable)"}

// TestI2P_ServiceAccessors covers the I2PService field accessors and the
// close path for a service that never acquired a provider, which is the
// state every environment without i2pd or a SAM bridge ends up in.
func TestI2P_ServiceAccessors(t *testing.T) {
	started := time.Now()
	svc := &I2PService{
		provider:       I2PProviderSAM,
		eepsiteAddress: "example.b32.i2p",
		i2pBackendPort: 41234,
		binaryPath:     "",
		samAddress:     "127.0.0.1:7656",
		startedAt:      started,
	}

	if got := svc.EepsiteAddress(); got != "example.b32.i2p" {
		t.Errorf("EepsiteAddress() = %q, want %q", got, "example.b32.i2p")
	}
	if got := svc.Provider(); got != I2PProviderSAM {
		t.Errorf("Provider() = %v, want %v", got, I2PProviderSAM)
	}
	if got := svc.ProviderName(); got != "sam" {
		t.Errorf("ProviderName() = %q, want %q", got, "sam")
	}
	if got := svc.BinaryPath(); got != "" {
		t.Errorf("BinaryPath() = %q, want an empty path for Model B", got)
	}
	if got := svc.SAMAddress(); got != "127.0.0.1:7656" {
		t.Errorf("SAMAddress() = %q, want %q", got, "127.0.0.1:7656")
	}
	if got := svc.BackendPort(); got != 41234 {
		t.Errorf("BackendPort() = %d, want %d", got, 41234)
	}
	if got := svc.StartedAt(); !got.Equal(started) {
		t.Errorf("StartedAt() = %v, want %v", got, started)
	}
	if err := svc.Close(); err != nil {
		t.Errorf("Close() = %v, want nil for a service with no live provider", err)
	}
}

// TestI2P_CloseReleasesSAMConn verifies Close tears down the SAM control
// connection and clears it, so a second Close stays a no-op.
func TestI2P_CloseReleasesSAMConn(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	svc := &I2PService{provider: I2PProviderSAM, samConn: conn}
	if err := svc.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if svc.samConn != nil {
		t.Error("Close() should clear the SAM connection")
	}
	if err := svc.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}

// TestI2P_ProviderNameMapping covers the display-name mapping for every
// provider constant, including the none fallback used before a provider is
// resolved.
func TestI2P_ProviderNameMapping(t *testing.T) {
	cases := []struct {
		provider I2PProvider
		want     string
	}{
		{I2PProviderI2PD, "i2pd"},
		{I2PProviderSAM, "sam"},
		{I2PProviderNone, "none"},
	}
	for _, tc := range cases {
		if got := providerName(tc.provider); got != tc.want {
			t.Errorf("providerName(%v) = %q, want %q", tc.provider, got, tc.want)
		}
	}
}

// TestI2P_CLIStatusDisabled covers the --status output for the default opt-out
// state, where no provider is contacted and no address exists.
func TestI2P_CLIStatusDisabled(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		status, address := I2PCLIStatus(nil)
		if status != "Disabled" {
			t.Errorf("status = %q, want %q", status, "Disabled")
		}
		if address != "" {
			t.Errorf("address = %q, want empty", address)
		}
	})

	t.Run("disabled config", func(t *testing.T) {
		cfg := testI2PConfig()
		cfg.Enabled = false
		status, address := I2PCLIStatus(cfg)
		if status != "Disabled" {
			t.Errorf("status = %q, want %q", status, "Disabled")
		}
		if address != "" {
			t.Errorf("address = %q, want empty", address)
		}
	})
}

// TestI2P_TunnelsConfContent verifies the generated i2pd server-tunnel carries
// the backend port, keyfile path, and every tunnel setting from config.
func TestI2P_TunnelsConfContent(t *testing.T) {
	cfg := testI2PConfig()
	conf := getI2PTunnelsConf(cfg, "/data/i2p/site/site-keys.dat", 41234)

	for _, want := range []string{
		"[site]",
		"type = server",
		"host = 127.0.0.1",
		"port = 41234",
		"keys = /data/i2p/site/site-keys.dat",
		"inbound.length = 3",
		"outbound.length = 3",
		"inbound.quantity = 5",
		"outbound.quantity = 5",
		"signaturetype = 7",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("tunnels.conf missing %q\ngot:\n%s", want, conf)
		}
	}
}

// i2pTestDestination builds a syntactically valid Destination blob with a null
// certificate, optionally followed by private-key material.
func i2pTestDestination(seed byte, extra int) []byte {
	raw := make([]byte, i2pMinDestinationLen+extra)
	for i := range raw {
		raw[i] = seed + byte(i%127)
	}
	raw[i2pCertHeaderOffset] = 0
	raw[i2pCertHeaderOffset+1] = 0
	raw[i2pCertHeaderOffset+2] = 0
	return raw
}

// withTestPaths points the global path resolver at a temp root for the duration
// of a test, restoring the previous values afterwards.
func withTestPaths(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	p := paths.GetInstance()
	original := *p
	t.Cleanup(func() { *paths.GetInstance() = original })
	p.ConfigDir = filepath.Join(root, "config")
	p.DataDir = filepath.Join(root, "data")
	p.LogDir = filepath.Join(root, "log")
	return root
}

// TestI2P_AddressFromKeysFileEncodings covers both persisted encodings: the raw
// binary keyfile i2pd writes and the I2P base64 text the SAM path persists.
func TestI2P_AddressFromKeysFileEncodings(t *testing.T) {
	dir := t.TempDir()
	dest := i2pTestDestination(3, 0)
	want := b32Address(dest)

	binaryPath := filepath.Join(dir, "binary.dat")
	if err := os.WriteFile(binaryPath, dest, 0600); err != nil {
		t.Fatalf("write binary keyfile: %v", err)
	}
	got, err := i2pAddressFromKeysFile(binaryPath)
	if err != nil {
		t.Fatalf("i2pAddressFromKeysFile(binary) = %v", err)
	}
	if got != want {
		t.Errorf("binary keyfile address = %q, want %q", got, want)
	}

	textPath := filepath.Join(dir, "text.dat")
	priv := i2pBase64.EncodeToString(append(append([]byte{}, dest...), make([]byte, 32)...))
	if err := os.WriteFile(textPath, []byte(priv+"\n"), 0600); err != nil {
		t.Fatalf("write text keyfile: %v", err)
	}
	got, err = i2pAddressFromKeysFile(textPath)
	if err != nil {
		t.Fatalf("i2pAddressFromKeysFile(base64) = %v", err)
	}
	if got != want {
		t.Errorf("base64 keyfile address = %q, want %q", got, want)
	}

	if _, err := i2pAddressFromKeysFile(filepath.Join(dir, "missing.dat")); err == nil {
		t.Error("i2pAddressFromKeysFile() on a missing file = nil error, want an error")
	}
}

// TestI2P_IsI2PBase64 covers the discriminator that separates a SAM text
// destination from a raw binary keyfile.
func TestI2P_IsI2PBase64(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"alphanumeric", "AbC123", true},
		{"i2p specials", "aA0-~=", true},
		{"standard base64 plus", "abc+def", false},
		{"standard base64 slash", "abc/def", false},
		{"binary bytes", string([]byte{0x00, 0x01, 0x02}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isI2PBase64(tt.in); got != tt.want {
				t.Errorf("isI2PBase64(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestI2P_CLIStatusNoProvider covers the --status output when I2P is enabled
// but neither an i2pd binary nor a SAM bridge is available.
func TestI2P_CLIStatusNoProvider(t *testing.T) {
	withTestPaths(t)
	cfg := testI2PConfig()
	cfg.Binary = filepath.Join(t.TempDir(), "absent-i2pd")
	cfg.SAMAddress = "127.0.0.1:1"

	status, address := I2PCLIStatus(cfg)
	if status != "No Provider" {
		t.Errorf("status = %q, want %q", status, "No Provider")
	}
	if address != "" {
		t.Errorf("address = %q, want empty", address)
	}
}

// TestI2P_CLIStatusRunning covers the "Running ({provider})" branch, using a
// stub binary (resolveI2PDBinary only stats the path) and a persisted keyfile.
func TestI2P_CLIStatusRunning(t *testing.T) {
	withTestPaths(t)
	stub := filepath.Join(t.TempDir(), "i2pd")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatalf("write stub binary: %v", err)
	}
	cfg := testI2PConfig()
	cfg.Binary = stub

	keysPath := filepath.Join(paths.GetDataDir(), "i2p", "site", "site-keys.dat")
	if err := os.MkdirAll(filepath.Dir(keysPath), 0700); err != nil {
		t.Fatalf("create site dir: %v", err)
	}
	dest := i2pTestDestination(11, 0)
	if err := os.WriteFile(keysPath, dest, 0600); err != nil {
		t.Fatalf("write keyfile: %v", err)
	}

	status, address := I2PCLIStatus(cfg)
	if status != "Running (i2pd)" {
		t.Errorf("status = %q, want %q", status, "Running (i2pd)")
	}
	if address != b32Address(dest) {
		t.Errorf("address = %q, want %q", address, b32Address(dest))
	}

	// A provider with no persisted destination is an error, not "Running".
	if err := os.Remove(keysPath); err != nil {
		t.Fatalf("remove keyfile: %v", err)
	}
	status, address = I2PCLIStatus(cfg)
	if status != "Error" {
		t.Errorf("status = %q, want %q", status, "Error")
	}
	if address != "" {
		t.Errorf("address = %q, want empty", address)
	}
}

// TestI2P_ManagerStartNoProvider covers startDedicatedI2P's provider-resolution
// failure: enabled I2P with no i2pd binary and an unreachable SAM bridge must
// report an error instead of starting anything.
func TestI2P_ManagerStartNoProvider(t *testing.T) {
	withTestPaths(t)
	cfg := testI2PConfig()
	cfg.Binary = filepath.Join(t.TempDir(), "absent-i2pd")
	cfg.SAMAddress = "127.0.0.1:1"

	im := NewI2PManager(context.Background(), cfg)
	im.SetBackendPort(41234)
	if im.backendPort != 41234 {
		t.Errorf("backendPort = %d, want 41234", im.backendPort)
	}

	err := im.Start()
	if err == nil {
		t.Fatal("Start() with no provider = nil, want an error")
	}
	if !strings.Contains(err.Error(), "no provider available") {
		t.Errorf("error = %v, want it to mention that no provider is available", err)
	}
	if got := im.Status(); !strings.HasPrefix(got, "error:") {
		t.Errorf("Status() = %q, want an error: prefix", got)
	}
	if im.IsRunning() {
		t.Error("IsRunning() = true, want false")
	}
	if got := im.EepsiteAddress(); got != "" {
		t.Errorf("EepsiteAddress() = %q, want empty", got)
	}
}

// TestI2P_ManagerRegenerateAddress covers the destination reset: the persisted
// site directory is removed even when the subsequent restart cannot find a
// provider, so a later start generates a brand new .b32.i2p address.
func TestI2P_ManagerRegenerateAddress(t *testing.T) {
	withTestPaths(t)
	cfg := testI2PConfig()
	cfg.Binary = filepath.Join(t.TempDir(), "absent-i2pd")
	cfg.SAMAddress = "127.0.0.1:1"

	im := NewI2PManager(context.Background(), cfg)
	siteDir := filepath.Join(im.dataDir, "site")
	if err := os.MkdirAll(siteDir, 0700); err != nil {
		t.Fatalf("create site dir: %v", err)
	}
	keysPath := filepath.Join(siteDir, "site-keys.dat")
	if err := os.WriteFile(keysPath, i2pTestDestination(5, 0), 0600); err != nil {
		t.Fatalf("write keyfile: %v", err)
	}

	address, err := im.RegenerateAddress()
	if err == nil {
		t.Fatal("RegenerateAddress() = nil error, want the provider failure")
	}
	if address != "" {
		t.Errorf("address = %q, want empty on failure", address)
	}
	if _, statErr := os.Stat(siteDir); !os.IsNotExist(statErr) {
		t.Errorf("site dir still present after regenerate: %v", statErr)
	}
}

// TestI2P_UpdateTunnelsRejectsBadDir covers the tunnels.conf write failure path
// when the parent directory cannot be created.
func TestI2P_UpdateTunnelsRejectsBadDir(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "i2p")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	err := updateI2PTunnels(filepath.Join(blocker, "tunnels.conf"), []byte("[site]\n"))
	if err == nil {
		t.Fatal("updateI2PTunnels() = nil error, want a directory-creation failure")
	}
	if !strings.Contains(err.Error(), "create tunnels dir") {
		t.Errorf("error = %v, want a create-tunnels-dir failure", err)
	}
}

// TestI2P_EnsureI2PDirsUsesResolvedPaths covers the wrapper that derives the
// non-configurable I2P directories from the global path resolver.
func TestI2P_EnsureI2PDirsUsesResolvedPaths(t *testing.T) {
	withTestPaths(t)
	if err := ensureI2PDirs(); err != nil {
		t.Fatalf("ensureI2PDirs() = %v", err)
	}
	for _, dir := range []string{
		filepath.Join(paths.GetConfigDir(), "i2p"),
		filepath.Join(paths.GetDataDir(), "i2p"),
		filepath.Join(paths.GetDataDir(), "i2p", "site"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if perm := info.Mode().Perm(); perm != 0700 {
			t.Errorf("%s mode = %o, want 700", dir, perm)
		}
	}
}

// TestI2P_LoadOrCreateSAMDestinationGenerates covers the SAM DEST GENERATE path
// and the 0600 persistence of the returned private destination.
func TestI2P_LoadOrCreateSAMDestinationGenerates(t *testing.T) {
	dest := i2pTestDestination(7, 32)
	priv := i2pBase64.EncodeToString(dest)

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		reader := bufio.NewReader(server)
		if _, err := reader.ReadString('\n'); err != nil {
			return
		}
		fmt.Fprintf(server, "DEST REPLY RESULT=OK PUB=ignored PRIV=%s\n", priv)
	}()

	keysPath := filepath.Join(t.TempDir(), "site", "site-keys.dat")
	got, err := loadOrCreateSAMDestination(client, bufio.NewReader(client), keysPath, 7)
	if err != nil {
		t.Fatalf("loadOrCreateSAMDestination() = %v", err)
	}
	if got.priv != priv {
		t.Error("returned private destination does not match the SAM reply")
	}
	if b32Address(got.pub) != b32Address(dest[:i2pMinDestinationLen]) {
		t.Error("derived destination does not match the SAM reply")
	}

	info, err := os.Stat(keysPath)
	if err != nil {
		t.Fatalf("stat persisted destination: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("keyfile mode = %o, want 600", perm)
	}
	stored, err := os.ReadFile(keysPath)
	if err != nil {
		t.Fatalf("read persisted destination: %v", err)
	}
	if strings.TrimSpace(string(stored)) != priv {
		t.Error("persisted destination does not match the SAM reply")
	}
}

// TestI2P_LoadOrCreateSAMDestinationRejectsCorruptFile covers both malformed
// persisted-destination branches: invalid base64 and a truncated Destination.
func TestI2P_LoadOrCreateSAMDestinationRejectsCorruptFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"invalid base64", "not valid base64 +++", "not valid I2P base64"},
		{"truncated destination", i2pBase64.EncodeToString([]byte("short")), "malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keysPath := filepath.Join(t.TempDir(), "site-keys.dat")
			if err := os.WriteFile(keysPath, []byte(tt.content), 0600); err != nil {
				t.Fatalf("write keyfile: %v", err)
			}
			_, err := loadOrCreateSAMDestination(nil, nil, keysPath, 7)
			if err == nil {
				t.Fatal("loadOrCreateSAMDestination() = nil error, want a rejection")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}
