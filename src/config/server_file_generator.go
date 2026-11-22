package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var serverFileComments = map[string]string{
	"server":                                  "Node-local server settings. Shared application settings live in yaml.d/.",
	"server.port":                             "HTTP port, or a comma-separated HTTP and HTTPS port pair.",
	"server.address":                          "Network interface address used by the listener.",
	"server.fqdn":                             "Public fully qualified domain name for this node.",
	"server.mode":                             "Runtime mode: production or development.",
	"server.api_version":                      "Default API version prefix.",
	"server.baseurl":                          "URL prefix when this server is mounted below the origin root.",
	"server.pidfile":                          "Create a process ID file for this server.",
	"server.daemonize":                        "Detach from the terminal when started directly.",
	"server.database":                         "Database connection used by this node.",
	"server.database.driver":                  "Database driver selected for this node.",
	"server.database.host":                    "Remote database host, when the selected driver uses one.",
	"server.database.port":                    "Remote database port, when the selected driver uses one.",
	"server.database.name":                    "Database name or file name for this node.",
	"server.database.username":                "Remote database username, when required.",
	"server.database.password":                "Remote database credential; stored separately in secrets.yml.",
	"server.database.sslmode":                 "TLS mode for PostgreSQL connections.",
	"server.cache":                            "Cache connection used by this node.",
	"server.cache.type":                       "Cache backend: memory, valkey, or redis.",
	"server.cache.url":                        "Cache URL; when set it takes precedence over discrete connection fields.",
	"server.cache.host":                       "Cache server host for discrete connections.",
	"server.cache.port":                       "Cache server port for discrete connections.",
	"server.cache.username":                   "Cache username for discrete connections.",
	"server.cache.password":                   "Cache credential; stored separately in secrets.yml.",
	"server.cache.db":                         "Cache database number.",
	"server.cache.tls":                        "Use TLS for cache connections.",
	"server.cache.tls_skip_verify":            "Skip TLS certificate verification for cache connections.",
	"server.cache.pool_size":                  "Maximum cache connection pool size.",
	"server.cache.min_idle":                   "Minimum idle cache connections to keep open.",
	"server.cache.timeout":                    "Cache connection timeout.",
	"server.cache.prefix":                     "Unique key prefix for this application.",
	"server.cache.ttl":                        "Default cache entry lifetime.",
	"server.cache.cluster":                    "Enable native Redis or Valkey cluster mode.",
	"server.cache.cluster_nodes":              "Nodes used by native Redis or Valkey cluster mode.",
	"server.ssl":                              "TLS listener and certificate settings for this node.",
	"server.ssl.enabled":                      "Enable HTTPS on the configured listener.",
	"server.ssl.cert":                         "Path to the TLS certificate file.",
	"server.ssl.key":                          "Path to the TLS private key file.",
	"server.ssl.min_version":                  "Minimum TLS protocol version.",
	"server.ssl.letsencrypt":                  "Automatic certificate issuance settings for this node.",
	"server.ssl.letsencrypt.enabled":          "Enable automatic certificate issuance.",
	"server.ssl.letsencrypt.email":            "Contact address for certificate registration.",
	"server.ssl.letsencrypt.challenge":        "ACME challenge type used to prove domain control.",
	"server.ssl.letsencrypt.staging":          "Use the ACME staging environment for testing.",
	"server.ssl.letsencrypt.dns_provider":     "Provider used for DNS-01 challenges.",
	"server.ssl.letsencrypt.dns_credentials":  "DNS provider credentials are stored separately in secrets.yml.",
	"server.trusted_proxies":                  "Proxy settings used to accept forwarded request headers.",
	"server.trusted_proxies.additional":       "Additional trusted proxy IP addresses or CIDR ranges.",
	"server.trusted_proxies.client_ip_header": "Forwarded client IP header selection, or auto detection.",
	"server.cluster":                          "Cluster membership settings for this node.",
	"server.cluster.enabled":                  "Join this node to the configured shared database cluster.",
	"server.cluster.id":                       "Cluster identity assigned during authenticated node enrollment.",
}

// GenerateDefaultServerYML creates a node-local server.yml from typed defaults.
// Shared settings and credentials are written to their canonical files by the
// config loader and are not duplicated here.
func GenerateDefaultServerYML(configDir, port string) error {
	if strings.TrimSpace(configDir) == "" {
		return fmt.Errorf("configuration directory is required")
	}
	if err := secureConfigDirectory(configDir); err != nil {
		return fmt.Errorf("secure config directory: %w", err)
	}
	configPath := filepath.Join(configDir, "server.yml")
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check server.yml: %w", err)
	}

	defaults := DefaultAppConfig()
	if strings.TrimSpace(port) != "" {
		if number, err := strconv.Atoi(port); err == nil {
			defaults.Server.Port = number
		} else {
			defaults.Server.Port = port
		}
	}
	data, err := yaml.Marshal(defaults)
	if err != nil {
		return fmt.Errorf("encode typed configuration defaults: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse typed configuration defaults: %w", err)
	}
	fullRoot, err := rootMapping(&document)
	if err != nil {
		return err
	}
	serverRoot := newMappingNode()
	for _, key := range nodeConfigPrefixes {
		value := nodeAtPath(fullRoot, strings.Split(key, "."))
		if value == nil {
			continue
		}
		selected := cloneYAMLNode(value)
		if selected.Kind == yaml.ScalarNode && (key == "server.address" || key == "server.port" && selected.Tag == "!!str" && strings.Contains(selected.Value, ",")) {
			selected.Style = yaml.DoubleQuotedStyle
		}
		if err := setNodePath(serverRoot, strings.Split(key, "."), selected); err != nil {
			return fmt.Errorf("select node setting %s: %w", key, err)
		}
	}
	removeGeneratedSecrets(serverRoot, "")
	annotateServerDefaults(serverRoot, "")
	encoded, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{serverRoot}})
	if err != nil {
		return fmt.Errorf("encode server.yml: %w", err)
	}
	encoded = append([]byte(canonicalServerConfigHeader), encoded...)
	if err := writeConfigAtomic(configPath, encoded, 0o600); err != nil {
		return fmt.Errorf("write server.yml: %w", err)
	}
	return nil
}

func removeGeneratedSecrets(node *yaml.Node, prefix string) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); {
			keyNode, valueNode := node.Content[i], node.Content[i+1]
			key := keyNode.Value
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			if CanonicalConfigOwner(path) == "secrets.yml" {
				node.Content = append(node.Content[:i], node.Content[i+2:]...)
				continue
			}
			removeGeneratedSecrets(valueNode, path)
			i += 2
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			removeGeneratedSecrets(child, fmt.Sprintf("%s.%d", prefix, i))
		}
	}
}

func annotateServerDefaults(node *yaml.Node, prefix string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		path := keyNode.Value
		if prefix != "" {
			path = prefix + "." + path
		}
		if comment := serverFileComments[path]; comment != "" {
			keyNode.HeadComment = comment
		}
		annotateServerDefaults(node.Content[i+1], path)
	}
}
