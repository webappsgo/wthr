package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/webappsgo/wthr/src/privilege"
	"github.com/webappsgo/wthr/src/util"
)

// serviceDataDirs lists the directories the service owns: config first (the home of the
// service account), then data, cache, log and the backup directory. A system service on
// Windows keeps everything below one %ProgramData% directory.
func serviceDataDirs(system bool) ([]string, error) {
	if system && runtime.GOOS == "windows" {
		return []string{windowsServiceBaseDir()}, nil
	}

	paths, err := util.GetDirectoryPaths()
	if err != nil {
		return nil, err
	}

	return []string{paths.Config, paths.Data, paths.Cache, paths.Log, util.GetBackupPath(paths)}, nil
}

// windowsServiceBaseDir is %ProgramData%\{internal_org}\{internal_name} (AI.md PART 4).
func windowsServiceBaseDir() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}

	return filepath.Join(programData, serviceOrg, serviceName)
}

// isServiceOwnedDir guards every recursive delete: the path must be absolute and end in, or
// sit below, {internal_org}/{internal_name}.
func isServiceOwnedDir(dir string) bool {
	if !filepath.IsAbs(dir) {
		return false
	}

	clean := filepath.ToSlash(filepath.Clean(dir))
	suffix := "/" + serviceOrg + "/" + serviceName

	return strings.HasSuffix(clean, suffix) || strings.Contains(clean, suffix+"/")
}

// removeServiceDirs deletes every directory that passes the ownership guard and then the
// organization directory above it once nothing else is in it.
func removeServiceDirs(dirs []string) error {
	for _, dir := range dirs {
		if !isServiceOwnedDir(dir) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf(T("cli.service.remove_data_failed"), dir, err)
		}

		if parent := filepath.Dir(dir); filepath.Base(parent) == serviceOrg {
			_ = os.Remove(parent)
		}
	}

	return nil
}

// removeServiceData deletes the config, data, cache, log and backup directories, the PID
// file directory and, for a system service on Unix, the service user and group.
func removeServiceData(system bool) error {
	dirs, err := serviceDataDirs(system)
	if err != nil {
		return err
	}
	if system && runtime.GOOS != "windows" {
		dirs = append(dirs, filepath.Dir(util.SystemPIDFilePath()))
	}

	if err := removeServiceDirs(dirs); err != nil {
		return err
	}

	if system && runtime.GOOS != "windows" {
		if err := privilege.RemoveAccount(serviceName); err != nil {
			return fmt.Errorf(T("cli.service.account_remove_failed"), err)
		}
	}

	return nil
}
