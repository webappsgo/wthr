package cli

import "github.com/webappsgo/wthr/src/config"

// GenerateServerYML writes node-local typed defaults when server.yml does not
// exist. The port argument is the value already selected for this startup.
func GenerateServerYML(configDir, port string) error {
	return config.GenerateDefaultServerYML(configDir, port)
}
