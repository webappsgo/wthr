package banner

import (
	"strings"
	"testing"
)

// TestPrintCompact directly exercises the compact banner renderer (60-79
// column terminals), which is unreachable through the public
// PrintStartupBanner API in this test environment because
// terminal.GetTerminalSize() always falls back to 80x24 (SizeModeStandard)
// when stdout is not a TTY.
func TestPrintCompact(t *testing.T) {
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
			wantContain: []string{"=== wthr 1.2.3 (production) ==="},
			wantAbsent:  []string{"Setup:"},
		},
		{
			name: "config with URLs",
			cfg: BannerConfig{
				AppName: "wthr",
				Version: "1.0.0",
				AppMode: "development",
				URLs:    []string{"http://localhost:8080"},
			},
			wantContain: []string{"http://localhost:8080"},
		},
		{
			name: "ShowSetup true with SetupToken",
			cfg: BannerConfig{
				AppName:    "wthr",
				Version:    "1.0.0",
				AppMode:    "development",
				ShowSetup:  true,
				SetupToken: "abc123token",
			},
			wantContain: []string{"Setup: abc123token"},
		},
		{
			name: "ShowSetup true but SetupToken empty omits setup line",
			cfg: BannerConfig{
				AppName:   "wthr",
				Version:   "1.0.0",
				AppMode:   "development",
				ShowSetup: true,
			},
			wantAbsent: []string{"Setup:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				printCompact(tt.cfg)
			})

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

// TestPrintMinimal directly exercises the minimal banner renderer (40-59
// column terminals) — see TestPrintCompact for why direct invocation is
// necessary rather than going through the public API.
func TestPrintMinimal(t *testing.T) {
	cfg := BannerConfig{
		AppName: "wthr",
		Version: "1.2.3",
		URLs:    []string{"http://localhost:8080", "https://weather.example.com"},
	}

	out := captureStdout(t, func() {
		printMinimal(cfg)
	})

	for _, want := range []string{"wthr 1.2.3", "http://localhost:8080", "https://weather.example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

// TestPrintMicro directly exercises the micro banner renderer (<40 column
// terminals) — see TestPrintCompact for why direct invocation is necessary
// rather than going through the public API.
func TestPrintMicro(t *testing.T) {
	cfg := BannerConfig{AppName: "wthr"}

	out := captureStdout(t, func() {
		printMicro(cfg)
	})

	if strings.TrimSpace(out) != "wthr" {
		t.Errorf("printMicro() output = %q, want %q", out, "wthr\n")
	}
}
