package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/display"
)

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

const (
	githubAPI    = "https://api.github.com/repos/webappsgo/wthr/releases"
	githubStable = githubAPI + "/latest"
	githubBeta   = githubAPI + "/tags/beta"
	githubDaily  = githubAPI + "/tags/daily"
)

// UpdateCommand handles update operations
func UpdateCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no update command specified. Use: check, yes, or branch {stable|beta|daily}")
	}

	cmd := args[0]

	switch cmd {
	case "check":
		return checkForUpdates()

	case "yes":
		return performUpdate("stable")

	case "branch":
		if len(args) < 2 {
			return fmt.Errorf("branch requires a value: stable, beta, or daily")
		}
		return performUpdate(args[1])

	default:
		return fmt.Errorf("unknown update command: %s", cmd)
	}
}

// checkForUpdates checks for available updates
func checkForUpdates() error {
	fmt.Println(T("cli.update.checking"))

	// Check stable
	stable, err := fetchRelease(githubStable)
	if err != nil {
		return fmt.Errorf("failed to check stable release: %w", err)
	}

	// Check beta
	beta, err := fetchRelease(githubBeta)
	if err != nil {
		fmt.Printf(T("cli.update.warning_beta_check_failed")+"\n", err)
	}

	// Check daily
	daily, err := fetchRelease(githubDaily)
	if err != nil {
		fmt.Printf(T("cli.update.warning_daily_check_failed")+"\n", err)
	}

	// Display current version
	fmt.Printf("\n"+T("cli.update.current_version")+"\n", Version)
	fmt.Println()

	// Display available versions
	if stable != nil {
		fmt.Printf(T("cli.update.stable_line")+"\n", stable.TagName, stable.PublishedAt.Format("2006-01-02"))
		if isNewer(stable.TagName, Version) {
			fmt.Printf(T("cli.update.update_available")+"\n", display.Emoji("✨", "*"))
		}
	}

	if beta != nil {
		fmt.Printf(T("cli.update.beta_line")+"\n", beta.TagName, beta.PublishedAt.Format("2006-01-02"))
		if isNewer(beta.TagName, Version) {
			fmt.Printf(T("cli.update.update_available")+"\n", display.Emoji("✨", "*"))
		}
	}

	if daily != nil {
		fmt.Printf(T("cli.update.daily_line")+"\n", daily.TagName, daily.PublishedAt.Format("2006-01-02"))
	}

	fmt.Println()
	fmt.Println(T("cli.update.to_update_heading"))
	fmt.Println(T("cli.update.hint_yes"))
	fmt.Println(T("cli.update.hint_branch_stable"))
	fmt.Println(T("cli.update.hint_branch_beta"))
	fmt.Println(T("cli.update.hint_branch_daily"))

	return nil
}

// performUpdate performs the actual update
func performUpdate(branch string) error {
	fmt.Printf(T("cli.update.updating_to_branch")+"\n", branch)

	// Get release URL based on branch
	var releaseURL string
	switch branch {
	case "stable":
		releaseURL = githubStable
	case "beta":
		releaseURL = githubBeta
	case "daily":
		releaseURL = githubDaily
	default:
		return fmt.Errorf("invalid branch: %s (use stable, beta, or daily)", branch)
	}

	// Fetch release info
	release, err := fetchRelease(releaseURL)
	if err != nil {
		return fmt.Errorf("failed to fetch release: %w", err)
	}

	fmt.Printf(T("cli.update.found_version")+"\n", release.TagName)

	// Check if already up to date
	if !isNewer(release.TagName, Version) && branch == "stable" {
		fmt.Printf(T("cli.update.already_up_to_date")+"\n", display.Emoji("✓", "[OK]"))
		return nil
	}

	// Find asset for current platform
	assetName := fmt.Sprintf("wthr-%s-%s", runtime.GOOS, runtime.GOARCH)
	var downloadURL string
	var assetSize int64

	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			assetSize = asset.Size
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	fmt.Printf(T("cli.update.downloading")+"\n", assetName, float64(assetSize)/(1024*1024))

	// Download binary to org-namespaced temp path per AI.md PART 29
	tmpDir := filepath.Join(os.TempDir(), "webappsgo")
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	tmpFile := filepath.Join(tmpDir, "wthr-update")
	if err := downloadFile(tmpFile, downloadURL); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	// Verify checksum per AI.md PART 23 line 32321
	// Checksum file is expected at {binary}.sha256
	checksumURL := downloadURL + ".sha256"
	fmt.Println(T("cli.update.verifying_checksum"))
	if err := verifyChecksum(tmpFile, checksumURL); err != nil {
		// Clean up failed download
		os.Remove(tmpFile)
		return fmt.Errorf("checksum verification failed: %w", err)
	}
	fmt.Printf(T("cli.update.checksum_verified")+"\n", display.Emoji("✓", "[OK]"))

	// Make executable
	if err := os.Chmod(tmpFile, 0755); err != nil {
		return fmt.Errorf("failed to make executable: %w", err)
	}

	// Get current binary path
	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current binary path: %w", err)
	}

	// Backup current binary
	backupPath := currentBinary + ".backup"
	if err := copyFile(currentBinary, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	fmt.Println(T("cli.update.backed_up_binary"))

	// Replace binary
	if err := os.Rename(tmpFile, currentBinary); err != nil {
		// Try copy if rename fails (cross-device link)
		if err := copyFile(tmpFile, currentBinary); err != nil {
			return fmt.Errorf("failed to replace binary: %w", err)
		}
		os.Remove(tmpFile)
	}

	// Verify new binary
	cmd := exec.Command(currentBinary, "--version")
	output, err := cmd.Output()
	if err != nil {
		// Restore backup
		os.Rename(backupPath, currentBinary)
		return fmt.Errorf("new binary verification failed, restored backup: %w", err)
	}

	fmt.Printf("\n"+T("cli.update.update_successful")+"\n", display.Emoji("✓", "[OK]"))
	fmt.Printf(T("cli.update.new_version")+"\n", release.TagName)
	fmt.Println(string(output))
	fmt.Println("\n" + T("cli.update.backup_saved_at") + " " + backupPath)
	fmt.Println(T("cli.update.restart_hint"))

	return nil
}

// fetchRelease fetches release information from GitHub
func fetchRelease(url string) (*GitHubRelease, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// downloadFile downloads a file from URL to local path
func downloadFile(filepath string, url string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Create progress indicator
	size := resp.ContentLength
	downloaded := int64(0)
	buf := make([]byte, 32*1024)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)

			// Show progress
			if size > 0 {
				percent := float64(downloaded) / float64(size) * 100
				fmt.Printf("\r"+T("cli.update.progress"), percent)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	// New line after progress
	fmt.Println()
	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Copy permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}

// isNewer checks if version a is newer than version b
func isNewer(a, b string) bool {
	// Strip 'v' prefix if present
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	// Simple string comparison for now
	// Could be enhanced with proper semantic versioning
	return a > b
}

// verifyChecksum verifies the SHA256 checksum of a file
// Per AI.md PART 23 line 32321: Update flow must verify checksum (SHA256)
func verifyChecksum(filepath, checksumURL string) error {
	// Download checksum file
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(checksumURL)
	if err != nil {
		return fmt.Errorf("failed to download checksum: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksum file not found (status %d)", resp.StatusCode)
	}

	// Read expected checksum (format: "hash  filename" or just "hash")
	checksumData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read checksum: %w", err)
	}

	expectedChecksum := strings.TrimSpace(string(checksumData))
	// Handle "hash  filename" format
	if parts := strings.Fields(expectedChecksum); len(parts) > 0 {
		expectedChecksum = parts[0]
	}

	// Calculate actual checksum of downloaded file
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))

	// Compare checksums (case-insensitive)
	if !strings.EqualFold(actualChecksum, expectedChecksum) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}
