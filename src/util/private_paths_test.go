package util

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTreeFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// TestEnforcePrivatePathsFixesAWorldReadableTree builds the tree at 0755/0644 and asserts
// every private path ends 0700/0600, tor/site and server.yml included (AI.md PART 8).
func TestEnforcePrivatePathsFixesAWorldReadableTree(t *testing.T) {
	config := t.TempDir()
	data := t.TempDir()

	files := []string{
		filepath.Join(config, "server.yml"),
		filepath.Join(config, "yaml.d", "10-extra.yml"),
		filepath.Join(config, "ssl", "local", "host", "cert.pem"),
		filepath.Join(config, "tor", "torrc"),
		filepath.Join(config, "i2p", "i2pd.conf"),
		filepath.Join(data, "security", "geoip", "country.mmdb"),
		filepath.Join(data, "tor", "site", "hs_ed25519_secret_key"),
	}
	for _, file := range files {
		writeTreeFile(t, file, 0o644)
	}
	for _, dir := range []string{
		filepath.Join(config, "yaml.d"), filepath.Join(config, "ssl"), filepath.Join(config, "tor"),
		filepath.Join(config, "i2p"), filepath.Join(data, "security"), filepath.Join(data, "tor"),
		filepath.Join(data, "tor", "site"),
	} {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := EnforcePrivatePaths(config, data); err != nil {
		t.Fatalf("EnforcePrivatePaths: %v", err)
	}

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o, want 600", file, info.Mode().Perm())
		}
	}
	for _, dir := range []string{
		filepath.Join(config, "yaml.d"), filepath.Join(config, "ssl", "local"), filepath.Join(config, "tor"),
		filepath.Join(config, "i2p"), filepath.Join(data, "security", "geoip"), filepath.Join(data, "tor", "site"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("%s mode = %o, want 700", dir, info.Mode().Perm())
		}
	}
}

func TestEnforcePrivatePathsLeavesOtherPathsAlone(t *testing.T) {
	config := t.TempDir()
	data := t.TempDir()
	other := filepath.Join(data, "db", "server.db")
	writeTreeFile(t, other, 0o644)

	if err := EnforcePrivatePaths(config, data); err != nil {
		t.Fatalf("EnforcePrivatePaths: %v", err)
	}
	info, err := os.Stat(other)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("a non-private path was changed to %o", info.Mode().Perm())
	}
}

func TestEnforcePrivatePathsDoesNotFollowSymlinks(t *testing.T) {
	config := t.TempDir()
	data := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeTreeFile(t, outside, 0o644)
	if err := os.MkdirAll(filepath.Join(config, "tor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(config, "tor", "link")); err != nil {
		t.Fatal(err)
	}

	if err := EnforcePrivatePaths(config, data); err != nil {
		t.Fatalf("EnforcePrivatePaths: %v", err)
	}
	info, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("a file behind a symlink was changed to %o", info.Mode().Perm())
	}
}
