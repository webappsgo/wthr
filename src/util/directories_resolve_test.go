package util

import "testing"

func noEnv(string) string { return "" }

// TestDefaultDirectories verifies the AI.md PART 4 tables for every OS and mode.
func TestDefaultDirectories(t *testing.T) {
	env := func(key string) string {
		return map[string]string{"ProgramData": `/pd`, "AppData": `/ad`, "LocalAppData": `/lad`}[key]
	}
	tests := []struct {
		name   string
		goos   string
		system bool
		want   DirectoryPaths
	}{
		{"linux system", "linux", true, DirectoryPaths{Config: "/etc/o/n", Data: "/var/lib/o/n", Cache: "/var/cache/o/n", Log: "/var/log/o/n"}},
		{"linux user", "linux", false, DirectoryPaths{Config: "/h/.config/o/n", Data: "/h/.local/share/o/n", Cache: "/h/.cache/o/n", Log: "/h/.local/log/o/n"}},
		{"bsd system", "freebsd", true, DirectoryPaths{Config: "/usr/local/etc/o/n", Data: "/var/db/o/n", Cache: "/var/cache/o/n", Log: "/var/log/o/n"}},
		{"bsd user", "openbsd", false, DirectoryPaths{Config: "/h/.config/o/n", Data: "/h/.local/share/o/n", Cache: "/h/.cache/o/n", Log: "/h/.local/log/o/n"}},
		{"darwin system", "darwin", true, DirectoryPaths{Config: "/Library/Application Support/o/n", Data: "/Library/Application Support/o/n/data", Cache: "/Library/Caches/o/n", Log: "/Library/Logs/o/n"}},
		{"darwin user", "darwin", false, DirectoryPaths{Config: "/h/Library/Application Support/o/n", Data: "/h/Library/Application Support/o/n", Cache: "/h/Library/Caches/o/n", Log: "/h/Library/Logs/o/n"}},
		{"windows system", "windows", true, DirectoryPaths{Config: "/pd/o/n", Data: "/pd/o/n/data", Cache: "/pd/o/n/cache", Log: "/pd/o/n/logs"}},
		{"windows user", "windows", false, DirectoryPaths{Config: "/ad/o/n", Data: "/lad/o/n", Cache: "/lad/o/n/cache", Log: "/lad/o/n/logs"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDirectories(tt.goos, tt.system, "/h", env, "o", "n")
			if got != tt.want {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// TestResolveBackupDir verifies the AI.md PART 8 GetBackupDir order.
func TestResolveBackupDir(t *testing.T) {
	writable := func(string) bool { return true }
	notWritable := func(string) bool { return false }
	env := func(key string) string {
		if key == "BACKUP_DIR" {
			return "/mnt/custom"
		}
		return ""
	}

	if got := ResolveBackupDir("linux", true, "/h", env, notWritable, "o", "n", "/data"); got != "/mnt/custom" {
		t.Errorf("BACKUP_DIR must win, got %q", got)
	}
	if got := ResolveBackupDir("linux", true, "/h", noEnv, writable, "o", "n", "/data"); got != "/mnt/Backups/o/n" {
		t.Errorf("a writable system backup dir must be used, got %q", got)
	}
	if got := ResolveBackupDir("linux", true, "/h", noEnv, notWritable, "o", "n", "/data"); got != "/data/backup" {
		t.Errorf("system mode must fall back inside the data dir, got %q", got)
	}
	if got := ResolveBackupDir("linux", false, "/h", noEnv, notWritable, "o", "n", "/data"); got != "/h/.local/share/Backups/o/n" {
		t.Errorf("user mode must fall back to the user backup dir, got %q", got)
	}
	if got := ResolveBackupDir("darwin", false, "/h", noEnv, notWritable, "o", "n", "/data"); got != "/h/Library/Backups/o/n" {
		t.Errorf("macOS user backup dir, got %q", got)
	}
	if got := SystemBackupDir("freebsd", noEnv, "o", "n"); got != "/var/backups/o/n" {
		t.Errorf("BSD system backup dir, got %q", got)
	}
}

// TestResolveDatabaseDir verifies DATABASE_DIR, the container default and the native default.
func TestResolveDatabaseDir(t *testing.T) {
	env := func(key string) string {
		if key == "DATABASE_DIR" {
			return "/db"
		}
		return ""
	}
	if got := ResolveDatabaseDir(env, true, "/data"); got != "/db" {
		t.Errorf("DATABASE_DIR must win, got %q", got)
	}
	if got := ResolveDatabaseDir(noEnv, true, "/data"); got != "/data/db/sqlite" {
		t.Errorf("container default, got %q", got)
	}
	if got := ResolveDatabaseDir(noEnv, false, "/data"); got != "/data/db" {
		t.Errorf("native default, got %q", got)
	}
}
