package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/i18n"
	"golang.org/x/mod/semver"
	"golang.org/x/term"
)

type cliBinaryVersion struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type cliUpdateInfo struct {
	Available       string
	Minimum         string
	MinimumRequired bool
	Binary          cliBinaryVersion
	ServerURL       string
}

type autodiscoverResponse struct {
	Primary       string                      `json:"primary"`
	Cluster       []string                    `json:"cluster"`
	APIVersion    string                      `json:"api_version"`
	CLIVersions   map[string]cliBinaryVersion `json:"cli_versions"`
	CLIMinVersion string                      `json:"cli_min_version"`
	Timeout       int                         `json:"timeout"`
	Retry         int                         `json:"retry"`
	RetryDelay    int                         `json:"retry_delay"`
}

var cliUpdateLanguage = "en"

func setCLIUpdateLanguage(configured string) {
	instance := i18n.GetGlobalI18n()
	if instance == nil {
		cliUpdateLanguage = "en"
		return
	}
	if strings.TrimSpace(configured) != "" && !strings.EqualFold(strings.TrimSpace(configured), "auto") {
		cliUpdateLanguage = normalizeCLILanguage(configured, instance)
		return
	}
	for _, variable := range []string{"LC_ALL", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(variable)); value != "" {
			cliUpdateLanguage = normalizeCLILanguage(value, instance)
			return
		}
	}
	cliUpdateLanguage = "en"
}

func normalizeCLILanguage(value string, instance *i18n.I18n) string {
	value = strings.ToLower(strings.TrimSpace(value))
	parts := strings.FieldsFunc(value, func(char rune) bool {
		return char == '_' || char == '.' || char == '@' || char == '-'
	})
	if len(parts) == 0 || !instance.IsSupported(parts[0]) {
		return "en"
	}
	return parts[0]
}

func discoverCLIUpdate(config *CLIConfig, profile string) (cliUpdateInfo, error) {
	client := NewHTTPClient(config)
	var discovered autodiscoverResponse
	if err := client.GetJSON("/api/autodiscover", &discovered); err != nil {
		return cliUpdateInfo{}, err
	}
	if len(discovered.Cluster) > 0 {
		config.Server.Cluster = discovered.Cluster
	}
	if discovered.APIVersion != "" {
		config.Server.APIVersion = discovered.APIVersion
	}
	if discovered.Timeout > 0 {
		config.Server.Timeout = fmt.Sprintf("%ds", discovered.Timeout)
	}
	if discovered.Retry > 0 {
		config.Server.Retry = discovered.Retry
	}
	if discovered.RetryDelay > 0 {
		config.Server.RetryDelay = fmt.Sprintf("%ds", discovered.RetryDelay)
	}
	persisted, err := LoadConfigFromProfile(profile)
	if err != nil {
		return cliUpdateInfo{}, err
	}
	persisted.Server.Cluster = config.Server.Cluster
	persisted.Server.APIVersion = config.Server.APIVersion
	persisted.Server.Timeout = config.Server.Timeout
	persisted.Server.Retry = config.Server.Retry
	persisted.Server.RetryDelay = config.Server.RetryDelay
	if err := SaveConfigToProfile(persisted, profile); err != nil {
		return cliUpdateInfo{}, err
	}

	info := cliUpdateInfo{Minimum: strings.TrimSpace(discovered.CLIMinVersion), ServerURL: client.currentServer}
	if info.Minimum != "" && versionLessThan(Version, info.Minimum) {
		info.MinimumRequired = true
	}
	platform := runtime.GOOS + "-" + runtime.GOARCH
	binary, ok := discovered.CLIVersions[platform]
	if ok && versionLessThan(Version, binary.Version) {
		info.Available = binary.Version
		info.Binary = binary
	}
	return info, nil
}

func versionLessThan(current, required string) bool {
	toSemver := func(value string) string {
		value = strings.TrimSpace(value)
		if value == "" || value == "dev" || value == "unknown" {
			return ""
		}
		if !strings.HasPrefix(value, "v") {
			value = "v" + value
		}
		if !semver.IsValid(value) {
			return ""
		}
		return value
	}
	currentSemver := toSemver(current)
	requiredSemver := toSemver(required)
	return currentSemver != "" && requiredSemver != "" && semver.Compare(currentSemver, requiredSemver) < 0
}

func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func validateCLIUpdateMode(auto, interactive bool) error {
	if !interactive && !auto {
		return NewConfigError("non-interactive CLI updates require update.auto: true")
	}
	return nil
}

func cliUpdateText(key string, values map[string]string) string {
	instance := i18n.GetGlobalI18n()
	if instance == nil {
		return key
	}
	return instance.TranslateFormat(cliUpdateLanguage, key, values)
}

func confirmCLIUpdate(version string) bool {
	fmt.Print(cliUpdateText("cli.update.client_prompt", map[string]string{"version": version}))
	var answer string
	_, _ = fmt.Scanln(&answer)
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes", "s", "si", "sí", "o", "oui", "j", "ja":
		return true
	default:
		return false
	}
}

func installCLIUpdate(config *CLIConfig, update cliUpdateInfo) error {
	if update.Available == "" || update.Binary.SHA256 == "" {
		return NewAPIError("server did not provide a verified CLI update")
	}
	if update.ServerURL == "" {
		return NewConnectionError("no server is available for the CLI update")
	}
	binaryName := projectName + "-cli-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	serverURL, err := url.Parse(update.ServerURL)
	if err != nil || serverURL.Scheme != "https" || serverURL.Host == "" {
		return NewAPIError("CLI updates require an HTTPS server URL")
	}
	requestURL := strings.TrimRight(update.ServerURL, "/") + "/cli/binaries/" + binaryName
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return NewAPIError(fmt.Sprintf("failed to create CLI update request: %v", err))
	}
	request.Header.Set("User-Agent", UserAgent())
	if config.Auth.Token != "" {
		request.Header.Set("Authorization", "Bearer "+config.Auth.Token)
	}
	response, err := (&http.Client{Timeout: 5 * time.Minute}).Do(request)
	if err != nil {
		return NewConnectionError(fmt.Sprintf("failed to download CLI update: %v", err))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return NewAPIError(fmt.Sprintf("CLI update download returned status %d", response.StatusCode))
	}
	base := filepath.Join(os.TempDir(), projectOrg)
	if err := os.MkdirAll(base, 0755); err != nil {
		return NewConfigError(fmt.Sprintf("failed to create update temp directory: %v", err))
	}
	tempDir, err := os.MkdirTemp(base, projectName+"-")
	if err != nil {
		return NewConfigError(fmt.Sprintf("failed to create update temp directory: %v", err))
	}
	tempPath := filepath.Join(tempDir, "cli.update.tmp")
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return NewConfigError(fmt.Sprintf("failed to create update temp file: %v", err))
	}
	hasher := sha256.New()
	bytesWritten, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, (200<<20)+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.RemoveAll(tempDir)
		if copyErr != nil {
			return NewAPIError(fmt.Sprintf("failed to download CLI update: %v", copyErr))
		}
		return NewAPIError(fmt.Sprintf("failed to close CLI update file: %v", closeErr))
	}
	if bytesWritten > 200<<20 {
		_ = os.RemoveAll(tempDir)
		return NewAPIError("CLI update exceeds the 200 MiB download limit")
	}
	if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), update.Binary.SHA256) {
		_ = os.RemoveAll(tempDir)
		return NewAPIError("CLI update checksum is invalid")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tempPath, 0755); err != nil {
			_ = os.RemoveAll(tempDir)
			return NewConfigError(fmt.Sprintf("failed to set executable permissions: %v", err))
		}
	}
	currentPath, err := os.Executable()
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return NewConfigError(fmt.Sprintf("failed to locate CLI binary: %v", err))
	}
	currentPath, err = filepath.EvalSymlinks(currentPath)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return NewConfigError(fmt.Sprintf("failed to resolve CLI binary path: %v", err))
	}
	if err := replaceClientBinary(currentPath, tempPath); err != nil {
		_ = os.RemoveAll(tempDir)
		if errors.Is(err, os.ErrPermission) {
			return NewConfigError(fmt.Sprintf("you do not have permission to update %s; ask your admin or move the binary to a writable path", currentPath))
		}
		return NewConfigError(fmt.Sprintf("failed to replace CLI binary %s: %v", currentPath, err))
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := os.RemoveAll(tempDir); err != nil {
		return NewConfigError(fmt.Sprintf("failed to clean update temp directory: %v", err))
	}
	return restartClient()
}
