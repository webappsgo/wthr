package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/webappsgo/wthr/src/config"
)

// ServeAccentCSS emits theme-specific primary colors for a configured accent.
func ServeAccentCSS(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetGlobalConfig()
	if cfg == nil || !validAccent(cfg.Web.UI.Accent) {
		http.NotFound(w, r)
		return
	}
	dark, light := accentForBackground(cfg.Web.UI.Accent, "#282a36"), accentForBackground(cfg.Web.UI.Accent, "#ffffff")
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = fmt.Fprintf(w, ":root{--color-primary:%s;--color-primary-bg:%s}\nhtml.theme-light{--color-primary:%s;--color-primary-bg:%s}\n@media(prefers-color-scheme:light){html.theme-auto{--color-primary:%s;--color-primary-bg:%s}}\n", dark, rgba(dark, .15), light, rgba(light, .12), light, rgba(light, .12))
}

// ServeBrandingAsset serves one validated local branding image.
func ServeBrandingAsset(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	if name == "." || name == "/" || name != r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:] {
		http.NotFound(w, r)
		return
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".svg" && ext != ".png" && ext != ".ico" && ext != ".webp" && ext != ".jpg" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(config.AssetsDir(), name)
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode().Perm() != 0o700 {
		http.NotFound(w, r)
		return
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() > 2<<20 {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil || len(content) > 2<<20 {
		http.NotFound(w, r)
		return
	}
	digest := sha256.Sum256(content)
	etag := `"` + hex.EncodeToString(digest[:]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox; style-src 'unsafe-inline'")
	if r.Header.Get("If-None-Match") == etag || r.Header.Get("If-Modified-Since") == info.ModTime().UTC().Format(http.TimeFormat) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	contentType := map[string]string{".svg": "image/svg+xml", ".png": "image/png", ".ico": "image/x-icon", ".webp": "image/webp", ".jpg": "image/jpeg"}[ext]
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprint(len(content)))
	_, _ = w.Write(content)
}

func validAccent(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(value[1:], 16, 24)
	return err == nil
}

func accentForBackground(accent, background string) string {
	r, g, b := parseHexColor(accent)
	br, bg, bb := parseHexColor(background)
	if contrastRatio(r, g, b, br, bg, bb) >= 4.5 {
		return strings.ToLower(accent)
	}
	h, s, l := rgbToHSL(r, g, b)
	step := -1.0
	if background == "#282a36" {
		step = 1
	}
	for l >= 0 && l <= 100 {
		l += step
		r, g, b = hslToRGB(h, s, l)
		if contrastRatio(r, g, b, br, bg, bb) >= 4.5 {
			return fmt.Sprintf("#%02x%02x%02x", r, g, b)
		}
	}
	return "#bd93f9"
}

func parseHexColor(value string) (int, int, int) {
	number, _ := strconv.ParseUint(strings.TrimPrefix(value, "#"), 16, 24)
	return int(number >> 16), int((number >> 8) & 0xff), int(number & 0xff)
}

func relativeLuminance(r, g, b int) float64 {
	channel := func(value int) float64 {
		c := float64(value) / 255
		if c <= .04045 {
			return c / 12.92
		}
		return math.Pow((c+.055)/1.055, 2.4)
	}
	return .2126*channel(r) + .7152*channel(g) + .0722*channel(b)
}

func contrastRatio(r, g, b, br, bg, bb int) float64 {
	a, bLum := relativeLuminance(r, g, b), relativeLuminance(br, bg, bb)
	if a < bLum {
		a, bLum = bLum, a
	}
	return (a + .05) / (bLum + .05)
}

func rgbToHSL(r, g, b int) (float64, float64, float64) {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	max, min := math.Max(rf, math.Max(gf, bf)), math.Min(rf, math.Min(gf, bf))
	h, s, l := 0.0, 0.0, (max+min)/2
	if max != min {
		d := max - min
		if l > .5 {
			s = d / (2 - max - min)
		} else {
			s = d / (max + min)
		}
		switch max {
		case rf:
			h = (gf - bf) / d
			if gf < bf {
				h += 6
			}
		case gf:
			h = (bf-rf)/d + 2
		default:
			h = (rf-gf)/d + 4
		}
		h *= 60
	}
	return h, s * 100, l * 100
}

func hslToRGB(h, s, l float64) (int, int, int) {
	s, l = s/100, l/100
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return int(math.Round((r + m) * 255)), int(math.Round((g + m) * 255)), int(math.Round((b + m) * 255))
}

func rgba(color string, alpha float64) string {
	r, g, b := parseHexColor(color)
	return fmt.Sprintf("rgba(%d, %d, %d, %.2f)", r, g, b, alpha)
}
