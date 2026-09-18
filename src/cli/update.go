package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/common/display"
)

// BuildEpoch is the Unix build timestamp injected at link time. The daily
// channel is a single rolling tag that never matches the running version, so
// publish time is compared against this instead.
var BuildEpoch = ""

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
	githubAPI          = "https://api.github.com/repos/webappsgo/wthr/releases"
	githubReleasesList = githubAPI + "?per_page=100"
	checksumAssetName  = "sha256.txt"
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
		if !isValidBranch(args[1]) {
			return fmt.Errorf("invalid branch: %s (use stable, beta, or daily)", args[1])
		}
		return performUpdate(args[1])

	default:
		return fmt.Errorf("unknown update command: %s", cmd)
	}
}

func isValidBranch(branch string) bool {
	switch branch {
	case "stable", "beta", "daily":
		return true
	}
	return false
}

// checkForUpdates checks for available updates
func checkForUpdates() error {
	fmt.Println(T("cli.update.checking"))

	releases, err := listReleases()
	if err != nil {
		return fmt.Errorf("failed to check releases: %w", err)
	}

	stable := SelectRelease(releases, "stable", Version)
	beta := SelectRelease(releases, "beta", Version)
	daily := SelectRelease(releases, "daily", Version)

	fmt.Printf("\n"+T("cli.update.current_version")+"\n", Version)
	fmt.Println()

	if stable != nil {
		fmt.Printf(T("cli.update.stable_line")+"\n", stable.TagName, stable.PublishedAt.Format("2006-01-02"))
		fmt.Printf(T("cli.update.update_available")+"\n", display.Emoji("✨", "*"))
	}

	if beta != nil {
		fmt.Printf(T("cli.update.beta_line")+"\n", beta.TagName, beta.PublishedAt.Format("2006-01-02"))
		fmt.Printf(T("cli.update.update_available")+"\n", display.Emoji("✨", "*"))
	}

	if daily != nil {
		fmt.Printf(T("cli.update.daily_line")+"\n", daily.TagName, daily.PublishedAt.Format("2006-01-02"))
	}

	if stable == nil && beta == nil && daily == nil {
		fmt.Printf(T("cli.update.already_up_to_date")+"\n", display.Emoji("✓", "[OK]"))
		return nil
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
	if !isValidBranch(branch) {
		return fmt.Errorf("invalid branch: %s (use stable, beta, or daily)", branch)
	}

	fmt.Printf(T("cli.update.updating_to_branch")+"\n", branch)

	releases, err := listReleases()
	if err != nil {
		return fmt.Errorf("failed to fetch releases: %w", err)
	}

	release := SelectRelease(releases, branch, Version)
	if release == nil {
		fmt.Printf(T("cli.update.already_up_to_date")+"\n", display.Emoji("✓", "[OK]"))
		return nil
	}

	fmt.Printf(T("cli.update.found_version")+"\n", release.TagName)

	assetName := getBinaryName()
	var downloadURL, checksumURL string
	var assetSize int64
	for _, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			downloadURL = asset.BrowserDownloadURL
			assetSize = asset.Size
		case checksumAssetName:
			checksumURL = asset.BrowserDownloadURL
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	// Checksum verification is mandatory; an unverifiable download is refused
	// rather than installed
	if checksumURL == "" {
		return fmt.Errorf("no %s asset found in release - refusing unverified update", checksumAssetName)
	}

	fmt.Printf(T("cli.update.downloading")+"\n", assetName, float64(assetSize)/(1024*1024))

	tmpFile, err := os.CreateTemp("", "wthr-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := downloadFile(tmpPath, downloadURL); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	fmt.Println(T("cli.update.verifying_checksum"))
	expectedHash, err := fetchChecksum(checksumURL, assetName)
	if err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}
	if err := verifyChecksum(tmpPath, expectedHash); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}
	fmt.Printf(T("cli.update.checksum_verified")+"\n", display.Emoji("✓", "[OK]"))

	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			return fmt.Errorf("failed to make executable: %w", err)
		}
	}

	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current binary path: %w", err)
	}
	currentPath, err = filepath.EvalSymlinks(currentPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	if err := replaceBinary(currentPath, tmpPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	fmt.Printf("\n"+T("cli.update.update_successful")+"\n", display.Emoji("✓", "[OK]"))
	fmt.Printf(T("cli.update.new_version")+"\n", release.TagName)
	fmt.Println(T("cli.update.restart_hint"))

	return nil
}

// listReleases returns every published release. A 404 from the GitHub API means
// the repository has no releases, which is reported as "no updates available"
// rather than an error.
func listReleases() ([]GitHubRelease, error) {
	return fetchReleases(githubReleasesList)
}

func fetchReleases(url string) ([]GitHubRelease, error) {
	client := &http.Client{Timeout: 30 * time.Second}
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

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// SelectRelease picks the newest release eligible for the given channel, or nil
// when the running version is already current for that channel.
func SelectRelease(releases []GitHubRelease, branch, currentVersion string) *GitHubRelease {
	var newest *GitHubRelease
	var currentPublished time.Time

	for i := range releases {
		r := &releases[i]
		if sameVersion(r.TagName, currentVersion) {
			currentPublished = r.PublishedAt
		}
		if MatchesBranch(*r, branch) && (newest == nil || r.PublishedAt.After(newest.PublishedAt)) {
			newest = r
		}
	}

	if newest == nil {
		return nil
	}
	if newest.TagName == "daily" {
		if newest.PublishedAt.Unix() <= buildEpoch() {
			return nil
		}
		return newest
	}
	if sameVersion(newest.TagName, currentVersion) || !newest.PublishedAt.After(currentPublished) {
		return nil
	}
	return newest
}

// MatchesBranch implements cumulative channels: beta = beta + stable,
// daily = daily + beta + stable. A less-stable channel never leaves the
// installation older than a more-stable one.
func MatchesBranch(r GitHubRelease, branch string) bool {
	isDaily := r.TagName == "daily"
	isBeta := strings.HasSuffix(r.TagName, "-beta")
	isStable := !r.Prerelease && !isDaily && !isBeta

	switch branch {
	case "beta":
		return isBeta || isStable
	case "daily":
		return isDaily || isBeta || isStable
	default:
		return isStable
	}
}

// IsReleaseEligible reports whether a release has aged past the defer window.
// It gates the scheduled update_check task only; a manual update always sees
// the true latest release.
func IsReleaseEligible(publishedAt time.Time, deferDays int, now time.Time) bool {
	if deferDays <= 0 {
		return true
	}
	return now.Sub(publishedAt) >= time.Duration(deferDays)*24*time.Hour
}

// buildEpoch returns the link-time build timestamp, or 0 when it was not set.
func buildEpoch() int64 {
	if BuildEpoch == "" {
		return 0
	}
	epoch, err := strconv.ParseInt(BuildEpoch, 10, 64)
	if err != nil {
		return 0
	}
	return epoch
}

func sameVersion(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

// getBinaryName returns the release asset name for the running platform
func getBinaryName() string {
	name := "wthr-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
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

	fmt.Println()
	return nil
}

// fetchChecksum reads the release's sha256.txt asset and returns the hash
// recorded for assetName.
func fetchChecksum(checksumURL, assetName string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(checksumURL)
	if err != nil {
		return "", fmt.Errorf("failed to download checksum: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum file not found (status %d)", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read checksum: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == assetName {
			return fields[0], nil
		}
	}

	return "", fmt.Errorf("no checksum entry for %s in %s", assetName, checksumAssetName)
}

// verifyChecksum compares the SHA256 of a file against the expected hash
func verifyChecksum(path, expectedHash string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualChecksum, expectedHash) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualChecksum)
	}

	return nil
}
