package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

const serverConfigHeader = `# Settings of this node only (listen, identity, database, cache, ssl). Everything else lives in yaml.d/: branding.yml, web.yml, pages.yml, contact.yml, privacy.yml, security.yml, logs.yml, metrics.yml, notifications.yml, tor.yml, i2p.yml, maintenance.yml, accounts.yml, app.yml. Credentials live in secrets.yml.

`

// brandingAreaOwnedKeys describes the settings that belong in yaml.d/branding.yml.
func brandingAreaOwnedKeys() map[string]string {
	const owner = DropinDirName + "/branding.yml"
	return map[string]string{
		"server.branding": owner,
		"server.seo":      owner,
		"web.ui.theme":    owner,
		"web.ui.accent":   owner,
	}
}

// MigrateBrandingArea moves legacy branding settings out of server.yml while
// preserving values already set by yaml.d/branding.yml. It is safe to call on
// every startup and leaves the merged configuration unchanged.
func MigrateBrandingArea(configPath string) error {
	if configPath == "" {
		return nil
	}
	serverData, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read server config for branding migration: %w", err)
	}
	serverDoc, serverRoot, err := parseBrandingDocument(serverData)
	if err != nil {
		return fmt.Errorf("parse server config for branding migration: %w", err)
	}
	area := extractBrandingArea(serverRoot)
	if !hasMappingEntries(area) {
		return nil
	}

	yamlDir := DropinDir(configPath)
	brandingPath := filepath.Join(yamlDir, "branding.yml")
	brandingRoot := newMappingNode()
	brandingExists := false
	if existingData, readErr := os.ReadFile(brandingPath); readErr == nil {
		_, parsedRoot, parseErr := parseBrandingDocument(existingData)
		if parseErr != nil {
			return fmt.Errorf("parse %s for branding migration: %w", brandingPath, parseErr)
		}
		if parsedRoot != nil {
			brandingRoot = parsedRoot
		}
		brandingExists = true
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return fmt.Errorf("read %s for branding migration: %w", brandingPath, readErr)
	}

	changed := mergeMissingMappings(brandingRoot, area)

	if err := os.MkdirAll(yamlDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", yamlDir, err)
	}
	if err := os.Chmod(yamlDir, 0o700); err != nil {
		return fmt.Errorf("secure %s: %w", yamlDir, err)
	}
	if err := writeBrandingBackup(configPath+".bak", serverData); err != nil {
		return fmt.Errorf("back up server config before branding migration: %w", err)
	}

	if changed {
		brandingData, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{brandingRoot}})
		if err != nil {
			return fmt.Errorf("encode migrated branding config: %w", err)
		}
		if err := writeConfigAtomic(brandingPath, brandingData, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", brandingPath, err)
		}
	}

	removeBrandingArea(serverRoot)
	serverOut, err := yaml.Marshal(serverDoc)
	if err != nil {
		return fmt.Errorf("encode server config after branding migration: %w", err)
	}
	serverOut = append([]byte(serverConfigHeader), serverOut...)
	if err := writeConfigAtomic(configPath, serverOut, 0o600); err != nil {
		return fmt.Errorf("rewrite server config after branding migration: %w", err)
	}

	if !brandingExists {
		fmt.Fprintf(os.Stderr, "Moved branding, SEO and theme settings to %s\n", filepath.Join(DropinDirName, "branding.yml"))
	}
	return nil
}

// SaveBrandingArea writes branding, SEO and UI theme values atomically to the
// area file assigned to them by AI.md PART 16.
func SaveBrandingArea(cfg *AppConfig) error {
	if cfg == nil {
		return fmt.Errorf("save branding config: nil config")
	}
	configPath := findConfigFile()
	if configPath == "" {
		configPath = getConfigPath()
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode branding config: %w", err)
	}
	_, root, err := parseBrandingDocument(data)
	if err != nil {
		return fmt.Errorf("parse encoded branding config: %w", err)
	}
	area := extractBrandingArea(root)
	areaData, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{area}})
	if err != nil {
		return fmt.Errorf("encode branding area file: %w", err)
	}
	yamlDir := DropinDir(configPath)
	if err := os.MkdirAll(yamlDir, 0o700); err != nil {
		return fmt.Errorf("create branding area directory: %w", err)
	}
	if err := os.Chmod(yamlDir, 0o700); err != nil {
		return fmt.Errorf("secure branding area directory: %w", err)
	}
	path := filepath.Join(yamlDir, "branding.yml")
	if err := writeConfigAtomic(path, areaData, 0o600); err != nil {
		return fmt.Errorf("write branding area file: %w", err)
	}
	if _, report, err := loadLayers(configPath, false); err == nil {
		setLayerReport(report)
	}
	return nil
}

// ValidateBrandingConfig normalizes branding values according to AI.md PART 5
// and PART 16 before a setup or admin write.
func ValidateBrandingConfig(cfg *AppConfig) {
	if cfg == nil {
		return
	}
	validateServerBranding(cfg)
	validateServerSEO(cfg)
}

func parseBrandingDocument(data []byte) (*yaml.Node, *yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, err
	}
	root, err := rootMapping(&doc)
	if err != nil {
		return nil, nil, err
	}
	return &doc, root, nil
}

func extractBrandingArea(source *yaml.Node) *yaml.Node {
	area := newMappingNode()
	if source == nil {
		return area
	}
	if server, ok := mappingValue(source, "server"); ok && server.Kind == yaml.MappingNode {
		serverArea := newMappingNode()
		for _, key := range []string{"branding", "seo"} {
			if value, found := mappingValue(server, key); found {
				appendMapping(serverArea, scalarNode(key), value)
			}
		}
		if len(serverArea.Content) != 0 {
			appendMapping(area, scalarNode("server"), serverArea)
		}
	}
	if web, ok := mappingValue(source, "web"); ok && web.Kind == yaml.MappingNode {
		if ui, ok := mappingValue(web, "ui"); ok && ui.Kind == yaml.MappingNode {
			uiArea := newMappingNode()
			for _, key := range []string{"theme", "accent"} {
				if value, found := mappingValue(ui, key); found {
					appendMapping(uiArea, scalarNode(key), value)
				}
			}
			if len(uiArea.Content) != 0 {
				webArea := newMappingNode()
				appendMapping(webArea, scalarNode("ui"), uiArea)
				appendMapping(area, scalarNode("web"), webArea)
			}
		}
	}
	return area
}

func removeBrandingArea(root *yaml.Node) {
	server, ok := mappingValue(root, "server")
	if ok && server.Kind == yaml.MappingNode {
		removeMappingKey(server, "branding")
		removeMappingKey(server, "seo")
		if len(server.Content) == 0 {
			removeMappingKey(root, "server")
		}
	}
	web, ok := mappingValue(root, "web")
	if !ok || web.Kind != yaml.MappingNode {
		return
	}
	ui, ok := mappingValue(web, "ui")
	if !ok || ui.Kind != yaml.MappingNode {
		return
	}
	removeMappingKey(ui, "theme")
	removeMappingKey(ui, "accent")
	if len(ui.Content) == 0 {
		removeMappingKey(web, "ui")
	}
	if len(web.Content) == 0 {
		removeMappingKey(root, "web")
	}
}

func mergeMissingMappings(destination, source *yaml.Node) bool {
	changed := false
	if destination == nil || source == nil || destination.Kind != yaml.MappingNode || source.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(source.Content); i += 2 {
		key, value := source.Content[i], source.Content[i+1]
		existing, ok := mappingValue(destination, key.Value)
		if !ok {
			appendMapping(destination, key, value)
			changed = true
			continue
		}
		if existing.Kind == yaml.MappingNode && value.Kind == yaml.MappingNode && mergeMissingMappings(existing, value) {
			changed = true
		}
	}
	return changed
}

func mappingValue(node *yaml.Node, key string) (*yaml.Node, bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], true
		}
	}
	return nil, false
}

func appendMapping(node, key, value *yaml.Node) {
	node.Content = append(node.Content, key, value)
}

func removeMappingKey(node *yaml.Node, key string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	kept := node.Content[:0]
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			continue
		}
		kept = append(kept, node.Content[i], node.Content[i+1])
	}
	node.Content = kept
}

func newMappingNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func hasMappingEntries(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.MappingNode && len(node.Content) != 0
}

func writeBrandingBackup(path string, data []byte) error {
	if err := writeConfigAtomic(path, data, 0o600); err != nil {
		return err
	}
	return nil
}

func writeConfigAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer func() {
		if err := os.Remove(tempPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "Warning: remove temporary config file %s: %v\n", tempPath, err)
		}
	}()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	written, err := file.Write(data)
	if err != nil {
		_ = file.Close()
		return err
	}
	if written != len(data) {
		_ = file.Close()
		return fmt.Errorf("short write: wrote %d of %d bytes", written, len(data))
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := directory.Sync(); err != nil {
			_ = directory.Close()
			return err
		}
	}
	return directory.Close()
}
