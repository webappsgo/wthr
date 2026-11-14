package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// DropinDirName is the optional drop-in directory next to server.yml
// (AI.md PART 5, "Drop-in Config Directory").
const DropinDirName = "yaml.d"

// DropinOverride records a setting that a drop-in file overrode: the dotted key
// and the file that won, so the startup log can name it.
type DropinOverride struct {
	Key  string
	File string
}

// LayerReport describes how the last configuration load was assembled from
// server.yml and the yaml.d drop-ins.
type LayerReport struct {
	// Files are the loaded drop-ins in merge order, as "yaml.d/{name}".
	Files []string
	// Overrides lists keys set by more than one layer; the last file wins.
	Overrides []DropinOverride
	// Sources maps every dotted key set by a drop-in to the winning file.
	Sources map[string]string
	// Warnings are drop-ins that were skipped (unreadable, unsafe permissions).
	Warnings []string
}

var (
	layerReportMu sync.RWMutex
	layerReport   LayerReport
)

// GetLayerReport returns a copy of the report from the most recent load.
func GetLayerReport() LayerReport {
	layerReportMu.RLock()
	defer layerReportMu.RUnlock()
	out := LayerReport{
		Files:     append([]string(nil), layerReport.Files...),
		Overrides: append([]DropinOverride(nil), layerReport.Overrides...),
		Warnings:  append([]string(nil), layerReport.Warnings...),
		Sources:   make(map[string]string, len(layerReport.Sources)),
	}
	for k, v := range layerReport.Sources {
		out.Sources[k] = v
	}
	return out
}

func setLayerReport(r LayerReport) {
	layerReportMu.Lock()
	layerReport = r
	layerReportMu.Unlock()
}

// DropinSource returns the drop-in file that owns the dotted setting key, or an
// empty string when no drop-in sets it. A key also counts as owned when a
// drop-in sets one of its parents or children as a whole value.
func DropinSource(key string) string {
	layerReportMu.RLock()
	defer layerReportMu.RUnlock()
	if file, ok := layerReport.Sources[key]; ok {
		return file
	}
	for owned, file := range layerReport.Sources {
		if strings.HasPrefix(key, owned+".") || strings.HasPrefix(owned, key+".") {
			return file
		}
	}
	return ""
}

// DropinDir returns the yaml.d directory that belongs to configPath.
func DropinDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), DropinDirName)
}

// listDropinFiles returns the usable drop-in files in byte-wise filename order.
// Only *.yml and *.yaml regular files qualify; hidden files, backups, other
// extensions and subdirectories are ignored. A symlink is followed only when
// its target stays inside the directory, and a file that is group or world
// writable is refused.
func listDropinFiles(dir string) (files, warnings []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", DropinDirName, err)}
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") ||
			strings.HasSuffix(name, ".bak") || strings.HasSuffix(name, ".orig") {
			continue
		}
		if ext := filepath.Ext(name); ext != ".yml" && ext != ".yaml" {
			continue
		}

		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err != nil || !pathInside(realDir, target) {
				warnings = append(warnings, fmt.Sprintf("%s/%s: symlink target is outside %s and was ignored", DropinDirName, name, DropinDirName))
				continue
			}
			info, err = os.Stat(target)
			if err != nil {
				continue
			}
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
			warnings = append(warnings, fmt.Sprintf("%s/%s: refused, the file is group or world writable", DropinDirName, name))
			continue
		}
		files = append(files, path)
	}
	return files, warnings
}

// pathInside reports whether target is dir itself or lies beneath it.
func pathInside(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// rootMapping returns the top-level mapping of a parsed YAML document, or nil
// when the document is empty.
func rootMapping(doc *yaml.Node) (*yaml.Node, error) {
	if doc == nil || doc.Kind == 0 {
		return nil, nil
	}
	root := doc
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			return nil, nil
		}
		root = doc.Content[0]
	}
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return nil, nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: the top level must be a mapping", root.Line)
	}
	return root, nil
}

// parseLayer parses one config file. A duplicate key anywhere in the file, a
// syntax error or a value of the wrong type is an error that names the file.
func parseLayer(label string, data []byte) (*yaml.Node, error) {
	var probe interface{}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	var typed AppConfig
	if err := yaml.Unmarshal(data, &typed); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	root, err := rootMapping(&doc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return root, nil
}

// leafPaths calls visit for every leaf under node. A scalar and a sequence are
// leaves; mappings are walked.
func leafPaths(node *yaml.Node, prefix string, visit func(path string, value *yaml.Node)) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		value := node.Content[i+1]
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if value.Kind == yaml.MappingNode {
			leafPaths(value, path, visit)
			continue
		}
		visit(path, value)
	}
}

// mergeLayer merges src into dst: maps merge key by key and recursively, while
// a scalar or a list in src replaces the earlier value whole. An explicit null
// is an error, never a reset.
func mergeLayer(dst, src *yaml.Node, prefix, label string) error {
	for i := 0; i+1 < len(src.Content); i += 2 {
		keyNode := src.Content[i]
		value := src.Content[i+1]
		path := keyNode.Value
		if prefix != "" {
			path = prefix + "." + keyNode.Value
		}
		if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
			return fmt.Errorf("%s: line %d: key %s: an explicit null is not allowed", label, value.Line, path)
		}

		index := -1
		for j := 0; j+1 < len(dst.Content); j += 2 {
			if dst.Content[j].Value == keyNode.Value {
				index = j
				break
			}
		}

		if index >= 0 && value.Kind == yaml.MappingNode && dst.Content[index+1].Kind == yaml.MappingNode {
			if err := mergeLayer(dst.Content[index+1], value, path, label); err != nil {
				return err
			}
			continue
		}
		if value.Kind == yaml.MappingNode {
			if err := rejectNulls(value, path, label); err != nil {
				return err
			}
		}
		if index >= 0 {
			dst.Content[index+1] = value
			continue
		}
		dst.Content = append(dst.Content, keyNode, value)
	}
	return nil
}

// rejectNulls fails on any explicit null nested under a mapping that is being
// copied in whole.
func rejectNulls(node *yaml.Node, prefix, label string) error {
	var failure error
	leafPaths(node, prefix, func(path string, value *yaml.Node) {
		if failure == nil && value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
			failure = fmt.Errorf("%s: line %d: key %s: an explicit null is not allowed", label, value.Line, path)
		}
	})
	return failure
}

// loadLayers reads server.yml (when configPath names one) and every drop-in and
// merges them into one mapping in precedence order. It returns the merged
// mapping and the report of what was loaded. With strict set, any invalid file
// is an error; otherwise an invalid drop-in is skipped with a warning and an
// invalid server.yml is an error the caller turns into the defaults.
func loadLayers(configPath string, strict bool) (*yaml.Node, LayerReport, error) {
	report := LayerReport{Sources: map[string]string{}}
	setBy := map[string]string{}

	merged := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, report, err
		}
		root, err := parseLayer(configPath, data)
		if err != nil {
			return nil, report, err
		}
		if root != nil {
			merged = root
			leafPaths(root, "", func(path string, _ *yaml.Node) { setBy[path] = "server.yml" })
		}
	}

	if configPath == "" {
		return merged, report, nil
	}

	files, warnings := listDropinFiles(DropinDir(configPath))
	report.Warnings = append(report.Warnings, warnings...)

	for _, path := range files {
		label := DropinDirName + "/" + filepath.Base(path)
		data, err := os.ReadFile(path)
		if err != nil {
			if strict {
				return nil, report, fmt.Errorf("%s: %w", label, err)
			}
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: %v", label, err))
			continue
		}
		root, err := parseLayer(label, data)
		if err == nil && root != nil {
			err = rejectNulls(root, "", label)
		}
		if err != nil {
			if strict {
				return nil, report, err
			}
			report.Warnings = append(report.Warnings, err.Error()+" (file skipped)")
			continue
		}

		report.Files = append(report.Files, label)
		if root == nil {
			continue
		}
		if err := mergeLayer(merged, root, "", label); err != nil {
			if strict {
				return nil, report, err
			}
			report.Warnings = append(report.Warnings, err.Error()+" (file skipped)")
			continue
		}
		leafPaths(root, "", func(p string, _ *yaml.Node) {
			if _, seen := setBy[p]; seen {
				report.Overrides = append(report.Overrides, DropinOverride{Key: p, File: label})
			}
			setBy[p] = label
			report.Sources[p] = label
		})
	}

	return merged, report, nil
}

// applyLayers decodes the merged mapping over cfg, so values missing from every
// file keep their built-in defaults.
func applyLayers(cfg *AppConfig, merged *yaml.Node) error {
	if merged == nil || len(merged.Content) == 0 {
		return nil
	}
	return merged.Decode(cfg)
}

// dropinOwnedKeys returns the dotted leaf keys that the current drop-in files
// set, mapped to the file that owns each, read fresh from disk.
func dropinOwnedKeys(configPath string) map[string]string {
	owned := map[string]string{}
	if configPath == "" {
		return owned
	}
	files, _ := listDropinFiles(DropinDir(configPath))
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		root, err := parseLayer(path, data)
		if err != nil || root == nil {
			continue
		}
		label := DropinDirName + "/" + filepath.Base(path)
		leafPaths(root, "", func(p string, _ *yaml.Node) { owned[p] = label })
	}
	return owned
}

// stripOwnedKeys removes every drop-in-owned leaf from the marshalled
// server.yml document so the panel and generated values never copy a drop-in
// setting into server.yml. Mappings left empty are pruned as well.
func stripOwnedKeys(data []byte, owned map[string]string) ([]byte, error) {
	if len(owned) == 0 {
		return data, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	root, err := rootMapping(&doc)
	if err != nil || root == nil {
		return data, err
	}
	pruneOwned(root, "", owned)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// pruneOwned deletes owned leaves under node and reports whether node is now empty.
func pruneOwned(node *yaml.Node, prefix string, owned map[string]string) bool {
	kept := node.Content[:0]
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		path := key.Value
		if prefix != "" {
			path = prefix + "." + key.Value
		}
		if _, hit := owned[path]; hit && value.Kind != yaml.MappingNode {
			continue
		}
		if value.Kind == yaml.MappingNode && pruneOwned(value, path, owned) {
			continue
		}
		kept = append(kept, key, value)
	}
	node.Content = kept
	return len(node.Content) == 0
}
