// Package backup implements backup and retention logic per AI.md PART 22
// (Backup Retention, lines 36207-36500).
package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RetentionConfig controls which backups the tiered pruning sweep keeps per
// AI.md PART 22's retention settings table (lines 36211-36217).
type RetentionConfig struct {
	// MaxBackups is the number of daily full backups to keep (default 1,
	// minimum 1 - invalid values are normalized to the default).
	MaxBackups int
	// KeepWeekly is the number of Sunday backups to keep (0 = disabled).
	KeepWeekly int
	// KeepMonthly is the number of 1st-of-month backups to keep (0 = disabled).
	KeepMonthly int
	// KeepYearly is the number of January 1st backups to keep (0 = disabled).
	KeepYearly int
	// MaxTotalSize is a hard cap on total backup directory size: a percent
	// string ("10%"), an absolute size ("50G"), or a falsey value to disable
	// the cap. It overrides all count-based limits once exceeded.
	MaxTotalSize string
}

// DefaultRetention returns AI.md PART 22's default retention policy: one
// daily full backup and a 10% hard size cap, with the weekly/monthly/yearly
// tiers disabled.
func DefaultRetention() RetentionConfig {
	return RetentionConfig{MaxBackups: 1, MaxTotalSize: "10%"}
}

// retentionFalseyValues are the values AI.md PART 22 defines as "disabled"
// for any retention setting that accepts a falsey override.
var retentionFalseyValues = map[string]bool{
	"0": true, "false": true, "no": true, "none": true,
	"disable": true, "disabled": true, "off": true,
}

func isFalseyRetentionValue(s string) bool {
	return retentionFalseyValues[strings.ToLower(strings.TrimSpace(s))]
}

// Normalize substitutes the spec default for any invalid setting and returns
// warnings for both invalid values and values that exceed the recommended
// thresholds, matching AI.md PART 22's "warn, don't error - server must
// start" validation rule (lines 36364-36388). Warnings never block startup or
// a save - the caller logs/returns them, the corrected config is what runs.
func (r RetentionConfig) Normalize() (RetentionConfig, []string) {
	var warnings []string
	out := r

	if out.MaxBackups < 1 {
		warnings = append(warnings, fmt.Sprintf("max_backups: %d invalid, using default 1", out.MaxBackups))
		out.MaxBackups = 1
	} else if out.MaxBackups > 7 {
		warnings = append(warnings, fmt.Sprintf("max_backups: %d exceeds recommended 7 (%d days of daily backups)", out.MaxBackups, out.MaxBackups))
	}

	if out.KeepWeekly < 0 {
		warnings = append(warnings, fmt.Sprintf("keep_weekly: %d invalid, using default 0", out.KeepWeekly))
		out.KeepWeekly = 0
	} else if out.KeepWeekly > 8 {
		warnings = append(warnings, fmt.Sprintf("keep_weekly: %d exceeds recommended 8 (more than 2 months of weekly backups)", out.KeepWeekly))
	}

	if out.KeepMonthly < 0 {
		warnings = append(warnings, fmt.Sprintf("keep_monthly: %d invalid, using default 0", out.KeepMonthly))
		out.KeepMonthly = 0
	} else if out.KeepMonthly > 12 {
		warnings = append(warnings, fmt.Sprintf("keep_monthly: %d exceeds recommended 12 (more than a year of monthly backups)", out.KeepMonthly))
	}

	if out.KeepYearly < 0 {
		warnings = append(warnings, fmt.Sprintf("keep_yearly: %d invalid, using default 0", out.KeepYearly))
		out.KeepYearly = 0
	} else if out.KeepYearly > 2 {
		warnings = append(warnings, fmt.Sprintf("keep_yearly: %d exceeds recommended 2 (more than 2 years of yearly backups)", out.KeepYearly))
	}

	if strings.TrimSpace(out.MaxTotalSize) == "" {
		out.MaxTotalSize = "10%"
	}

	return out, warnings
}

// sizeSpecPattern parses an absolute size like "50G", "512MB", or "1024" into
// a numeric value plus an optional binary-unit suffix.
var sizeSpecPattern = regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)\s*(TB|GB|MB|KB|B|T|G|M|K)?$`)

// ParseMaxTotalSizeBytes resolves a retention.max_total_size value into an
// absolute byte cap. A percent string is resolved against volumeTotalBytes
// (the backup volume's total capacity); an absolute size is parsed directly;
// a falsey value or empty result disables the cap (returns 0, nil). Exported
// so callers (e.g. the admin API save handler) can validate a max_total_size
// string before persisting it.
func ParseMaxTotalSizeBytes(spec string, volumeTotalBytes int64) (int64, error) {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" || isFalseyRetentionValue(trimmed) {
		return 0, nil
	}

	if strings.HasSuffix(trimmed, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(trimmed, "%")), 64)
		if err != nil || pct <= 0 {
			return 0, fmt.Errorf("invalid max_total_size percent %q", spec)
		}
		if volumeTotalBytes <= 0 {
			// Volume size could not be determined - treat the cap as disabled
			// rather than pruning against an unknown denominator.
			return 0, nil
		}
		return int64(float64(volumeTotalBytes) * pct / 100), nil
	}

	m := sizeSpecPattern.FindStringSubmatch(trimmed)
	if m == nil {
		return 0, fmt.Errorf("invalid max_total_size %q", spec)
	}
	value, err := strconv.ParseFloat(m[1], 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid max_total_size %q", spec)
	}

	var multiplier int64 = 1
	switch strings.ToUpper(m[2]) {
	case "K", "KB":
		multiplier = 1 << 10
	case "M", "MB":
		multiplier = 1 << 20
	case "G", "GB":
		multiplier = 1 << 30
	case "T", "TB":
		multiplier = 1 << 40
	}
	return int64(value * float64(multiplier)), nil
}

// backupNamePattern matches every full backup archive src/backup creates
// that is subject to count-based tiered retention: the date-only scheduled
// daily full (wthr_backup_YYYY-MM-DD.tar.gz[.enc]) and the timestamped
// manual/CLI/API backup (wthr_backup_YYYY-MM-DD_HHMMSS.tar.gz[.enc]), per
// AI.md PART 22's "Backup Files Created" table (lines 36453-36471). The date
// comes from the filename, not the file's mtime, so classification survives
// a restore or a copy that changes mtimes.
var backupNamePattern = regexp.MustCompile(`^wthr_backup_(\d{4})-(\d{2})-(\d{2})(?:_\d{6})?\.tar\.gz(\.enc)?$`)

// incrementalNamePattern matches the daily/hourly incremental backup files
// (wthr-daily.tar.gz[.enc], wthr-hourly.tar.gz[.enc]) per AI.md PART 22.
// These are always exactly one file per kind, replaced in place by the next
// scheduled run, and are never subject to the count-based tiered retention
// sweep - only the max_total_size cap can remove them.
var incrementalNamePattern = regexp.MustCompile(`^wthr-(daily|hourly)\.tar\.gz(\.enc)?$`)

// retainedBackup is one backup archive under consideration by the retention
// sweep.
type retainedBackup struct {
	path string
	name string
	date time.Time
	size int64
}

// listRetainedBackups collects every backup archive in backupDir that
// matches the app's naming pattern, oldest first. Files that don't match
// (stray uploads, foreign files) are left alone - the sweep only prunes
// backups it created.
func listRetainedBackups(backupDir string) ([]retainedBackup, error) {
	matches, err := filepath.Glob(filepath.Join(backupDir, "wthr_backup_*.tar.gz*"))
	if err != nil {
		return nil, err
	}

	backups := make([]retainedBackup, 0, len(matches))
	for _, m := range matches {
		name := filepath.Base(m)
		sub := backupNamePattern.FindStringSubmatch(name)
		if sub == nil {
			continue
		}
		date, err := time.Parse("2006-01-02", sub[1]+"-"+sub[2]+"-"+sub[3])
		if err != nil {
			continue
		}
		var size int64
		if info, err := os.Stat(m); err == nil {
			size = info.Size()
		}
		backups = append(backups, retainedBackup{path: m, name: name, date: date, size: size})
	}

	sort.Slice(backups, func(i, j int) bool { return backups[i].date.Before(backups[j].date) })
	return backups, nil
}

// listIncrementalBackups collects the daily/hourly incremental files present
// in backupDir (0-2 entries - each kind is always exactly one file, replaced
// in place). Their "date" is the file's mtime, since these files are
// overwritten rather than renamed, so the filename carries no date.
func listIncrementalBackups(backupDir string) ([]retainedBackup, error) {
	matches, err := filepath.Glob(filepath.Join(backupDir, "wthr-*.tar.gz*"))
	if err != nil {
		return nil, err
	}

	backups := make([]retainedBackup, 0, len(matches))
	for _, m := range matches {
		name := filepath.Base(m)
		if !incrementalNamePattern.MatchString(name) {
			continue
		}
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		backups = append(backups, retainedBackup{path: m, name: name, date: info.ModTime(), size: info.Size()})
	}

	sort.Slice(backups, func(i, j int) bool { return backups[i].date.Before(backups[j].date) })
	return backups, nil
}

// keepNewest marks the newest n entries of list (list must be sorted oldest
// first) as kept in the given set.
func keepNewest(list []retainedBackup, n int, kept map[string]bool) {
	if n <= 0 {
		return
	}
	start := len(list) - n
	if start < 0 {
		start = 0
	}
	for _, b := range list[start:] {
		kept[b.path] = true
	}
}

// BackupInfo is a lightweight, exported view of a dated backup archive for
// callers outside the package (e.g. the scheduler's disk-space guard).
type BackupInfo struct {
	Path string
	Name string
	Date time.Time
	Size int64
}

// ListDatedBackups exposes the app-created full backups subject to
// count-based tiered retention (wthr_backup_YYYY-MM-DD[_HHMMSS].tar.gz[.enc])
// to callers outside the package, oldest first.
func ListDatedBackups(backupDir string) ([]BackupInfo, error) {
	backups, err := listRetainedBackups(backupDir)
	if err != nil {
		return nil, err
	}
	out := make([]BackupInfo, len(backups))
	for i, b := range backups {
		out[i] = BackupInfo{Path: b.path, Name: b.name, Date: b.date, Size: b.size}
	}
	return out, nil
}

// CountBackups returns how many app-created backup archives remain in
// backupDir, matching the same naming pattern applyRetention prunes against.
// Callers use this after Create() to report the "remaining count" AI.md PART
// 22 requires in the backup.retention_cleanup audit event.
func CountBackups(backupDir string) (int, error) {
	backups, err := listRetainedBackups(backupDir)
	return len(backups), err
}

// applyRetention prunes backupDir per AI.md PART 22's tiered retention
// algorithm (lines 36481-36498): a backup can satisfy yearly, monthly,
// weekly, and daily tiers simultaneously, so each tier independently keeps
// its newest N candidates and a backup survives if any tier keeps it.
// Unmarked backups are deleted, oldest first. If retention.MaxTotalSize
// resolves to a positive cap, a final pass deletes the oldest survivors
// until the total is back under the cap - the size cap overrides every
// count-based limit. It returns the filenames deleted, for the
// backup.retention_cleanup audit event.
func applyRetention(backupDir string, retention RetentionConfig, volumeTotalBytes int64) ([]string, error) {
	normalized, _ := retention.Normalize()

	backups, err := listRetainedBackups(backupDir)
	if err != nil {
		return nil, err
	}

	var yearly, monthly, weekly []retainedBackup
	for _, b := range backups {
		if b.date.Month() == time.January && b.date.Day() == 1 {
			yearly = append(yearly, b)
		}
		if b.date.Day() == 1 {
			monthly = append(monthly, b)
		}
		if b.date.Weekday() == time.Sunday {
			weekly = append(weekly, b)
		}
	}

	kept := make(map[string]bool, len(backups))
	keepNewest(yearly, normalized.KeepYearly, kept)
	keepNewest(monthly, normalized.KeepMonthly, kept)
	keepNewest(weekly, normalized.KeepWeekly, kept)
	keepNewest(backups, normalized.MaxBackups, kept)

	var deleted []string
	remaining := make([]retainedBackup, 0, len(backups))
	for _, b := range backups {
		if kept[b.path] {
			remaining = append(remaining, b)
			continue
		}
		if err := os.Remove(b.path); err != nil {
			return deleted, fmt.Errorf("failed to delete old backup %s: %w", b.name, err)
		}
		deleted = append(deleted, b.name)
	}

	// Daily/hourly incrementals are never pruned by the count-based tiers
	// above (AI.md PART 22: "always exactly 1 file, always replaced"), but
	// they still occupy space, so the size cap pass below - the one sweep
	// step nothing is exempt from - considers them too.
	incrementals, incErr := listIncrementalBackups(backupDir)
	if incErr != nil {
		return deleted, incErr
	}
	remaining = append(remaining, incrementals...)
	sort.Slice(remaining, func(i, j int) bool { return remaining[i].date.Before(remaining[j].date) })

	capBytes, capErr := ParseMaxTotalSizeBytes(normalized.MaxTotalSize, volumeTotalBytes)
	if capErr == nil && capBytes > 0 {
		var total int64
		for _, b := range remaining {
			total += b.size
		}
		for i := 0; i < len(remaining) && total > capBytes; i++ {
			if err := os.Remove(remaining[i].path); err != nil {
				return deleted, fmt.Errorf("failed to delete old backup %s: %w", remaining[i].name, err)
			}
			total -= remaining[i].size
			deleted = append(deleted, remaining[i].name)
		}
	}

	return deleted, nil
}
