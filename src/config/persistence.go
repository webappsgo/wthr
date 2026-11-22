package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var canonicalConfigAreas = map[string]string{
	"branding":      "server.branding server.seo web.ui",
	"web":           "web server.cors server.csrf server.url_detection server.compression server.healthz server.i18n",
	"pages":         "server.pages",
	"contact":       "server.contact",
	"privacy":       "server.privacy server.compliance server.tracking",
	"security":      "server.security server.rate_limit server.geoip server.limits server.tokens data",
	"logs":          "server.logs",
	"metrics":       "server.metrics",
	"notifications": "server.notifications",
	"tor":           "server.tor tor",
	"i2p":           "server.i2p",
	"maintenance":   "server.maintenance server.scheduler server.update server.auto_update server.backup server.restore",
	"accounts":      "server.auth server.users server.orgs server.session server.admin server.admin_path",
}

var nodeConfigPrefixes = []string{
	"server.port", "server.address", "server.fqdn", "server.mode", "server.api_version", "server.baseurl",
	"server.pidfile", "server.daemonize", "server.debug", "server.database", "server.cache", "server.ssl",
	"server.trusted_proxies", "server.cluster",
}

const canonicalServerConfigHeader = "# This node only: listen, identity, database, cache, ssl. The rest is in yaml.d/ and credentials are in secrets.yml.\n\n"

var errUnsafeSecretMigration = errors.New("could not safely migrate credentials")

// CanonicalConfigOwner returns the PART 5 file that owns a dotted config key.
func CanonicalConfigOwner(key string) string {
	if key == "smtp.password" {
		key = "server.notifications.email.smtp.password"
	}
	if isSecretConfigKey(key, "") {
		return "secrets.yml"
	}
	for _, prefix := range nodeConfigPrefixes {
		if key == prefix || strings.HasPrefix(key, prefix+".") {
			return "server.yml"
		}
	}
	owner, ownerLength := "", 0
	for filename, prefixes := range canonicalConfigAreas {
		for _, prefix := range strings.Fields(prefixes) {
			if (key == prefix || strings.HasPrefix(key, prefix+".")) && len(prefix) > ownerLength {
				owner = filepath.Join(DropinDirName, filename+".yml")
				ownerLength = len(prefix)
			}
		}
	}
	if owner != "" {
		return owner
	}
	return filepath.Join(DropinDirName, "app.yml")
}

func saveConfigFiles(configPath string, configData []byte) error {
	var configDoc yaml.Node
	if err := yaml.Unmarshal(configData, &configDoc); err != nil {
		return fmt.Errorf("parse marshaled config: %w", err)
	}
	configRoot, err := rootMapping(&configDoc)
	if err != nil {
		return err
	}
	if configRoot == nil {
		configRoot = newMappingNode()
	}

	configDir := filepath.Dir(configPath)
	if err := secureConfigDirectory(configDir); err != nil {
		return fmt.Errorf("secure config directory: %w", err)
	}
	areaDir := DropinDir(configPath)
	if err := os.MkdirAll(areaDir, 0o700); err != nil {
		return fmt.Errorf("create config area directory: %w", err)
	}
	if err := os.Chmod(areaDir, 0o700); err != nil {
		return fmt.Errorf("secure config area directory: %w", err)
	}

	_, report, reportErr := loadLayers(configPath, false)
	if reportErr != nil {
		report.Sources = nil
	}

	serverRoot := newMappingNode()
	areaRoots := make(map[string]*yaml.Node, len(canonicalConfigAreas)+1)
	for name := range canonicalConfigAreas {
		areaPath := filepath.Join(areaDir, name+".yml")
		areaRoots[name], err = readConfigRoot(areaPath)
		if err != nil {
			return err
		}
	}
	appPath := filepath.Join(areaDir, "app.yml")
	areaRoots["app"], err = readConfigRoot(appPath)
	if err != nil {
		return err
	}
	secrets := make([]secretConfigValue, 0)
	areaNames := make([]string, 0, len(areaRoots))
	for name := range areaRoots {
		areaNames = append(areaNames, name)
	}
	sort.Strings(areaNames)
	for _, areaName := range areaNames {
		areaRoot := areaRoots[areaName]
		walkConfigLeaves(areaRoot, "", func(key string, value *yaml.Node) {
			if isSecretConfigKey(key, value.Value) {
				secrets = append(secrets, secretConfigValue{key: key, value: cloneYAMLNode(value)})
				_ = setNodePath(areaRoot, strings.Split(key, "."), scalarNode(""))
			}
		})
		areaRoots[areaName] = areaRoot
	}

	if oldData, readErr := os.ReadFile(configPath); readErr == nil {
		var oldDoc yaml.Node
		if err := yaml.Unmarshal(oldData, &oldDoc); err != nil {
			return fmt.Errorf("parse existing server config: %w", err)
		}
		oldRoot, rootErr := rootMapping(&oldDoc)
		if rootErr != nil {
			return fmt.Errorf("parse existing server config: %w", rootErr)
		}
		if oldRoot != nil {
			var setErr error
			walkConfigLeaves(oldRoot, "", func(key string, value *yaml.Node) {
				if setErr != nil {
					return
				}
				if isSecretConfigKey(key, value.Value) {
					secrets = append(secrets, secretConfigValue{key: key, value: cloneYAMLNode(value)})
					return
				}
				owner := CanonicalConfigOwner(key)
				if owner == "server.yml" {
					return
				}
				areaName := strings.TrimSuffix(filepath.Base(owner), ".yml")
				area := areaRoots[areaName]
				if area != nil && nodeAtPath(area, strings.Split(key, ".")) == nil {
					setErr = setNodePath(area, strings.Split(key, "."), cloneYAMLNode(value))
				}
			})
			if setErr != nil {
				return fmt.Errorf("move legacy config setting: %w", setErr)
			}
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read existing server config: %w", readErr)
	}

	var setErr error
	walkConfigLeaves(configRoot, "", func(key string, value *yaml.Node) {
		if setErr != nil {
			return
		}
		if isSecretConfigKey(key, value.Value) {
			secrets = append(secrets, secretConfigValue{key: key, value: cloneYAMLNode(value)})
			return
		}
		owner := CanonicalConfigOwner(key)
		if source := sourceForConfigKey(report.Sources, key); source != "" && source != owner {
			return
		}
		if owner == "server.yml" {
			setErr = setNodePath(serverRoot, strings.Split(key, "."), cloneYAMLNode(value))
			return
		}
		areaName := strings.TrimSuffix(filepath.Base(owner), ".yml")
		area := areaRoots[areaName]
		if area == nil {
			area = newMappingNode()
			areaRoots[areaName] = area
		}
		setErr = setNodePath(area, strings.Split(key, "."), cloneYAMLNode(value))
	})
	if setErr != nil {
		return fmt.Errorf("write config setting: %w", setErr)
	}
	if err := persistSecretConfigValues(secrets); err != nil {
		return err
	}

	for _, name := range areaNames {
		root := areaRoots[name]
		if root == nil || len(root.Content) == 0 {
			continue
		}
		data, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}})
		if err != nil {
			return fmt.Errorf("encode %s config: %w", name, err)
		}
		path := filepath.Join(areaDir, name+".yml")
		if err := writeConfigAtomic(path, data, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	serverData, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{serverRoot}})
	if err != nil {
		return fmt.Errorf("encode server config: %w", err)
	}
	serverData = append([]byte(canonicalServerConfigHeader), serverData...)
	if err := writeConfigAtomic(configPath, serverData, 0o600); err != nil {
		return fmt.Errorf("write server config: %w", err)
	}
	return nil
}

func migrateConfigAreas(configPath string) (migrationErr error) {
	hasPlaintextSecret := false
	defer func() {
		if migrationErr != nil && hasPlaintextSecret && !errors.Is(migrationErr, errUnsafeSecretMigration) {
			migrationErr = fmt.Errorf("%w: %v", errUnsafeSecretMigration, migrationErr)
		}
	}()
	if err := secureConfigDirectory(filepath.Dir(configPath)); err != nil {
		return fmt.Errorf("%w: secure config directory before migration: %v", errUnsafeSecretMigration, err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read server config for area migration: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse server config for area migration: %w", err)
	}
	originalRoot, err := rootMapping(&doc)
	if err != nil || originalRoot == nil {
		return err
	}
	serverRoot := cloneYAMLNode(originalRoot)
	type migrationValue struct {
		key   string
		owner string
		value *yaml.Node
	}
	values := make([]migrationValue, 0)
	walkConfigLeaves(originalRoot, "", func(key string, value *yaml.Node) {
		owner := CanonicalConfigOwner(key)
		if owner != "server.yml" {
			if owner == "secrets.yml" {
				hasPlaintextSecret = true
			}
			values = append(values, migrationValue{key: key, owner: owner, value: cloneYAMLNode(value)})
		}
	})
	if len(values) == 0 {
		return migrateDropinSecrets(configPath)
	}

	areaDir := DropinDir(configPath)
	if err := os.MkdirAll(areaDir, 0o700); err != nil {
		return fmt.Errorf("create config area directory: %w", err)
	}
	if err := os.Chmod(areaDir, 0o700); err != nil {
		return fmt.Errorf("secure config area directory: %w", err)
	}
	areaRoots := make(map[string]*yaml.Node)
	secretValues := make([]secretConfigValue, 0)
	secretRoot, err := readSecretsFile()
	if err != nil {
		for _, item := range values {
			if item.owner == "secrets.yml" {
				return fmt.Errorf("%w: read existing secrets: %v", errUnsafeSecretMigration, err)
			}
		}
		return err
	}
	for _, item := range values {
		if item.owner == "secrets.yml" {
			if !secretPathExists(secretRoot, strings.Split(item.key, ".")) {
				secretValues = append(secretValues, secretConfigValue{key: item.key, value: item.value})
			}
			removeNodePath(serverRoot, strings.Split(item.key, "."))
			continue
		}
		areaName := strings.TrimSuffix(filepath.Base(item.owner), ".yml")
		areaRoot := areaRoots[areaName]
		if areaRoot == nil {
			areaRoot, err = readConfigRoot(filepath.Join(areaDir, areaName+".yml"))
			if err != nil {
				return err
			}
			areaRoots[areaName] = areaRoot
		}
		parts := strings.Split(item.key, ".")
		if nodeAtPath(areaRoot, parts) == nil {
			if err := setNodePath(areaRoot, parts, item.value); err != nil {
				return fmt.Errorf("move %s to %s: %w", item.key, item.owner, err)
			}
		}
		removeNodePath(serverRoot, parts)
	}

	backupRoot := cloneYAMLNode(originalRoot)
	var secretPaths []string
	walkConfigLeaves(originalRoot, "", func(key string, value *yaml.Node) {
		if isSecretConfigKey(key, value.Value) {
			secretPaths = append(secretPaths, key)
		}
	})
	for _, key := range secretPaths {
		removeNodePath(backupRoot, strings.Split(key, "."))
	}
	backupData, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{backupRoot}})
	if err != nil {
		return fmt.Errorf("encode redacted server config backup: %w", err)
	}
	if err := writeConfigAtomic(configPath+".bak", backupData, 0o600); err != nil {
		if len(secretValues) > 0 {
			return fmt.Errorf("%w: back up server config before secret migration: %v", errUnsafeSecretMigration, err)
		}
		return fmt.Errorf("back up server config before area migration: %w", err)
	}
	if err := persistSecretConfigValues(secretValues); err != nil {
		if len(secretValues) > 0 {
			return fmt.Errorf("%w: write secrets file: %v", errUnsafeSecretMigration, err)
		}
		return err
	}
	areaNames := make([]string, 0, len(areaRoots))
	for name := range areaRoots {
		areaNames = append(areaNames, name)
	}
	sort.Strings(areaNames)
	for _, name := range areaNames {
		root := areaRoots[name]
		encoded, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}})
		if err != nil {
			return fmt.Errorf("encode migrated %s config: %w", name, err)
		}
		if err := writeConfigAtomic(filepath.Join(areaDir, name+".yml"), encoded, 0o600); err != nil {
			if len(secretValues) > 0 {
				return fmt.Errorf("%w: finish writing migrated files: %v", errUnsafeSecretMigration, err)
			}
			return fmt.Errorf("write migrated %s config: %w", name, err)
		}
	}
	serverData, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{serverRoot}})
	if err != nil {
		return fmt.Errorf("encode server config after area migration: %w", err)
	}
	serverData = append([]byte(canonicalServerConfigHeader), serverData...)
	if err := writeConfigAtomic(configPath, serverData, 0o600); err != nil {
		if len(secretValues) > 0 {
			return fmt.Errorf("%w: remove migrated credentials from server config: %v", errUnsafeSecretMigration, err)
		}
		return fmt.Errorf("rewrite server config after area migration: %w", err)
	}
	return migrateDropinSecrets(configPath)
}

// migrateDropinSecrets moves credentials from loaded operator drop-ins to
// secrets.yml. Existing secrets.yml values win, and among drop-ins the last
// file in configured merge order wins. Secret storage is written before any
// source drop-in is rewritten, making an interrupted migration retry-safe.
func migrateDropinSecrets(configPath string) error {
	files, warnings := listDropinFiles(DropinDir(configPath))
	for _, warning := range warnings {
		if strings.Contains(warning, ": refused,") {
			return fmt.Errorf("%w: %s", errUnsafeSecretMigration, warning)
		}
	}
	type sourceFile struct {
		path string
		root *yaml.Node
	}
	sources := make([]sourceFile, 0, len(files))
	winners := make(map[string]*yaml.Node)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%w: read %s: %v", errUnsafeSecretMigration, filepath.Base(path), err)
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(data, &doc); err != nil {
			// The normal loader ignores malformed files. Report that migration
			// could not inspect this source without printing potentially secret
			// parser context or changing the operator's recovery file.
			fmt.Fprintf(os.Stderr, "Warning: could not inspect malformed %s/%s during credential migration; the normal config loader will ignore it\n", DropinDirName, filepath.Base(path))
			continue
		}
		root, err := rootMapping(&doc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not inspect %s/%s during credential migration: top-level YAML value must be a mapping\n", DropinDirName, filepath.Base(path))
			continue
		}
		if root == nil {
			continue
		}
		sources = append(sources, sourceFile{path: path, root: cloneYAMLNode(root)})
		walkConfigLeaves(root, "", func(key string, value *yaml.Node) {
			if isSecretConfigKey(key, value.Value) {
				winners[key] = cloneYAMLNode(value)
			}
		})
	}
	if len(winners) == 0 {
		return nil
	}
	secretRoot, err := readSecretsFile()
	if err != nil {
		return fmt.Errorf("%w: read existing secrets: %v", errUnsafeSecretMigration, err)
	}
	secretValues := make([]secretConfigValue, 0, len(winners))
	for key, value := range winners {
		if !secretPathExists(secretRoot, strings.Split(key, ".")) {
			secretValues = append(secretValues, secretConfigValue{key: key, value: value})
		}
	}
	if err := persistSecretConfigValues(secretValues); err != nil {
		return fmt.Errorf("%w: write secrets file: %v", errUnsafeSecretMigration, err)
	}
	changed := make([]sourceFile, 0)
	for _, source := range sources {
		var keys []string
		walkConfigLeaves(source.root, "", func(key string, value *yaml.Node) {
			if isSecretConfigKey(key, value.Value) {
				keys = append(keys, key)
			}
		})
		if len(keys) == 0 {
			continue
		}
		for _, key := range keys {
			removeNodePath(source.root, strings.Split(key, "."))
		}
		changed = append(changed, source)
	}
	for _, source := range changed {
		encoded, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{source.root}})
		if err != nil {
			return fmt.Errorf("%w: encode %s: %v", errUnsafeSecretMigration, filepath.Base(source.path), err)
		}
		if err := writeConfigAtomic(source.path, encoded, 0o600); err != nil {
			return fmt.Errorf("%w: remove credentials from %s: %v", errUnsafeSecretMigration, filepath.Base(source.path), err)
		}
	}
	return nil
}

func secureConfigDirectory(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	return nil
}

func secretPathExists(root interface{}, parts []string) bool {
	current := root
	for _, part := range parts {
		switch typed := current.(type) {
		case map[string]interface{}:
			var exists bool
			current, exists = typed[part]
			if !exists {
				return false
			}
		case []interface{}:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return false
			}
			current = typed[index]
		default:
			return false
		}
	}
	return current != nil
}

func removeNodePath(root *yaml.Node, parts []string) {
	if root == nil || len(parts) == 0 {
		return
	}
	current := root
	for _, part := range parts[:len(parts)-1] {
		if current.Kind == yaml.SequenceNode {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current.Content) {
				return
			}
			current = current.Content[index]
			continue
		}
		current, _ = mappingValue(current, part)
		if current == nil {
			return
		}
	}
	if current.Kind == yaml.SequenceNode {
		index, err := strconv.Atoi(parts[len(parts)-1])
		if err == nil && index >= 0 && index < len(current.Content) {
			current.Content[index] = scalarNode("")
		}
		return
	}
	removeMappingKey(current, parts[len(parts)-1])
	pruneEmptyMappings(root)
}

func pruneEmptyMappings(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	kept := node.Content[:0]
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if value.Kind == yaml.MappingNode && pruneEmptyMappings(value) {
			continue
		}
		kept = append(kept, key, value)
	}
	node.Content = kept
	return len(node.Content) == 0
}

func mergeSecretsIntoConfig(merged *yaml.Node) error {
	values, err := readSecretsFile()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("encode secrets layer: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse secrets layer: %w", err)
	}
	root, err := rootMapping(&doc)
	if err != nil || root == nil {
		return err
	}
	return mergeSecretNodes(merged, root)
}

func mergeSecretNodes(dst, src *yaml.Node) error {
	if dst == nil || src == nil {
		return nil
	}
	if dst.Kind == yaml.MappingNode && src.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(src.Content); i += 2 {
			key := src.Content[i].Value
			value := src.Content[i+1]
			current, exists := mappingValue(dst, key)
			if !exists {
				appendMapping(dst, cloneYAMLNode(src.Content[i]), cloneYAMLNode(value))
				continue
			}
			if err := mergeSecretNodes(current, value); err != nil {
				return err
			}
		}
		return nil
	}
	if dst.Kind == yaml.SequenceNode && src.Kind == yaml.SequenceNode {
		for index, value := range src.Content {
			if index >= len(dst.Content) {
				dst.Content = append(dst.Content, cloneYAMLNode(value))
				continue
			}
			if err := mergeSecretNodes(dst.Content[index], value); err != nil {
				return err
			}
		}
		return nil
	}
	*dst = *cloneYAMLNode(src)
	return nil
}

type secretConfigValue struct {
	key       string
	sourceKey string
	value     *yaml.Node
}

func persistSecretConfigValues(values []secretConfigValue) error {
	stored := make([]secretConfigValue, 0, len(values))
	for _, value := range values {
		if value.value != nil && value.value.Tag != "!!null" {
			stored = append(stored, value)
		}
	}
	if len(stored) == 0 {
		return nil
	}
	secretsFileMu.Lock()
	defer secretsFileMu.Unlock()

	root, err := readSecretsFile()
	if err != nil {
		return err
	}
	for _, item := range stored {
		var decoded interface{}
		if err := item.value.Decode(&decoded); err != nil {
			return fmt.Errorf("decode secret %s: %w", item.key, err)
		}
		if err := setSecretValue(root, strings.Split(item.key, "."), decoded); err != nil {
			return fmt.Errorf("store secret %s: %w", item.key, err)
		}
	}
	data, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("encode secrets config: %w", err)
	}
	path := secretsConfigPath()
	dir := filepath.Dir(path)
	if err := secureConfigDirectory(dir); err != nil {
		return fmt.Errorf("secure secrets directory: %w", err)
	}
	if err := writeConfigAtomic(filepath.Join(dir, "secrets.yml"), data, 0o600); err != nil {
		return fmt.Errorf("write secrets file: %w", err)
	}
	return nil
}

func isSecretConfigKey(key, value string) bool {
	switch key {
	case "server.token", "server.security.encryption_key", "server.security.encryption_key_version",
		"server.notifications.email.smtp.password", "server.cache.password", "server.database.password",
		"server.database.token", "server.pages.contact.captcha_secret_key", "data.cve.api_key",
		"server.backup.encryption_password":
		return true
	}
	if key == "server.ssl.letsencrypt.dns_credentials" || strings.HasPrefix(key, "server.ssl.letsencrypt.dns_credentials.") ||
		strings.HasPrefix(key, "server.metrics.auth.tokens.") ||
		strings.HasPrefix(key, "server.auth.ldap.providers.") && strings.HasSuffix(key, ".bind_password") ||
		strings.HasPrefix(key, "server.auth.oidc.providers.") && strings.HasSuffix(key, ".client_secret") ||
		strings.Contains(key, ".contact.") && strings.Contains(key, ".webhooks.") &&
			(strings.HasSuffix(key, ".url") || strings.HasSuffix(key, "_url") || strings.HasSuffix(key, "_secret") || strings.HasSuffix(key, ".secret") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")) {
		return true
	}
	if key == "server.database.url" || key == "server.cache.url" {
		parsed, err := url.Parse(value)
		if err != nil {
			return false
		}
		if parsed.User != nil {
			if password, hasPassword := parsed.User.Password(); hasPassword && password != "" {
				return true
			}
		}
		for name, values := range parsed.Query() {
			if strings.EqualFold(name, "password") || strings.EqualFold(name, "pwd") || strings.EqualFold(name, "token") || strings.EqualFold(name, "authToken") {
				for _, queryValue := range values {
					if queryValue != "" {
						return true
					}
				}
			}
		}
	}
	return false
}

func walkConfigLeaves(node *yaml.Node, prefix string, visit func(string, *yaml.Node)) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if prefix != "" {
				key = prefix + "." + key
			}
			walkConfigLeaves(node.Content[i+1], key, visit)
		}
	case yaml.SequenceNode:
		for i, value := range node.Content {
			walkConfigLeaves(value, prefix+"."+strconv.Itoa(i), visit)
		}
	default:
		visit(prefix, node)
	}
}

func readConfigRoot(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newMappingNode(), nil
		}
		return nil, fmt.Errorf("read config area %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse config area %s: %w", path, err)
	}
	root, err := rootMapping(&doc)
	if err != nil || root == nil {
		if err != nil {
			return nil, fmt.Errorf("parse config area %s: %w", path, err)
		}
		return newMappingNode(), nil
	}
	return cloneYAMLNode(root), nil
}

func nodeAtPath(root *yaml.Node, parts []string) *yaml.Node {
	current := root
	for _, part := range parts {
		if current == nil {
			return nil
		}
		if current.Kind == yaml.SequenceNode {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current.Content) {
				return nil
			}
			current = current.Content[index]
			continue
		}
		current, _ = mappingValue(current, part)
	}
	return current
}

func setNodePath(root *yaml.Node, parts []string, value *yaml.Node) error {
	if len(parts) == 0 {
		return fmt.Errorf("empty config key")
	}
	current := root
	for i, part := range parts {
		last := i == len(parts)-1
		if current.Kind == yaml.SequenceNode {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index > 10000 {
				return fmt.Errorf("invalid sequence index %q", part)
			}
			for len(current.Content) <= index {
				current.Content = append(current.Content, newMappingNode())
			}
			if last {
				current.Content[index] = value
				return nil
			}
			current = current.Content[index]
			continue
		}
		if current.Kind != yaml.MappingNode {
			return fmt.Errorf("config key crosses a non-mapping node")
		}
		child, exists := mappingValue(current, part)
		if last {
			if exists {
				for j := 0; j+1 < len(current.Content); j += 2 {
					if current.Content[j].Value == part {
						current.Content[j+1] = value
						return nil
					}
				}
			}
			appendMapping(current, scalarNode(part), value)
			return nil
		}
		if !exists {
			if _, err := strconv.Atoi(parts[i+1]); err == nil {
				child = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			} else {
				child = newMappingNode()
			}
			appendMapping(current, scalarNode(part), child)
		}
		current = child
	}
	return nil
}

func setSecretValue(root map[string]interface{}, parts []string, value interface{}) error {
	_, err := setSecretPath(root, parts, value)
	return err
}

func setSecretPath(current interface{}, parts []string, value interface{}) (interface{}, error) {
	if len(parts) == 0 {
		return value, nil
	}
	switch typed := current.(type) {
	case map[string]interface{}:
		part := parts[0]
		if len(parts) == 1 {
			typed[part] = value
			return typed, nil
		}
		child := typed[part]
		if child == nil {
			if _, err := strconv.Atoi(parts[1]); err == nil {
				child = []interface{}{}
			} else {
				child = make(map[string]interface{})
			}
		}
		updated, err := setSecretPath(child, parts[1:], value)
		if err != nil {
			return nil, err
		}
		typed[part] = updated
		return typed, nil
	case []interface{}:
		index, err := strconv.Atoi(parts[0])
		if err != nil || index < 0 || index > 10000 {
			return nil, fmt.Errorf("invalid sequence index %q", parts[0])
		}
		for len(typed) <= index {
			typed = append(typed, map[string]interface{}{})
		}
		updated, err := setSecretPath(typed[index], parts[1:], value)
		if err != nil {
			return nil, err
		}
		typed[index] = updated
		return typed, nil
	default:
		return nil, fmt.Errorf("secret path crosses non-container")
	}
}

func cloneYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	copy := *node
	copy.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		copy.Content[i] = cloneYAMLNode(child)
	}
	return &copy
}

func sourceForConfigKey(sources map[string]string, key string) string {
	if source := sources[key]; source != "" {
		return source
	}
	for sourceKey, source := range sources {
		if strings.HasPrefix(key, sourceKey+".") || strings.HasPrefix(sourceKey, key+".") {
			return source
		}
	}
	return ""
}
