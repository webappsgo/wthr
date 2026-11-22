package middleware

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/wthr/src/config"
)

func TestNormalizeRemoteGIFToStaticPNG(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.White, color.Black})
	var source bytes.Buffer
	if err := gif.Encode(&source, frame, nil); err != nil {
		t.Fatal(err)
	}
	data, extension, err := normalizeRemoteBrandingImage(source.Bytes(), "image/gif")
	if err != nil {
		t.Fatalf("normalize GIF: %v", err)
	}
	if extension != ".png" || !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("normalized image extension=%q, PNG signature=%t", extension, bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")))
	}
}

func TestGenerateBrandingImageVariantsPreservesAspectRatioAndCanvasSizes(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			source.Set(x, y, color.NRGBA{R: 40, G: 90, B: 180, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}

	variants, err := GenerateBrandingImageVariants("upload", encoded.Bytes(), "image/png")
	if err != nil {
		t.Fatalf("generate variants: %v", err)
	}
	if len(variants) != 9 {
		t.Fatalf("generated %d variants, want 9", len(variants))
	}
	wantDimensions := map[string]image.Point{
		"upload-logo-200.png":    image.Pt(200, 100),
		"upload-logo-50.png":     image.Pt(50, 25),
		"upload-og-1200x630.jpg": image.Pt(1200, 630),
	}
	for _, size := range []int{16, 32, 48, 180, 192, 512} {
		wantDimensions[fmt.Sprintf("upload-favicon-%d.png", size)] = image.Pt(size, size)
	}
	for _, variant := range variants {
		configImage, _, err := image.DecodeConfig(bytes.NewReader(variant.Data))
		if err != nil {
			t.Fatalf("decode %s: %v", variant.Name, err)
		}
		want, ok := wantDimensions[variant.Name]
		if !ok || configImage.Width != want.X || configImage.Height != want.Y {
			t.Errorf("%s dimensions = %dx%d, want %v", variant.Name, configImage.Width, configImage.Height, want)
		}
	}
}

func TestGenerateBrandingImageVariantsSkipsNonRasterFormats(t *testing.T) {
	variants, err := GenerateBrandingImageVariants("vector", []byte("<svg/>"), "image/svg+xml")
	if err != nil || len(variants) != 0 {
		t.Fatalf("SVG variants = %v, err=%v, want none without error", variants, err)
	}
}

func TestNormalizeRemoteBrandingImageRejectsUnsupportedType(t *testing.T) {
	if _, _, err := normalizeRemoteBrandingImage([]byte("text"), "text/plain"); err == nil {
		t.Fatal("unsupported content type was accepted")
	}
}

func TestBrandingVariantURLRequiresGeneratedPrivateAsset(t *testing.T) {
	if got := brandingVariantURL("https://example.com/logo.png", "-logo-200.png"); got != "" {
		t.Fatalf("remote source unexpectedly resolved without a local cache: %q", got)
	}
	if got := brandingVariantURL("data:image/png;base64,AA==", "-logo-200.png"); got != "" {
		t.Fatalf("data URI unexpectedly resolved as a local variant: %q", got)
	}
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server:\n  port: 8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_DIR", configDir)
	if err := config.SaveBrandingAsset("logo-logo-200.png", []byte("\x89PNG\r\n\x1a\n")); err != nil {
		t.Fatalf("save generated asset fixture: %v", err)
	}
	if got := brandingVariantURL("/assets/logo.png", "-logo-200.png"); got != "/assets/logo-logo-200.png" {
		t.Fatalf("branding variant URL = %q", got)
	}
}

func TestFindRemoteBrandingAssetIgnoresGeneratedVariants(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server:\n  port: 8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_DIR", configDir)
	for name, data := range map[string][]byte{
		"remote123.png":            []byte("\x89PNG\r\n\x1a\n"),
		"remote123-favicon-16.png": []byte("\x89PNG\r\n\x1a\n"),
	} {
		if err := config.SaveBrandingAsset(name, data); err != nil {
			t.Fatalf("save %s fixture: %v", name, err)
		}
	}
	if got := findRemoteBrandingAsset("remote123"); got != "remote123.png" {
		t.Fatalf("cached original = %q, want remote123.png", got)
	}
}
