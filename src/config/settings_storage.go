package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var sharedConfigSyncMu sync.RWMutex
var sharedConfigWrite func(string, any) error
var sharedConfigDelete func(string) error
var sharedConfigFileSync func() error

// SetSharedConfigSyncHooks installs the database-first config sync hooks used
// only while the application is connected to a shared Cluster Mode database.
func SetSharedConfigSyncHooks(write func(string, any) error, delete func(string) error, fileSync ...func() error) {
	sharedConfigSyncMu.Lock()
	defer sharedConfigSyncMu.Unlock()
	sharedConfigWrite = write
	sharedConfigDelete = delete
	sharedConfigFileSync = nil
	if len(fileSync) > 0 {
		sharedConfigFileSync = fileSync[0]
	}
}

// SyncSharedConfigFromFiles imports changed area-file values into the shared
// database after the file watcher validates a hand edit.
func SyncSharedConfigFromFiles() error {
	sharedConfigSyncMu.RLock()
	syncFiles := sharedConfigFileSync
	sharedConfigSyncMu.RUnlock()
	if syncFiles == nil {
		return nil
	}
	return syncFiles()
}

// ReadSetting returns a value from the merged configuration files and reports
// whether the dotted key exists. Credentials are read only from secrets.yml.
func ReadSetting(key string) (any, bool, error) {
	if err := validateSettingKey(key); err != nil {
		return nil, false, err
	}
	key = canonicalSettingKey(key)
	if CanonicalConfigOwner(key) == "secrets.yml" {
		return ReadSecretValue(key)
	}
	if key == "server.database.url" || key == "server.cache.url" {
		if secret, exists, err := ReadSecretValue(key); err != nil {
			return nil, false, err
		} else if exists {
			return secret, true, nil
		}
	}
	configPath := resolveConfigPath()
	merged, _, err := loadLayers(configPath, false)
	if err != nil {
		return nil, false, err
	}
	node := nodeAtPath(merged, strings.Split(key, "."))
	if node == nil {
		return nil, false, nil
	}
	var value any
	if err := node.Decode(&value); err != nil {
		return nil, false, fmt.Errorf("decode setting %s: %w", key, err)
	}
	return value, true, nil
}

// ReadAllSettings returns all non-secret values in the merged YAML config.
func ReadAllSettings() (map[string]any, error) {
	merged, _, err := loadLayers(resolveConfigPath(), false)
	if err != nil {
		return nil, err
	}
	values := make(map[string]any)
	flattenSettingNodes(merged, "", values)
	return values, nil
}

// SettingFileModTime returns the timestamp of the area file that owns a key.
func SettingFileModTime(key string) (time.Time, bool) {
	if strings.HasPrefix(key, "assets.") {
		name := strings.TrimPrefix(key, "assets.")
		if filepath.Base(name) != name || name == "." || strings.HasPrefix(name, ".") {
			return time.Time{}, false
		}
		info, err := os.Lstat(filepath.Join(AssetsDir(), name))
		if err != nil || !info.Mode().IsRegular() {
			return time.Time{}, false
		}
		return info.ModTime(), true
	}
	owner := CanonicalConfigOwner(canonicalSettingKey(key))
	if owner == "secrets.yml" {
		return time.Time{}, false
	}
	configPath := resolveConfigPath()
	path := configPath
	if owner != "server.yml" {
		path = filepath.Join(DropinDir(configPath), filepath.Base(owner))
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

func flattenSettingNodes(node *yaml.Node, prefix string, values map[string]any) {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			flattenSettingNodes(node.Content[i+1], path, values)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			path := fmt.Sprintf("%s.%d", prefix, i)
			flattenSettingNodes(child, path, values)
		}
	default:
		if prefix == "" || CanonicalConfigOwner(prefix) == "secrets.yml" {
			return
		}
		var value any
		if node.Decode(&value) == nil {
			values[prefix] = value
		}
	}
}

// WriteSetting writes a value to the file that owns its canonical config key.
// The file is atomically replaced with owner-only permissions.
func WriteSetting(key string, value any) error {
	return writeSetting(key, value, false)
}

// WriteSyncedSetting applies a shared database value to this node's area file
// without writing it back to the shared database.
func WriteSyncedSetting(key string, value any) error {
	return writeSetting(key, value, true)
}

func writeSetting(key string, value any, fromSync bool) error {
	if err := validateSettingKey(key); err != nil {
		return err
	}
	key = canonicalSettingKey(key)
	if !fromSync && CanonicalConfigOwner(key) != "server.yml" && CanonicalConfigOwner(key) != "secrets.yml" {
		sharedConfigSyncMu.RLock()
		writer := sharedConfigWrite
		sharedConfigSyncMu.RUnlock()
		if writer != nil {
			if err := writer(key, value); err != nil {
				return fmt.Errorf("write shared setting %s: %w", key, err)
			}
		}
	}
	if CanonicalConfigOwner(key) == "secrets.yml" || isSecretSettingValue(key, value) {
		if err := WriteSecretValue(key, value); err != nil {
			return err
		}
		if isSecretSettingValue(key, value) {
			if err := deleteOwnerValue(resolveConfigPath(), key, true); err != nil {
				return err
			}
		}
		refreshGlobalConfig()
		return nil
	}
	if key == "server.database.url" || key == "server.cache.url" {
		if err := DeleteSecret(key); err != nil {
			return err
		}
	}
	configPath := resolveConfigPath()
	owner := CanonicalConfigOwner(key)
	ownerPath := configPath
	if owner != "server.yml" {
		ownerPath = filepath.Join(DropinDir(configPath), filepath.Base(owner))
	}
	root, err := readConfigRoot(ownerPath)
	if err != nil {
		return err
	}
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode setting %s: %w", key, err)
	}
	var valueDoc yaml.Node
	if err := yaml.Unmarshal(encoded, &valueDoc); err != nil {
		return fmt.Errorf("parse setting %s: %w", key, err)
	}
	valueNode, err := rootValue(&valueDoc)
	if err != nil {
		return fmt.Errorf("parse setting %s: %w", key, err)
	}
	if err := setNodePath(root, strings.Split(key, "."), valueNode); err != nil {
		return fmt.Errorf("set config setting %s: %w", key, err)
	}
	secretValues := collectSecretConfigValues(root, "", "")
	if err := persistSecretConfigValues(secretValues); err != nil {
		return fmt.Errorf("store secret values from setting %s: %w", key, err)
	}
	for _, secret := range secretValues {
		removeNodePath(root, strings.Split(secret.sourceKey, "."))
	}
	if err := writeOwnedConfig(ownerPath, root, owner == "server.yml"); err != nil {
		return err
	}
	refreshGlobalConfig()
	return nil
}

func collectSecretConfigValues(node *yaml.Node, prefix, sourcePrefix string) []secretConfigValue {
	values := make([]secretConfigValue, 0)
	if node == nil {
		return values
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			path := key
			sourcePath := key
			if prefix != "" {
				path = prefix + "." + key
				sourcePath = sourcePrefix + "." + key
			}
			values = append(values, collectSecretConfigValues(node.Content[i+1], path, sourcePath)...)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			part := fmt.Sprintf("%s.%d", prefix, i)
			sourcePart := fmt.Sprintf("%s.%d", sourcePrefix, i)
			if prefix == "server.auth.oidc.providers" || prefix == "server.auth.ldap.providers" {
				if name := mappingString(child, "name"); name != "" {
					part = prefix + "." + name
				}
			}
			values = append(values, collectSecretConfigValues(child, part, sourcePart)...)
		}
	default:
		if isSecretConfigKey(prefix, node.Value) {
			values = append(values, secretConfigValue{key: prefix, sourceKey: sourcePrefix, value: cloneYAMLNode(node)})
		}
	}
	return values
}

func mappingString(node *yaml.Node, key string) string {
	value := nodeAtPath(node, []string{key})
	if value == nil || value.Kind != yaml.ScalarNode {
		return ""
	}
	return value.Value
}

func isSecretSettingValue(key string, value any) bool {
	text, ok := value.(string)
	return ok && isSecretConfigKey(key, text)
}

// DeleteSetting removes a key from its owning configuration file.
func DeleteSetting(key string) error {
	return deleteSetting(key, false)
}

// DeleteSyncedSetting removes a shared database value from this node's area
// file without issuing another database mutation.
func DeleteSyncedSetting(key string) error {
	return deleteSetting(key, true)
}

func deleteSetting(key string, fromSync bool) error {
	if err := validateSettingKey(key); err != nil {
		return err
	}
	key = canonicalSettingKey(key)
	if !fromSync && CanonicalConfigOwner(key) != "server.yml" && CanonicalConfigOwner(key) != "secrets.yml" {
		sharedConfigSyncMu.RLock()
		deleter := sharedConfigDelete
		sharedConfigSyncMu.RUnlock()
		if deleter != nil {
			if err := deleter(key); err != nil {
				return fmt.Errorf("delete shared setting %s: %w", key, err)
			}
		}
	}
	if CanonicalConfigOwner(key) == "secrets.yml" {
		if err := DeleteSecret(key); err != nil {
			return err
		}
		refreshGlobalConfig()
		return nil
	}
	if key == "server.database.url" || key == "server.cache.url" {
		if err := DeleteSecret(key); err != nil {
			return err
		}
	}
	configPath := resolveConfigPath()
	owner := CanonicalConfigOwner(key)
	ownerPath := configPath
	if owner != "server.yml" {
		ownerPath = filepath.Join(DropinDir(configPath), filepath.Base(owner))
	}
	root, err := readConfigRoot(ownerPath)
	if err != nil {
		return err
	}
	if !deleteNodePath(root, strings.Split(key, ".")) {
		return nil
	}
	if err := writeOwnedConfig(ownerPath, root, owner == "server.yml"); err != nil {
		return err
	}
	refreshGlobalConfig()
	return nil
}

func deleteOwnerValue(ownerPath, key string, serverFile bool) error {
	root, err := readConfigRoot(ownerPath)
	if err != nil {
		return err
	}
	if !deleteNodePath(root, strings.Split(key, ".")) {
		return nil
	}
	return writeOwnedConfig(ownerPath, root, serverFile)
}

func canonicalSettingKey(key string) string {
	if key == "smtp.password" {
		return "server.notifications.email.smtp.password"
	}
	return key
}

// ResetAreaSettings restores configured shared settings to their validated
// defaults while preserving node-local server.yml and secrets.yml values.
func ResetAreaSettings() error {
	configPath := resolveConfigPath()
	if _, err := LoadConfig(); err != nil {
		return fmt.Errorf("load configuration before reset: %w", err)
	}
	areaDir := DropinDir(configPath)
	if err := os.MkdirAll(areaDir, 0o700); err != nil {
		return fmt.Errorf("create config area directory: %w", err)
	}
	if err := os.Chmod(areaDir, 0o700); err != nil {
		return fmt.Errorf("secure config area directory: %w", err)
	}
	entries, err := os.ReadDir(areaDir)
	if err != nil {
		return fmt.Errorf("list config area files: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		if err := os.Remove(filepath.Join(areaDir, entry.Name())); err != nil {
			return fmt.Errorf("remove config area file %s: %w", entry.Name(), err)
		}
	}
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("reload default configuration: %w", err)
	}
	if err := SaveConfig(cfg); err != nil {
		return fmt.Errorf("write default configuration: %w", err)
	}
	SetGlobalConfig(cfg)
	return nil
}

func validateSettingKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("setting key is required")
	}
	for _, part := range strings.Split(key, ".") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid setting key %q", key)
		}
	}
	return nil
}

func resolveConfigPath() string {
	if path := findConfigFile(); path != "" {
		return path
	}
	return getConfigPath()
}

// CurrentConfigDir returns the directory containing the active server.yml.
func CurrentConfigDir() string { return filepath.Dir(resolveConfigPath()) }

func rootValue(doc *yaml.Node) (*yaml.Node, error) {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, errors.New("empty YAML value")
	}
	return cloneYAMLNode(doc.Content[0]), nil
}

func deleteNodePath(root *yaml.Node, parts []string) bool {
	if root == nil || len(parts) == 0 {
		return false
	}
	if root.Kind == yaml.SequenceNode {
		index := -1
		if _, err := fmt.Sscanf(parts[0], "%d", &index); err != nil || index < 0 || index >= len(root.Content) {
			return false
		}
		if len(parts) == 1 {
			root.Content = append(root.Content[:index], root.Content[index+1:]...)
			return true
		}
		return deleteNodePath(root.Content[index], parts[1:])
	}
	if root.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != parts[0] {
			continue
		}
		if len(parts) == 1 {
			root.Content = append(root.Content[:i], root.Content[i+2:]...)
			return true
		}
		return deleteNodePath(root.Content[i+1], parts[1:])
	}
	return false
}

func writeOwnedConfig(path string, root *yaml.Node, serverFile bool) error {
	dir := filepath.Dir(path)
	if serverFile {
		if err := secureConfigDirectory(dir); err != nil {
			return fmt.Errorf("secure config directory: %w", err)
		}
	} else {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create config area directory: %w", err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("secure config area directory: %w", err)
		}
	}
	data, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}})
	if err != nil {
		return fmt.Errorf("encode config %s: %w", filepath.Base(path), err)
	}
	if serverFile {
		data = append([]byte(canonicalServerConfigHeader), data...)
	}
	if err := writeConfigAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", filepath.Base(path), err)
	}
	return nil
}

func refreshGlobalConfig() {
	cfg, err := LoadConfig()
	if err == nil {
		SetGlobalConfig(cfg)
	}
}
