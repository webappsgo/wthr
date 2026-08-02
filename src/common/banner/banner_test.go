package banner

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout to a pipe for the duration of fn and
// returns everything written to it. PrintStartupBanner has no seam for
// mocking terminal.GetTerminalSize(), so in this Docker/CI environment
// (stdout is never a TTY) term.GetSize always errors and GetTerminalSize
// falls back to 80x24 -> SizeModeStandard -> printFull is always the path
// exercised through the public API. printCompact/printMinimal/printMicro
// are covered directly in banner_internal_test.go instead, since no
// size-mocking seam exists to reach them through PrintStartupBanner.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("pipe close error: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("pipe read error: %v", err)
	}
	return string(out)
}

func TestPrintStartupBanner(t *testing.T) {
	tests := []struct {
		name        string
		cfg         BannerConfig
		wantContain []string
		wantAbsent  []string
	}{
		{
			name: "basic config no URLs no setup",
			cfg: BannerConfig{
				AppName: "wthr",
				Version: "1.2.3",
				AppMode: "production",
			},
			wantContain: []string{"wthr 1.2.3 (production)"},
			wantAbsent:  []string{"Setup token:"},
		},
		{
			name: "config with multiple URLs",
			cfg: BannerConfig{
				AppName: "wthr",
				Version: "1.0.0",
				AppMode: "development",
				URLs: []string{
					"http://localhost:8080",
					"https://weather.example.com",
				},
			},
			wantContain: []string{
				"wthr 1.0.0 (development)",
				"http://localhost:8080",
				"https://weather.example.com",
			},
		},
		{
			name: "ShowSetup true with SetupToken shows setup section",
			cfg: BannerConfig{
				AppName:    "wthr",
				Version:    "1.0.0",
				AppMode:    "development",
				ShowSetup:  true,
				SetupToken: "abc123token",
			},
			wantContain: []string{"Setup token: abc123token"},
		},
		{
			name: "ShowSetup true but SetupToken empty omits setup section",
			cfg: BannerConfig{
				AppName:   "wthr",
				Version:   "1.0.0",
				AppMode:   "development",
				ShowSetup: true,
			},
			wantAbsent: []string{"Setup token:"},
		},
		{
			name: "empty AppName Version AppMode does not panic",
			cfg:  BannerConfig{},
			wantContain: []string{
				"┌", "└",
			},
		},
		{
			name: "many URLs boundary case",
			cfg: BannerConfig{
				AppName: "wthr",
				Version: "1.0.0",
				AppMode: "production",
				URLs: []string{
					"http://a.example.com",
					"http://b.example.com",
					"http://c.example.com",
					"http://d.example.com",
					"http://e.example.com",
					"http://f.example.com",
					"http://g.example.com",
					"http://h.example.com",
				},
			},
			wantContain: []string{
				"http://a.example.com",
				"http://h.example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				PrintStartupBanner(tt.cfg)
			})

			if out == "" {
				t.Fatalf("PrintStartupBanner() produced no output")
			}

			for _, want := range tt.wantContain {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\nfull output:\n%s", want, out)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(out, absent) {
					t.Errorf("output unexpectedly contains %q\nfull output:\n%s", absent, out)
				}
			}
		})
	}
}

// TestPrintStartupBannerLongURL documents behavior when a single URL word
// is longer than the frame width. printFull uses fmt.Printf("%-*s", ...)
// which does not truncate: a %-*s verb with a string longer than the width
// simply widens the field instead of panicking or wrapping, so the frame
// border misaligns with that line but the program does not crash.
func TestPrintStartupBannerLongURL(t *testing.T) {
	longURL := "http://" + strings.Repeat("a", 300) + ".example.com"

	cfg := BannerConfig{
		AppName: "wthr",
		Version: "1.0.0",
		AppMode: "production",
		URLs:    []string{longURL},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PrintStartupBanner panicked on long URL: %v", r)
		}
	}()

	out := captureStdout(t, func() {
		PrintStartupBanner(cfg)
	})

	if !strings.Contains(out, longURL) {
		t.Errorf("output missing long URL")
	}
}
