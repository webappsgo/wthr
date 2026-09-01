package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/webappsgo/wthr/src/config"
	paths "github.com/webappsgo/wthr/src/path"
	"github.com/webappsgo/wthr/src/util"
)

// I2PProvider identifies which backend created the eepsite.
type I2PProvider int

const (
	// I2PProviderNone means no provider was available (I2P disabled).
	I2PProviderNone I2PProvider = iota
	// I2PProviderI2PD spawns and manages a dedicated i2pd process (Model A).
	I2PProviderI2PD
	// I2PProviderSAM uses an external SAMv3 bridge (Model B).
	I2PProviderSAM
)

const (
	// i2pMinDestinationLen is the smallest valid I2P Destination in bytes.
	i2pMinDestinationLen = 387
	// i2pCertHeaderOffset is where the certificate header starts in a Destination.
	i2pCertHeaderOffset = 384
	// i2pKeyPollInterval is how often the i2pd keyfile is polled during bootstrap.
	i2pKeyPollInterval = 2 * time.Second
	// i2pSAMDialTimeout bounds the SAM control-connection dial.
	i2pSAMDialTimeout = 5 * time.Second
	// i2pSAMProbeTimeout bounds the reachability probe of a SAM bridge.
	i2pSAMProbeTimeout = 3 * time.Second
	// i2pHealthProbeTimeout is the read deadline used to detect a dead SAM connection.
	i2pHealthProbeTimeout = 50 * time.Millisecond
	// i2pMonitorInterval is how often the provider is health-checked.
	i2pMonitorInterval = 30 * time.Second
	// i2pStatusErrorMaxLen caps the "error:{short message}" status string.
	i2pStatusErrorMaxLen = 120
)

// i2pBase64 is the I2P base64 variant used by SAM ('+' becomes '-', '/' becomes '~').
var i2pBase64 = base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-~")

// I2PService manages the I2P eepsite. Server binary owns the provider lifecycle.
type I2PService struct {
	provider I2PProvider
	// Full .b32.i2p address, derived from the persisted destination
	eepsiteAddress string
	// Dedicated plain loopback port the eepsite forwards to
	i2pBackendPort int
	// Model A: the managed i2pd process (nil for Model B)
	i2pd *exec.Cmd
	// Model B: the live SAM control connection (nil for Model A)
	samConn net.Conn
	// Model A: resolved i2pd binary path (empty for Model B)
	binaryPath string
	// Model B: SAM bridge address (empty for Model A)
	samAddress string
	// When the eepsite came up
	startedAt time.Time
	// Set by the reaper goroutine once the i2pd child process has exited
	i2pdExited atomic.Bool
}

// EepsiteAddress returns the full .b32.i2p address.
func (s *I2PService) EepsiteAddress() string { return s.eepsiteAddress }

// Provider returns the provider that created the eepsite.
func (s *I2PService) Provider() I2PProvider { return s.provider }

// ProviderName returns the provider as a display string: i2pd, sam, or none.
func (s *I2PService) ProviderName() string { return providerName(s.provider) }

// BinaryPath returns the resolved i2pd binary path (empty for Model B).
func (s *I2PService) BinaryPath() string { return s.binaryPath }

// SAMAddress returns the SAM bridge address in use (empty for Model A).
func (s *I2PService) SAMAddress() string { return s.samAddress }

// BackendPort returns the dedicated plain loopback port the eepsite forwards to.
func (s *I2PService) BackendPort() int { return s.i2pBackendPort }

// StartedAt returns the time the eepsite came up.
func (s *I2PService) StartedAt() time.Time { return s.startedAt }

// Close shuts down the provider (i2pd process or SAM session).
func (s *I2PService) Close() error {
	if s.samConn != nil {
		s.samConn.Close()
		s.samConn = nil
	}
	if s.i2pd != nil && s.i2pd.Process != nil {
		err := s.i2pd.Process.Signal(os.Interrupt)
		s.i2pd = nil
		return err
	}
	return nil
}

// providerName maps a provider to its display name.
func providerName(p I2PProvider) string {
	switch p {
	case I2PProviderI2PD:
		return "i2pd"
	case I2PProviderSAM:
		return "sam"
	default:
		return "none"
	}
}

// resolveI2PDBinary locates the i2pd executable: an explicit cfg.Binary override
// wins, then common install locations, then $PATH. An error means no i2pd is
// available and the caller should fall back to SAM (Model B).
func resolveI2PDBinary(cfg *config.I2PConfig) (string, error) {
	if cfg.Binary != "" {
		if _, err := os.Stat(cfg.Binary); err == nil {
			return cfg.Binary, nil
		}
		return "", fmt.Errorf("configured i2pd binary not found: %s", cfg.Binary)
	}
	for _, p := range []string{
		"/usr/bin/i2pd",
		"/usr/sbin/i2pd",
		"/usr/local/bin/i2pd",
		"/opt/homebrew/bin/i2pd",
	} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("i2pd"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("i2pd binary not found")
}

// samReachable reports whether a SAMv3 bridge is accepting connections at addr.
func samReachable(addr string) bool {
	if addr == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", addr, i2pSAMProbeTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ensureI2PDirs creates all I2P directories with correct permissions BEFORE any
// file is written (mirrors ensureTorDirs).
func ensureI2PDirs() error {
	return ensureI2PDirsAt(paths.GetConfigDir(), paths.GetDataDir())
}

// ensureI2PDirsAt is the testable form of ensureI2PDirs, creating the I2P
// config/data/site directories under the supplied roots with 0700 permissions.
func ensureI2PDirsAt(configDir, dataDir string) error {
	dirs := []string{
		filepath.Join(configDir, "i2p"),
		filepath.Join(dataDir, "i2p"),
		filepath.Join(dataDir, "i2p", "site"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create i2p dir %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return fmt.Errorf("chmod i2p dir %s: %w", dir, err)
		}
		// Ownership already matches this process, so a chown failure (Windows) is not fatal.
		if err := os.Chown(dir, os.Getuid(), os.Getgid()); err != nil {
			log.Printf("WARNING: could not chown i2p dir %s: %v", dir, err)
		}
	}
	return nil
}

// getI2PTunnelsConf generates the i2pd server-tunnel definition pointing the
// eepsite at the dedicated backend port, with the destination stored in keysPath.
func getI2PTunnelsConf(cfg *config.I2PConfig, keysPath string, i2pBackendPort int) string {
	return fmt.Sprintf(`[site]
type = server
host = 127.0.0.1
port = %d
keys = %s
inbound.length = %d
outbound.length = %d
inbound.quantity = %d
outbound.quantity = %d
signaturetype = %d
`, i2pBackendPort, keysPath,
		cfg.InboundLength, cfg.OutboundLength,
		cfg.InboundQuantity, cfg.OutboundQuantity, cfg.SignatureType)
}

// updateI2PTunnels writes tunnels.conf atomically with 0600 permissions. The
// file is derived state regenerated on every startup, so a clobbering write is
// always safe: the destination identity lives in site-keys.dat, not here.
func updateI2PTunnels(tunnelsPath string, content []byte) error {
	dir := filepath.Dir(tunnelsPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create tunnels dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tunnels-*.conf")
	if err != nil {
		return fmt.Errorf("create temp tunnels file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp tunnels file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp tunnels file: %w", err)
	}
	if err := os.Chmod(tmpName, 0600); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp tunnels file: %w", err)
	}
	if err := os.Rename(tmpName, tunnelsPath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("install tunnels file %s: %w", tunnelsPath, err)
	}
	return nil
}

// startI2PD writes tunnels.conf (regenerated each run) and starts a dedicated
// i2pd child process, then reads the .b32.i2p once the destination is ready.
func startI2PD(ctx context.Context, cfg *config.I2PConfig, binary, keysPath string, i2pBackendPort int, svc *I2PService) (string, error) {
	configDir := paths.GetConfigDir()
	dataDir := paths.GetDataDir()
	tunnelsPath := filepath.Join(configDir, "i2p", "tunnels.conf")

	conf := getI2PTunnelsConf(cfg, keysPath, i2pBackendPort)
	// Regenerate every startup (derived state); the identity lives in keysPath.
	if err := updateI2PTunnels(tunnelsPath, []byte(conf)); err != nil {
		return "", fmt.Errorf("failed to write tunnels.conf: %w", err)
	}
	log.Printf("Regenerated tunnels.conf at %s (backend port %d)", tunnelsPath, i2pBackendPort)

	cmd := exec.CommandContext(ctx, binary,
		"--datadir", filepath.Join(dataDir, "i2p"),
		"--tunconf", tunnelsPath,
		"--log", "file",
		"--logfile", filepath.Join(paths.GetLogDir(), "i2pd.log"),
	)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start i2pd: %w", err)
	}
	svc.i2pd = cmd
	svc.binaryPath = binary
	svc.i2pdExited.Store(false)

	// Reap the child so its exit is observable portably, without signal probing.
	go func(c *exec.Cmd, s *I2PService) {
		c.Wait()
		s.i2pdExited.Store(true)
	}(cmd, svc)

	deadline := time.Duration(cfg.BootstrapTimeout) * time.Second
	addr, err := waitForI2PDAddress(ctx, keysPath, deadline)
	if err != nil {
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt)
		}
		return "", err
	}
	return addr, nil
}

// waitForI2PDAddress polls the i2pd keyfile until the destination has been
// written, then derives the .b32.i2p address from it.
func waitForI2PDAddress(ctx context.Context, keysPath string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		return "", fmt.Errorf("invalid i2p bootstrap timeout: %s", timeout)
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(i2pKeyPollInterval)
	defer ticker.Stop()

	for {
		if addr, err := i2pAddressFromKeysFile(keysPath); err == nil {
			return addr, nil
		}
		select {
		case <-waitCtx.Done():
			return "", fmt.Errorf("timed out after %s waiting for i2pd destination at %s", timeout, keysPath)
		case <-ticker.C:
		}
	}
}

// i2pAddressFromKeysFile reads a persisted destination file and derives the
// .b32.i2p address from the Destination stored at its head. Model A (i2pd)
// writes the keyfile as raw binary while Model B (SAM) persists the same
// destination as I2P base64 text, so both encodings are accepted.
func i2pAddressFromKeysFile(keysPath string) (string, error) {
	data, err := os.ReadFile(keysPath)
	if err != nil {
		return "", err
	}
	if text := strings.TrimSpace(string(data)); isI2PBase64(text) {
		if decoded, decErr := i2pBase64.DecodeString(text); decErr == nil {
			if dest, destErr := i2pDestinationPrefix(decoded); destErr == nil {
				return b32Address(dest), nil
			}
		}
	}
	dest, err := i2pDestinationPrefix(data)
	if err != nil {
		return "", err
	}
	return b32Address(dest), nil
}

// isI2PBase64 reports whether s consists solely of I2P base64 characters, which
// distinguishes a SAM-persisted text destination from a raw binary keyfile.
func isI2PBase64(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-', r == '~', r == '=':
		default:
			return false
		}
	}
	return true
}

// i2pDestinationPrefix extracts the public Destination prefixing an i2pd keyfile
// or a SAM private-destination blob. Layout: a 256-byte encryption key, then a
// 128-byte signing key, then a certificate whose 3-byte header carries a type
// byte and a big-endian uint16 payload length.
func i2pDestinationPrefix(data []byte) ([]byte, error) {
	if len(data) < i2pMinDestinationLen {
		return nil, fmt.Errorf("i2p destination too short: %d bytes", len(data))
	}
	certLen := int(binary.BigEndian.Uint16(data[i2pCertHeaderOffset+1 : i2pCertHeaderOffset+3]))
	destLen := i2pMinDestinationLen + certLen
	if len(data) < destLen {
		return nil, fmt.Errorf("i2p destination truncated: have %d bytes, need %d", len(data), destLen)
	}
	return data[:destLen], nil
}

// samDestination pairs the binary public Destination with the base64 private
// destination string SAM expects in SESSION CREATE.
type samDestination struct {
	pub  []byte
	priv string
}

// startSAMEepsite opens a SAMv3 control connection, loads (or generates and
// persists) the destination, creates a STREAM session, and forwards incoming
// streams to the dedicated backend port. Returns the .b32.i2p address.
func startSAMEepsite(ctx context.Context, cfg *config.I2PConfig, keysPath string, i2pBackendPort int, svc *I2PService) (string, error) {
	dialer := &net.Dialer{Timeout: i2pSAMDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.SAMAddress)
	if err != nil {
		return "", fmt.Errorf("failed to dial SAM %s: %w", cfg.SAMAddress, err)
	}
	r := bufio.NewReader(conn)

	// 1. Handshake.
	if _, err := fmt.Fprintf(conn, "HELLO VERSION MIN=3.0 MAX=3.3\n"); err != nil {
		conn.Close()
		return "", err
	}
	if _, err := readSAMReply(r, "HELLO REPLY"); err != nil {
		conn.Close()
		return "", err
	}

	// 2. Load the persisted destination or generate and persist a new one.
	dest, err := loadOrCreateSAMDestination(conn, r, keysPath, cfg.SignatureType)
	if err != nil {
		conn.Close()
		return "", err
	}

	// 3. Create the STREAM session bound to that destination.
	if _, err := fmt.Fprintf(conn, "SESSION CREATE STYLE=STREAM ID=site DESTINATION=%s "+
		"inbound.length=%d outbound.length=%d inbound.quantity=%d outbound.quantity=%d\n",
		dest.priv, cfg.InboundLength, cfg.OutboundLength, cfg.InboundQuantity, cfg.OutboundQuantity); err != nil {
		conn.Close()
		return "", err
	}
	if _, err := readSAMReply(r, "SESSION STATUS"); err != nil {
		conn.Close()
		return "", err
	}

	// 4. Forward incoming eepsite streams to the backend port.
	if _, err := fmt.Fprintf(conn, "STREAM FORWARD ID=site PORT=%d HOST=127.0.0.1\n", i2pBackendPort); err != nil {
		conn.Close()
		return "", err
	}
	if _, err := readSAMReply(r, "STREAM STATUS"); err != nil {
		conn.Close()
		return "", err
	}

	// The control connection stays open for the session lifetime.
	svc.samConn = conn
	svc.samAddress = cfg.SAMAddress
	return b32Address(dest.pub), nil
}

// loadOrCreateSAMDestination returns the persisted destination from keysPath, or
// asks the router to generate one and persists it with 0600 permissions so the
// .b32.i2p address survives restarts.
func loadOrCreateSAMDestination(conn net.Conn, r *bufio.Reader, keysPath string, signatureType int) (*samDestination, error) {
	if raw, err := os.ReadFile(keysPath); err == nil {
		priv := strings.TrimSpace(string(raw))
		if priv != "" {
			decoded, decErr := i2pBase64.DecodeString(priv)
			if decErr != nil {
				return nil, fmt.Errorf("persisted i2p destination at %s is not valid I2P base64: %w", keysPath, decErr)
			}
			pub, pubErr := i2pDestinationPrefix(decoded)
			if pubErr != nil {
				return nil, fmt.Errorf("persisted i2p destination at %s is malformed: %w", keysPath, pubErr)
			}
			return &samDestination{pub: pub, priv: priv}, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read i2p destination %s: %w", keysPath, err)
	}

	if _, err := fmt.Fprintf(conn, "DEST GENERATE SIGNATURE_TYPE=%d\n", signatureType); err != nil {
		return nil, err
	}
	fields, err := readSAMReply(r, "DEST REPLY")
	if err != nil {
		return nil, err
	}
	priv := fields["PRIV"]
	if priv == "" {
		return nil, fmt.Errorf("SAM DEST REPLY did not contain a private destination")
	}
	decoded, err := i2pBase64.DecodeString(priv)
	if err != nil {
		return nil, fmt.Errorf("SAM returned an unparseable private destination: %w", err)
	}
	pub, err := i2pDestinationPrefix(decoded)
	if err != nil {
		return nil, fmt.Errorf("SAM returned a malformed destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keysPath), 0700); err != nil {
		return nil, fmt.Errorf("create i2p site dir: %w", err)
	}
	if err := os.WriteFile(keysPath, []byte(priv+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("persist i2p destination %s: %w", keysPath, err)
	}
	return &samDestination{pub: pub, priv: priv}, nil
}

// readSAMReply reads one SAMv3 reply line, verifies it is the expected message
// type, parses its KEY=VALUE fields, and turns a non-OK RESULT into an error.
func readSAMReply(r *bufio.Reader, expect string) (map[string]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		if line == "" {
			return nil, fmt.Errorf("read SAM %s: %w", expect, err)
		}
		return nil, fmt.Errorf("read SAM %s: %w (partial reply %q)", expect, err, strings.TrimSpace(line))
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, expect) {
		return nil, fmt.Errorf("unexpected SAM reply, expected %q: %s", expect, line)
	}
	fields := parseSAMFields(strings.TrimPrefix(line, expect))
	if result, ok := fields["RESULT"]; ok && result != "OK" {
		if msg := fields["MESSAGE"]; msg != "" {
			return fields, fmt.Errorf("SAM %s failed: %s (%s)", expect, result, msg)
		}
		return fields, fmt.Errorf("SAM %s failed: %s", expect, result)
	}
	return fields, nil
}

// parseSAMFields splits a SAM reply tail into KEY=VALUE pairs, honoring the
// double-quoted values SAM uses for messages containing spaces.
func parseSAMFields(tail string) map[string]string {
	fields := make(map[string]string)
	var token strings.Builder
	inQuotes := false
	flush := func() {
		part := token.String()
		token.Reset()
		if part == "" {
			return
		}
		key, value, found := strings.Cut(part, "=")
		if !found {
			return
		}
		fields[key] = strings.Trim(value, `"`)
	}
	for _, ch := range tail {
		switch {
		case ch == '"':
			inQuotes = !inQuotes
			token.WriteRune(ch)
		case ch == ' ' && !inQuotes:
			flush()
		default:
			token.WriteRune(ch)
		}
	}
	flush()
	return fields
}

// b32Address derives the .b32.i2p address: base32(sha256(destination)) without
// padding, lowercased, plus the ".b32.i2p" suffix.
func b32Address(destBinary []byte) string {
	sum := sha256.Sum256(destBinary)
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return strings.ToLower(enc.EncodeToString(sum[:])) + ".b32.i2p"
}

// startDedicatedI2P creates the eepsite when I2P is enabled AND a provider is
// available. It resolves the provider FIRST (i2pd binary, else a reachable SAM
// bridge); if neither is available it returns an error and NO backend port is
// allocated. Only after a provider is confirmed does it allocate a DEDICATED
// plain loopback listener, mapping .b32.i2p:{virtual_port} to that port.
func startDedicatedI2P(ctx context.Context, cfg *config.I2PConfig, backendPort int) (*I2PService, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, fmt.Errorf("i2p disabled (opt-in) - eepsite not started")
	}

	// Resolve the provider first, before allocating a port or writing files.
	provider := I2PProviderNone
	i2pdBinary := ""
	if b, err := resolveI2PDBinary(cfg); err == nil {
		provider, i2pdBinary = I2PProviderI2PD, b
	} else if samReachable(cfg.SAMAddress) {
		provider = I2PProviderSAM
	} else {
		return nil, fmt.Errorf("i2p enabled but no provider available (no i2pd binary, SAM %s unreachable)", cfg.SAMAddress)
	}

	if err := ensureI2PDirs(); err != nil {
		return nil, fmt.Errorf("failed to create i2p directories: %w", err)
	}

	// The eepsite forwards to the running HTTP listener when the caller knows
	// its port. With no port supplied, fall back to the spec's dedicated
	// loopback port, using the same random-unused detection as the server's own
	// port; that port is deliberately not persisted.
	i2pBackendPort := backendPort
	if i2pBackendPort <= 0 {
		allocated, err := util.GetRandomAvailablePort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate i2p backend port: %w", err)
		}
		i2pBackendPort = allocated
	}

	dataDir := paths.GetDataDir()
	// The destination key persists here and the .b32.i2p address derives from it.
	keysPath := filepath.Join(dataDir, "i2p", "site", "site-keys.dat")

	svc := &I2PService{provider: provider, i2pBackendPort: i2pBackendPort, startedAt: time.Now()}

	switch provider {
	case I2PProviderI2PD:
		addr, err := startI2PD(ctx, cfg, i2pdBinary, keysPath, i2pBackendPort, svc)
		if err != nil {
			return nil, err
		}
		svc.eepsiteAddress = addr
	case I2PProviderSAM:
		addr, err := startSAMEepsite(ctx, cfg, keysPath, i2pBackendPort, svc)
		if err != nil {
			return nil, err
		}
		svc.eepsiteAddress = addr
	}

	log.Printf("I2P eepsite started (%s): %s:%d -> 127.0.0.1:%d",
		providerName(provider), svc.eepsiteAddress, cfg.VirtualPort, i2pBackendPort)
	return svc, nil
}

// i2pProviderHealthy reports whether the provider backing the eepsite is still
// alive: Model A checks the i2pd child process, Model B probes the SAM control
// connection for an EOF or reset.
func i2pProviderHealthy(service *I2PService) bool {
	if service == nil {
		return false
	}
	switch service.provider {
	case I2PProviderI2PD:
		if service.i2pd == nil || service.i2pd.Process == nil {
			return false
		}
		return !service.i2pdExited.Load()
	case I2PProviderSAM:
		conn := service.samConn
		if conn == nil {
			return false
		}
		if err := conn.SetReadDeadline(time.Now().Add(i2pHealthProbeTimeout)); err != nil {
			return false
		}
		var probe [1]byte
		_, err := conn.Read(probe[:])
		conn.SetReadDeadline(time.Time{})
		if err == nil {
			// The router sent unsolicited data, so the session is alive.
			return true
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			// An idle but open control connection is the healthy steady state.
			return true
		}
		return !errors.Is(err, io.EOF)
	default:
		return false
	}
}

// I2PManager handles all I2P lifecycle operations. It stores NO backend port:
// the dedicated port is allocated inside startDedicatedI2P, only when I2P is
// enabled AND a provider is available.
type I2PManager struct {
	mu          sync.Mutex
	service     *I2PService
	config      *config.I2PConfig
	dataDir     string
	ctx         context.Context
	lastErr     error
	backendPort int
}

// NewI2PManager creates a new I2P manager with the given configuration and
// starts its background health monitor, which restarts an unresponsive
// eepsite the same way the Tor hidden service is kept running - I2P is
// opt-in, but once enabled it must never treat a provider crash as fatal to
// the server (AI.md PART 32).
func NewI2PManager(ctx context.Context, cfg *config.I2PConfig) *I2PManager {
	im := &I2PManager{
		config:  cfg,
		dataDir: filepath.Join(paths.GetDataDir(), "i2p"),
		ctx:     ctx,
	}
	go im.monitorLoop(ctx)
	return im
}

// monitorLoop periodically health-checks a running eepsite and restarts it
// if it has died, until ctx is cancelled (process shutdown). Disabled or
// not-yet-started managers are simply skipped each tick - no provider means
// nothing to monitor.
func (im *I2PManager) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(i2pMonitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			im.mu.Lock()
			needsRestart := im.config != nil && im.config.Enabled &&
				im.service != nil && !i2pProviderHealthy(im.service)
			if needsRestart {
				log.Println("I2P provider unresponsive, restarting...")
				im.service.Close()
				im.service = nil
				if err := im.startLocked(); err != nil {
					log.Printf("Failed to restart I2P: %v", err)
				}
			}
			im.mu.Unlock()
		}
	}
}

// SetBackendPort points the eepsite tunnel at an already-running HTTP listener.
// A zero port keeps the dedicated random loopback port behavior.
func (im *I2PManager) SetBackendPort(port int) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.backendPort = port
}

// Start initializes the eepsite if I2P is enabled and a provider is available.
func (im *I2PManager) Start() error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.startLocked()
}

// startLocked starts the provider; callers must already hold im.mu.
func (im *I2PManager) startLocked() error {
	service, err := startDedicatedI2P(im.ctx, im.config, im.backendPort)
	if err != nil {
		im.lastErr = err
		return err
	}
	im.service = service
	im.lastErr = nil
	return nil
}

// UpdateConfig applies new settings and restarts I2P. startDedicatedI2P
// regenerates tunnels.conf with a freshly allocated backend port from the new
// config, so no separate tunnels.conf write is needed here.
func (im *I2PManager) UpdateConfig(cfg *config.I2PConfig) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.config = cfg
	if im.service != nil {
		im.service.Close()
		im.service = nil
	}
	// A config that disables I2P leaves the eepsite down, respecting opt-in.
	if cfg == nil || !cfg.Enabled {
		im.lastErr = nil
		return nil
	}
	return im.startLocked()
}

// RegenerateAddress creates a new random .b32.i2p destination.
func (im *I2PManager) RegenerateAddress() (string, error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		im.service.Close()
		im.service = nil
	}
	if err := os.RemoveAll(filepath.Join(im.dataDir, "site")); err != nil {
		return "", fmt.Errorf("failed to remove old i2p keys: %w", err)
	}
	if err := im.startLocked(); err != nil {
		return "", err
	}
	return im.service.EepsiteAddress(), nil
}

// EepsiteAddress returns the current .b32.i2p address (empty if not running).
func (im *I2PManager) EepsiteAddress() string {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		return im.service.EepsiteAddress()
	}
	return ""
}

// I2PCLIStatus reports the eepsite state for the `--status` CLI output
// (AI.md PART 32.2 CLI table) without starting anything.
// The returned status is "Running ({provider})", "Disabled", "No Provider", or
// "Error"; address is the persisted .b32.i2p address, empty when none exists.
func I2PCLIStatus(cfg *config.I2PConfig) (string, string) {
	if cfg == nil || !cfg.Enabled {
		return "Disabled", ""
	}

	provider := ""
	if _, err := resolveI2PDBinary(cfg); err == nil {
		provider = providerName(I2PProviderI2PD)
	} else if samReachable(cfg.SAMAddress) {
		provider = providerName(I2PProviderSAM)
	}

	keysPath := filepath.Join(paths.GetDataDir(), "i2p", "site", "site-keys.dat")
	address, addrErr := i2pAddressFromKeysFile(keysPath)

	if provider == "" {
		return "No Provider", address
	}
	if addrErr != nil || address == "" {
		return "Error", ""
	}
	return fmt.Sprintf("Running (%s)", provider), address
}

// IsRunning reports whether an eepsite is currently up.
func (im *I2PManager) IsRunning() bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.service != nil && i2pProviderHealthy(im.service)
}

// Status returns the health-response status string: "disabled" when I2P is not
// opted in, "healthy" when the eepsite is up, or "error:{short message}".
func (im *I2PManager) Status() string {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.config == nil || !im.config.Enabled {
		return "disabled"
	}
	if im.service != nil && i2pProviderHealthy(im.service) {
		return "healthy"
	}
	if im.lastErr != nil {
		return "error:" + shortI2PError(im.lastErr)
	}
	if im.service == nil {
		return "error:not started"
	}
	return "error:provider unresponsive"
}

// Provider returns the active provider name: i2pd, sam, or none.
func (im *I2PManager) Provider() string {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		return providerName(im.service.provider)
	}
	return providerName(I2PProviderNone)
}

// BinaryPath returns the resolved i2pd binary path (empty unless Model A runs).
func (im *I2PManager) BinaryPath() string {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		return im.service.binaryPath
	}
	return ""
}

// SAMAddress returns the SAM bridge address in use (empty unless Model B runs).
func (im *I2PManager) SAMAddress() string {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		return im.service.samAddress
	}
	return ""
}

// StartedAt returns when the eepsite came up (zero time when not running).
func (im *I2PManager) StartedAt() time.Time {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		return im.service.startedAt
	}
	return time.Time{}
}

// UptimeSeconds returns how long the eepsite has been up, 0 when not running.
func (im *I2PManager) UptimeSeconds() int64 {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service == nil || im.service.startedAt.IsZero() {
		return 0
	}
	return int64(time.Since(im.service.startedAt).Seconds())
}

// Close shuts down the provider.
func (im *I2PManager) Close() error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		err := im.service.Close()
		im.service = nil
		return err
	}
	return nil
}

// shortI2PError condenses an error into the single short line the health
// response's "error:{short message}" form expects.
func shortI2PError(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := strings.TrimSpace(strings.SplitN(err.Error(), "\n", 2)[0])
	if msg == "" {
		return "unknown"
	}
	if len(msg) > i2pStatusErrorMaxLen {
		return msg[:i2pStatusErrorMaxLen]
	}
	return msg
}

// I2P config validation lives solely in config.ValidateI2PConfig (src/config/config.go)
// per AI.md PART 32.2 — that is the version production code (admin_i2p.go, config.go's
// own save path) actually calls. A duplicate validator used to live here but only its
// own test ever called it, so it was removed to eliminate the drift risk of two
// independently-maintained copies of the same validation rules.
