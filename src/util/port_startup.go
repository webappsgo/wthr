package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// StartupPorts are the listen ports decided before the privilege drop (AI.md PART 8
// step 8e), so privileged ports can be bound while the process is still root.
type StartupPorts struct {
	// HTTP is the first (or only) port; it serves plain HTTP unless it is 443
	HTTP int
	// HTTPS is the second port of a dual-port setting, or 0
	HTTPS int
	// Spec is the port setting as written to server.yml: "8080" or "80,443"
	Spec string
	// FromConfig is true when server.yml already named the port
	FromConfig bool
}

// ParsePortSpec reads a port setting: "8080" for one port, or "80,443" for HTTP then HTTPS.
func ParsePortSpec(spec string) (httpPort, httpsPort int, err error) {
	parts := strings.Split(strings.TrimSpace(spec), ",")
	if len(parts) > 2 {
		return 0, 0, fmt.Errorf("invalid port setting %q: use one port or HTTP,HTTPS", spec)
	}

	httpPort, err = parsePortNumber(parts[0])
	if err != nil {
		return 0, 0, err
	}
	if len(parts) == 2 {
		httpsPort, err = parsePortNumber(parts[1])
		if err != nil {
			return 0, 0, err
		}
		if httpsPort == httpPort {
			return 0, 0, fmt.Errorf("invalid port setting %q: the HTTP and HTTPS ports must differ", spec)
		}
	}
	return httpPort, httpsPort, nil
}

// parsePortNumber reads one TCP port number.
func parsePortNumber(text string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port number %q", strings.TrimSpace(text))
	}
	return port, nil
}

// peekConfigPort reads only server.port from {config_dir}/server.yml. Nothing else in
// the file is read or validated here; the full load happens later (AI.md PART 8 step 8e).
// It returns "" when the file or the key is missing, or the file cannot be parsed.
func peekConfigPort(configDir string) string {
	data, err := os.ReadFile(filepath.Join(configDir, "server.yml"))
	if err != nil {
		return ""
	}

	var peek struct {
		Server struct {
			Port interface{} `yaml:"port"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(data, &peek); err != nil {
		return ""
	}

	switch value := peek.Server.Port.(type) {
	case int:
		return strconv.Itoa(value)
	case string:
		return strings.TrimSpace(value)
	}
	return ""
}

// ResolveStartupPorts decides the listen ports in AI.md PART 8 step 8e order: the PORT
// environment variable (the --port flag sets it), then server.port from server.yml, then
// a random unused port in 64000-64999 for a first run.
func ResolveStartupPorts(configDir string) (StartupPorts, error) {
	if spec := strings.TrimSpace(os.Getenv("PORT")); spec != "" {
		httpPort, httpsPort, err := ParsePortSpec(spec)
		if err != nil {
			return StartupPorts{}, err
		}
		return StartupPorts{HTTP: httpPort, HTTPS: httpsPort, Spec: spec}, nil
	}

	if spec := peekConfigPort(configDir); spec != "" {
		httpPort, httpsPort, err := ParsePortSpec(spec)
		if err != nil {
			return StartupPorts{}, err
		}
		return StartupPorts{HTTP: httpPort, HTTPS: httpsPort, Spec: spec, FromConfig: true}, nil
	}

	random, err := GetRandomAvailablePort()
	if err != nil {
		return StartupPorts{}, err
	}
	return StartupPorts{HTTP: random, Spec: strconv.Itoa(random)}, nil
}
