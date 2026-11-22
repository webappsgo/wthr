package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var secretsFileMu sync.Mutex

// ReadSecret returns a dotted secret value from secrets.yml without exposing
// the file contents through the general configuration API.
func ReadSecret(key string) (string, error) {
	value, exists, err := ReadSecretValue(key)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	text, _ := value.(string)
	return text, nil
}

// ReadSecretValue reads any scalar, mapping, or sequence stored at a dotted
// secret path and reports whether the path exists.
func ReadSecretValue(key string) (any, bool, error) {
	secretsFileMu.Lock()
	defer secretsFileMu.Unlock()
	values, err := readSecretsFile()
	if err != nil {
		return nil, false, err
	}
	var current any = values
	for _, part := range strings.Split(key, ".") {
		switch typed := current.(type) {
		case map[string]interface{}:
			var exists bool
			current, exists = typed[part]
			if !exists {
				return nil, false, nil
			}
		case []interface{}:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false, nil
			}
			current = typed[index]
		default:
			return nil, false, nil
		}
	}
	return current, current != nil, nil
}

// WriteSecret atomically stores a dotted secret value in secrets.yml with
// owner-only permissions.
func WriteSecret(key, value string) error {
	if value == "" {
		return fmt.Errorf("secret value is required")
	}
	return WriteSecretValue(key, value)
}

// WriteSecretValue stores any YAML value at a dotted secret path atomically.
func WriteSecretValue(key string, value any) error {
	if strings.TrimSpace(key) == "" || value == nil {
		return fmt.Errorf("secret key and value are required")
	}
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode secret value: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(encoded, &document); err != nil {
		return fmt.Errorf("parse secret value: %w", err)
	}
	valueNode, err := rootValue(&document)
	if err != nil {
		return fmt.Errorf("parse secret value: %w", err)
	}
	return persistSecretConfigValues([]secretConfigValue{{key: key, value: valueNode}})
}

// ReadClusterSharedSecrets returns only generated credentials that AI.md
// explicitly shares during the authenticated cluster join handshake.
func ReadClusterSharedSecrets() (map[string]any, error) {
	secretsFileMu.Lock()
	defer secretsFileMu.Unlock()
	values, err := readSecretsFile()
	if err != nil {
		return nil, err
	}
	shared := make(map[string]any)
	flattenClusterSecrets(values, "", shared)
	return shared, nil
}

// WriteClusterSharedSecrets replaces only keys supplied by the authenticated
// join payload and preserves every unrelated local secret.
func WriteClusterSharedSecrets(values map[string]any) error {
	secretsFileMu.Lock()
	defer secretsFileMu.Unlock()
	root, err := readSecretsFile()
	if err != nil {
		return err
	}
	for key, value := range values {
		if !isClusterSharedSecretKey(key) || value == nil {
			return fmt.Errorf("invalid shared secret key")
		}
		encoded, err := yaml.Marshal(value)
		if err != nil {
			return err
		}
		var document yaml.Node
		if err := yaml.Unmarshal(encoded, &document); err != nil {
			return err
		}
		valueNode, err := rootValue(&document)
		if err != nil {
			return err
		}
		var decoded any
		if err := valueNode.Decode(&decoded); err != nil {
			return err
		}
		if err := setSecretValue(root, strings.Split(key, "."), decoded); err != nil {
			return err
		}
	}
	data, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("encode secrets.yml: %w", err)
	}
	path := secretsConfigPath()
	dir := filepath.Dir(path)
	if err := secureConfigDirectory(dir); err != nil {
		return err
	}
	return writeConfigAtomic(filepath.Join(dir, "secrets.yml"), data, 0o600)
}

func flattenClusterSecrets(node map[string]interface{}, prefix string, values map[string]any) {
	for key, value := range node {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if nested, ok := value.(map[string]interface{}); ok {
			flattenClusterSecrets(nested, path, values)
			continue
		}
		if isClusterSharedSecretKey(path) {
			values[path] = value
		}
	}
}

func isClusterSharedSecretKey(key string) bool {
	return key == "server.token" || key == "server.security.encryption_key" || key == "server.security.encryption_key_version" ||
		strings.HasPrefix(key, "server.metrics.auth.tokens.") ||
		strings.Contains(key, ".contact.") && strings.Contains(key, ".webhooks.") && strings.HasSuffix(key, "_secret")
}

// DeleteSecret removes a dotted secret from secrets.yml using the same atomic,
// owner-only write guarantees as WriteSecret.
func DeleteSecret(key string) error {
	secretsFileMu.Lock()
	defer secretsFileMu.Unlock()
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("secret key is required")
	}
	values, err := readSecretsFile()
	if err != nil {
		return err
	}
	parts := strings.Split(key, ".")
	current := values
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	delete(current, parts[len(parts)-1])
	data, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("encode secrets.yml: %w", err)
	}
	configPath := secretsConfigPath()
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".secrets-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary secrets file: %w", err)
	}
	path := temporary.Name()
	defer func() { _ = os.Remove(path) }()
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary secrets file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write secrets.yml: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync secrets.yml: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close secrets.yml: %w", err)
	}
	if err := os.Rename(path, filepath.Join(dir, "secrets.yml")); err != nil {
		return fmt.Errorf("replace secrets.yml: %w", err)
	}
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func readSecretsFile() (map[string]interface{}, error) {
	configPath := secretsConfigPath()
	path := filepath.Join(filepath.Dir(configPath), "secrets.yml")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return make(map[string]interface{}), nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect secrets.yml: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("secrets.yml is not a regular file")
	}
	if info.Mode().Perm() != 0600 {
		if err := os.Chmod(path, 0600); err != nil {
			return nil, fmt.Errorf("secure secrets.yml: %w", err)
		}
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]interface{}), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read secrets.yml: %w", err)
	}
	values := make(map[string]interface{})
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse secrets.yml: %w", err)
	}
	return values, nil
}

func secretsConfigPath() string {
	if configDir := strings.TrimSpace(os.Getenv("CONFIG_DIR")); configDir != "" {
		return filepath.Join(configDir, "server.yml")
	}
	if configPath := findConfigFile(); configPath != "" {
		return configPath
	}
	return getConfigPath()
}
