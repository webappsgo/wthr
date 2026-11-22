package handler

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func TestAccentForBackgroundMeetsContrastTarget(t *testing.T) {
	for _, background := range []string{"#282a36", "#ffffff"} {
		t.Run(background, func(t *testing.T) {
			r, g, b := parseHexColor(accentForBackground("#777777", background))
			br, bg, bb := parseHexColor(background)
			if ratio := contrastRatio(r, g, b, br, bg, bb); ratio < 4.5 {
				t.Fatalf("contrast ratio %.2f < 4.5", ratio)
			}
		})
	}
}

func TestSaveBrandingUploadsStoresPrivateImage(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server:\n  port: 8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_DIR", configDir)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("logo_upload", "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	logoImage := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < logoImage.Bounds().Dy(); y++ {
		for x := 0; x < logoImage.Bounds().Dx(); x++ {
			logoImage.Set(x, y, color.NRGBA{R: 40, G: 90, B: 180, A: 255})
		}
	}
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, logoImage); err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(imageData.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/server/administration/branding", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = request.MultipartForm.RemoveAll() })
	cfg := &config.AppConfig{}
	if err := saveBrandingUploads(cfg, request); err != nil {
		t.Fatalf("save upload: %v", err)
	}
	if cfg.Server.Branding.Logo == "" || filepath.Ext(cfg.Server.Branding.Logo) != ".png" {
		t.Fatalf("saved logo reference = %q", cfg.Server.Branding.Logo)
	}
	savedPath := filepath.Join(config.AssetsDir(), cfg.Server.Branding.Logo)
	info, err := os.Stat(savedPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("asset mode = %o, want 600", info.Mode().Perm())
	}
	saved, err := os.ReadFile(savedPath)
	if err != nil || !bytes.Equal(saved, imageData.Bytes()) {
		t.Fatalf("saved image data = %q, err=%v", saved, err)
	}
	for _, variant := range []struct {
		name string
		dims image.Point
	}{{"-logo-200.png", image.Pt(200, 100)}, {"-logo-50.png", image.Pt(50, 25)}, {"-favicon-16.png", image.Pt(16, 16)}, {"-favicon-512.png", image.Pt(512, 512)}, {"-og-1200x630.jpg", image.Pt(1200, 630)}} {
		variantData, readErr := os.ReadFile(filepath.Join(config.AssetsDir(), strings.TrimSuffix(cfg.Server.Branding.Logo, filepath.Ext(cfg.Server.Branding.Logo))+variant.name))
		if readErr != nil {
			t.Fatalf("read generated variant %s: %v", variant.name, readErr)
		}
		decoded, _, decodeErr := image.DecodeConfig(bytes.NewReader(variantData))
		if decodeErr != nil || decoded.Width != variant.dims.X || decoded.Height != variant.dims.Y {
			t.Fatalf("variant %s dimensions=%dx%d err=%v want=%v", variant.name, decoded.Width, decoded.Height, decodeErr, variant.dims)
		}
	}
	if len(cfg.Server.Branding.Logo) != 28 {
		t.Fatalf("asset hash reference length = %d, want 28", len(cfg.Server.Branding.Logo))
	}
}

func TestServeAccentCSSWithoutConfiguredAccent(t *testing.T) {
	previous := config.GetGlobalConfig()
	config.SetGlobalConfig(nil)
	t.Cleanup(func() { config.SetGlobalConfig(previous) })

	recorder := httptest.NewRecorder()
	ServeAccentCSS(recorder, httptest.NewRequest("GET", "/static/css/accent.css", nil))
	if recorder.Code != 404 {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestServeBrandingAssetRequiresPrivateRegularFile(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server:\n  port: 8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assetsDir := filepath.Join(configDir, "assets")
	if err := os.Mkdir(assetsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Join(assetsDir, "logo.png")
	if err := os.WriteFile(assetPath, []byte("image bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(assetPath, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_DIR", configDir)

	recorder := httptest.NewRecorder()
	ServeBrandingAsset(recorder, httptest.NewRequest("GET", "/assets/logo.png", nil))
	if recorder.Code != 200 || recorder.Header().Get("Content-Type") != "image/png" || recorder.Header().Get("ETag") == "" {
		t.Fatalf("asset response = %d, headers %v", recorder.Code, recorder.Header())
	}

	if err := os.Chmod(assetPath, 0o644); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	ServeBrandingAsset(recorder, httptest.NewRequest("GET", "/assets/logo.png", nil))
	if recorder.Code != 404 {
		t.Fatalf("public asset permissions accepted: status=%d", recorder.Code)
	}
}
