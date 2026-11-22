package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/common/urlutil"
	"github.com/webappsgo/wthr/src/config"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const brandingRemoteRefresh = 24 * time.Hour
const brandingRemoteRetry = time.Hour

var brandingRemoteMu sync.Mutex
var brandingRemoteStates = map[string]*brandingRemoteState{}

type brandingRemoteState struct {
	name        string
	lastAttempt time.Time
	fetching    bool
}

// resolveRemoteBrandingImage returns only the local cached asset. A missing or
// stale cache triggers background refresh so third-party hosts never block a
// page request and failures fall back to the built-in branding asset.
func resolveRemoteBrandingImage(source string) string {
	digest := sha256.Sum256([]byte(source))
	key := hex.EncodeToString(digest[:])
	brandingRemoteMu.Lock()
	state := brandingRemoteStates[key]
	if state == nil {
		state = &brandingRemoteState{}
		brandingRemoteStates[key] = state
	}
	if state.name == "" {
		state.name = findRemoteBrandingAsset(key[:24])
	}
	if state.name != "" && localBrandingAssetURL(state.name) == "" {
		state.name = ""
	}
	cachedName := state.name
	cachedAt := time.Time{}
	if cachedName != "" {
		if info, err := os.Stat(filepath.Join(config.AssetsDir(), cachedName)); err == nil {
			cachedAt = info.ModTime()
		}
	}
	refreshAfter := brandingRemoteRefresh
	if cachedName == "" {
		refreshAfter = brandingRemoteRetry
	}
	if !state.fetching && time.Since(state.lastAttempt) >= refreshAfter {
		state.fetching = true
		state.lastAttempt = time.Now()
		go refreshRemoteBrandingAsset(source, key, state)
	}
	brandingRemoteMu.Unlock()
	if cachedName == "" || time.Since(cachedAt) < 0 || time.Since(cachedAt) >= brandingRemoteRefresh {
		return ""
	}
	return localBrandingAssetURL(cachedName)
}

func refreshRemoteBrandingAsset(source, key string, state *brandingRemoteState) {
	defer func() {
		brandingRemoteMu.Lock()
		state.fetching = false
		brandingRemoteMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	data, contentType, err := urlutil.FetchRemoteImage(ctx, source, urlutil.DefaultFetchRemoteImageConfig())
	if err != nil {
		log.Printf("WARNING: remote branding image refresh failed: %v", err)
		return
	}
	var extension string
	data, extension, err = normalizeRemoteBrandingImage(data, contentType)
	if err != nil {
		log.Printf("WARNING: remote branding image is not a supported cached image: %v", err)
		return
	}
	if extension == "" || int64(len(data)) > config.BrandingAssetMaxSize() {
		log.Printf("WARNING: remote branding image is not a supported cached image or exceeds the 2 MiB cache limit")
		return
	}
	name := key[:24] + extension
	if err := config.SaveBrandingAsset(name, data); err != nil {
		log.Printf("WARNING: remote branding image could not be cached: %v", err)
		return
	}
	contentType = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".webp": "image/webp", ".ico": "image/x-icon"}[extension]
	if err := SaveBrandingImageVariants(key[:24], data, contentType); err != nil {
		log.Printf("WARNING: remote branding image variants could not be generated: %v", err)
	}
	brandingRemoteMu.Lock()
	state.name = name
	brandingRemoteMu.Unlock()
}

// BrandingImageVariant is a generated, local PNG or JPEG derivative of a
// configured branding image.
type BrandingImageVariant struct {
	Name string
	Data []byte
}

// SaveBrandingImageVariants writes the AI.md PART 16 logo, favicon and OpenGraph
// derivatives beside an original asset. Unsupported vector and icon formats
// remain usable as originals and are left for the browser to scale.
func SaveBrandingImageVariants(prefix string, data []byte, contentType string) error {
	variants, err := GenerateBrandingImageVariants(prefix, data, contentType)
	if err != nil {
		return err
	}
	for _, variant := range variants {
		if err := config.SaveBrandingAsset(variant.Name, variant.Data); err != nil {
			return err
		}
	}
	return nil
}

// GenerateBrandingImageVariants creates the fixed-size derivatives required
// by AI.md PART 16 while preserving each source image's aspect ratio.
func GenerateBrandingImageVariants(prefix string, data []byte, contentType string) ([]BrandingImageVariant, error) {
	if prefix == "" || filepath.Base(prefix) != prefix || strings.HasPrefix(prefix, ".") {
		return nil, os.ErrInvalid
	}
	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" && contentType != "image/gif" {
		return nil, nil
	}
	configImage, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode branding image bounds: %w", err)
	}
	if configImage.Width < 1 || configImage.Height < 1 || configImage.Width > 20000 || configImage.Height > 20000 || int64(configImage.Width)*int64(configImage.Height) > maxBrandingSourcePixels {
		return nil, fmt.Errorf("branding image dimensions are outside the supported range")
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode branding image: %w", err)
	}
	variants := make([]BrandingImageVariant, 0, 9)
	for _, width := range []int{200, 50} {
		height := max(1, int(float64(source.Bounds().Dy())*float64(width)/float64(source.Bounds().Dx())+0.5))
		resized := resizeBrandingImage(source, width, height)
		encoded, err := encodeBrandingPNG(resized)
		if err != nil {
			return nil, err
		}
		variants = append(variants, BrandingImageVariant{Name: fmt.Sprintf("%s-logo-%d.png", prefix, width), Data: encoded})
	}
	for _, size := range []int{16, 32, 48, 180, 192, 512} {
		resized := fitBrandingImage(source, size, size, color.NRGBA{})
		encoded, err := encodeBrandingPNG(resized)
		if err != nil {
			return nil, err
		}
		variants = append(variants, BrandingImageVariant{Name: fmt.Sprintf("%s-favicon-%d.png", prefix, size), Data: encoded})
	}
	openGraph := fitBrandingImage(source, 1200, 630, color.White)
	encoded, err := encodeBrandingJPEG(openGraph)
	if err != nil {
		return nil, err
	}
	variants = append(variants, BrandingImageVariant{Name: prefix + "-og-1200x630.jpg", Data: encoded})
	return variants, nil
}

const maxBrandingSourcePixels = 20_000_000

func resizeBrandingImage(source image.Image, width, height int) *image.NRGBA {
	resized := image.NewNRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(resized, resized.Bounds(), source, source.Bounds(), xdraw.Over, nil)
	return resized
}

func fitBrandingImage(source image.Image, width, height int, background color.Color) *image.NRGBA {
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	if background != nil {
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				canvas.Set(x, y, background)
			}
		}
	}
	sourceWidth, sourceHeight := source.Bounds().Dx(), source.Bounds().Dy()
	scale := min(float64(width)/float64(sourceWidth), float64(height)/float64(sourceHeight))
	resizedWidth := max(1, int(float64(sourceWidth)*scale+0.5))
	resizedHeight := max(1, int(float64(sourceHeight)*scale+0.5))
	resized := resizeBrandingImage(source, resizedWidth, resizedHeight)
	point := image.Pt((width-resizedWidth)/2, (height-resizedHeight)/2)
	xdraw.Draw(canvas, image.Rectangle{Min: point, Max: point.Add(resized.Bounds().Size())}, resized, image.Point{}, xdraw.Over)
	return canvas
}

func encodeBrandingPNG(source image.Image) ([]byte, error) {
	var output bytes.Buffer
	if err := png.Encode(&output, source); err != nil {
		return nil, fmt.Errorf("encode branding PNG variant: %w", err)
	}
	return output.Bytes(), nil
}

func encodeBrandingJPEG(source image.Image) ([]byte, error) {
	var output bytes.Buffer
	for quality := 85; quality >= 45; quality -= 10 {
		output.Reset()
		if err := jpeg.Encode(&output, source, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("encode OpenGraph JPEG variant: %w", err)
		}
		if int64(output.Len()) <= config.BrandingAssetMaxSize() {
			return output.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("OpenGraph image variant exceeds the 2 MiB asset limit")
}

// brandingVariantURL returns an existing generated local derivative. An empty
// result means the original asset should be used instead.
func brandingVariantURL(original, suffix string) string {
	parsed, err := url.Parse(original)
	if err != nil || parsed.Path == "" || !strings.HasPrefix(parsed.Path, "/assets/") {
		return ""
	}
	name := filepath.Base(parsed.Path)
	variant := strings.TrimSuffix(name, filepath.Ext(name)) + suffix
	if variantURL := localBrandingAssetURL(variant); variantURL != "" {
		return variantURL
	}
	return ""
}

func normalizeRemoteBrandingImage(data []byte, contentType string) ([]byte, string, error) {
	if contentType != "image/gif" {
		extension := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/x-icon": ".ico"}[strings.ToLower(contentType)]
		if extension == "" {
			return nil, "", os.ErrInvalid
		}
		return data, extension, nil
	}
	decoded, err := gif.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	var rendered bytes.Buffer
	if err := png.Encode(&rendered, decoded); err != nil {
		return nil, "", err
	}
	return rendered.Bytes(), ".png", nil
}

func findRemoteBrandingAsset(prefix string) string {
	entries, err := os.ReadDir(config.AssetsDir())
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		name := entry.Name()
		ext := filepath.Ext(name)
		if strings.TrimSuffix(name, ext) == prefix && (ext == ".png" || ext == ".jpg" || ext == ".webp" || ext == ".ico") {
			return name
		}
	}
	return ""
}

func localBrandingAssetURL(name string) string {
	if filepath.Base(name) != name {
		return ""
	}
	path := filepath.Join(config.AssetsDir(), name)
	directory, err := os.Stat(filepath.Dir(path))
	if err != nil || !directory.IsDir() || directory.Mode().Perm() != 0o700 {
		return ""
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() > 2<<20 {
		return ""
	}
	return "/assets/" + url.PathEscape(name)
}
