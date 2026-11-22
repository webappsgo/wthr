package config

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxBrandingAssetSize = 2 << 20

// SaveBrandingAsset writes one supported image into the private assets folder.
func SaveBrandingAsset(name string, data []byte) error {
	if filepath.Base(name) != name || name == "." || strings.HasPrefix(name, ".") || len(data) == 0 || len(data) > maxBrandingAssetSize {
		return fmt.Errorf("invalid branding asset name or size")
	}
	ext := strings.ToLower(filepath.Ext(name))
	_, ok := map[string]string{".svg": "image/svg+xml", ".png": "image/png", ".ico": "image/x-icon", ".webp": "image/webp", ".jpg": "image/jpeg"}[ext]
	if !ok || !validBrandingAssetBytes(ext, data) {
		return fmt.Errorf("branding asset format does not match its file extension")
	}
	dir := AssetsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create branding assets directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure branding assets directory: %w", err)
	}
	return writeConfigAtomic(filepath.Join(dir, name), data, 0o600)
}

func validBrandingAssetBytes(ext string, data []byte) bool {
	switch ext {
	case ".png":
		return bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n"))
	case ".jpg":
		return bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff})
	case ".webp":
		return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
	case ".ico":
		return len(data) >= 6 && bytes.Equal(data[:4], []byte{0, 0, 1, 0})
	case ".svg":
		var root struct {
			XMLName xml.Name
		}
		decoder := xml.NewDecoder(bytes.NewReader(data))
		decoder.Strict = true
		if err := decoder.Decode(&root); err != nil || !strings.EqualFold(root.XMLName.Local, "svg") {
			return false
		}
		return true
	default:
		return false
	}
}

// BrandingAssetMaxSize returns the upload cap for one image.
func BrandingAssetMaxSize() int64 { return maxBrandingAssetSize }

// WritePrivateFile atomically installs a private TLS file within a caller-owned
// configuration subdirectory.
func WritePrivateFile(path string, data []byte) error {
	if len(data) == 0 || len(data) > 2<<20 {
		return fmt.Errorf("invalid private file size")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	return writeConfigAtomic(path, data, 0o600)
}

// ReadBrandingAssets returns valid regular asset files keyed by their safe
// single-segment names for cluster synchronization.
func ReadBrandingAssets() (map[string][]byte, error) {
	dir := AssetsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]byte{}, nil
		}
		return nil, err
	}
	assets := make(map[string][]byte)
	for _, entry := range entries {
		if !entry.Type().IsRegular() || entry.Name() == "." || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxBrandingAssetSize {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !validBrandingAssetBytes(ext, data) {
			continue
		}
		assets[entry.Name()] = data
	}
	return assets, nil
}

// DeleteBrandingAsset removes a validated local asset during cluster sync.
func DeleteBrandingAsset(name string) error {
	if filepath.Base(name) != name || name == "." || strings.HasPrefix(name, ".") {
		return fmt.Errorf("invalid branding asset name")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if _, ok := map[string]bool{".svg": true, ".png": true, ".ico": true, ".webp": true, ".jpg": true}[ext]; !ok {
		return fmt.Errorf("unsupported branding asset extension")
	}
	path := filepath.Join(AssetsDir(), name)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fs.ErrInvalid
	}
	return os.Remove(path)
}
