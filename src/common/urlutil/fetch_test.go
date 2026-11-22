package urlutil

import (
	"testing"
	"time"
)

func TestDefaultFetchRemoteImageConfig(t *testing.T) {
	cfg := DefaultFetchRemoteImageConfig()
	if cfg.MaxSize != 10<<20 || cfg.Timeout != 30*time.Second || len(cfg.AllowedSchemes) != 1 || cfg.AllowedSchemes[0] != "https" {
		t.Fatalf("unexpected safe defaults: %+v", cfg)
	}
	if got := normalizeConfig(FetchRemoteImageConfig{}); got.MaxSize != cfg.MaxSize || got.Timeout != cfg.Timeout {
		t.Fatalf("empty config did not receive defaults: %+v", got)
	}
}

func TestValidateRemoteURLRejectsUnsafeTargets(t *testing.T) {
	cfg := DefaultFetchRemoteImageConfig()
	for _, rawURL := range []string{
		"http://example.com/image.png",
		"https://user:pass@example.com/image.png",
		"https://localhost/image.png",
		"https://127.0.0.1/image.png",
		"https://[::1]/image.png",
		"https://service.local/image.png",
		"https://service.internal/image.png",
		"file:///etc/passwd",
	} {
		t.Run(rawURL, func(t *testing.T) {
			if err := ValidateRemoteURL(rawURL, cfg); err == nil {
				t.Errorf("ValidateRemoteURL(%q) unexpectedly allowed target", rawURL)
			}
		})
	}
}

func TestDetectedImageContentTypeNormalizesIcon(t *testing.T) {
	got := detectedImageContentType([]byte{0, 0, 1, 0, 1, 0, 0, 0})
	if got != "image/x-icon" {
		t.Fatalf("icon content type = %q, want image/x-icon", got)
	}
}
