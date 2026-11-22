package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

var securityAreaMu sync.Mutex

// SaveIPAllowlistArea writes trusted IP entries to server.security.allowlist
// in the owning security.yml area file.
func SaveIPAllowlistArea(entries []IPAllowlistEntry) error {
	return updateSecurityArea(func(server map[string]interface{}) error {
		security, ok := server["security"].(map[string]interface{})
		if !ok {
			security = make(map[string]interface{})
			server["security"] = security
		}
		data, err := yaml.Marshal(entries)
		if err != nil {
			return err
		}
		var values interface{}
		if err := yaml.Unmarshal(data, &values); err != nil {
			return err
		}
		security["allowlist"] = values
		return nil
	})
}

func updateSecurityArea(update func(map[string]interface{}) error) error {
	securityAreaMu.Lock()
	defer securityAreaMu.Unlock()
	configPath := secretsConfigPath()
	dir := filepath.Join(filepath.Dir(configPath), "yaml.d")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create yaml.d: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("secure yaml.d: %w", err)
	}
	path := filepath.Join(dir, "security.yml")
	values := make(map[string]interface{})
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("security.yml is not a regular file")
		}
		if info.Mode().Perm() != 0600 {
			if err := os.Chmod(path, 0600); err != nil {
				return fmt.Errorf("secure security.yml: %w", err)
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read security.yml: %w", err)
		}
		if err := yaml.Unmarshal(data, &values); err != nil {
			return fmt.Errorf("parse security.yml: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect security.yml: %w", err)
	}
	server, ok := values["server"].(map[string]interface{})
	if !ok {
		server = make(map[string]interface{})
		values["server"] = server
	}
	if err := update(server); err != nil {
		return fmt.Errorf("update security.yml: %w", err)
	}
	data, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("encode security.yml: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".security-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary security.yml: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary security.yml: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write security.yml: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync security.yml: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close security.yml: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace security.yml: %w", err)
	}
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}
