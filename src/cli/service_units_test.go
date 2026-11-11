package cli

import (
	"strings"
	"testing"
)

func TestServiceTemplatesHaveNoUnresolvedPlaceholders(t *testing.T) {
	templates := map[string]string{
		"systemd": systemdUnitTemplate,
		"openrc":  openRCScriptTemplate,
		"sysv":    sysVScriptTemplate,
		"runit":   runitRunTemplate,
		"runit-l": runitLogRunTemplate,
	}
	for name, text := range templates {
		out := serviceTemplate(text)
		for _, marker := range []string{"{internal_name}", "{internal_org}", "{project_name}", "{app_name}", "{binary}", "{docs}"} {
			if strings.Contains(out, marker) {
				t.Errorf("%s: placeholder %s left unresolved", name, marker)
			}
		}
	}
}

// AI.md PART 25: the service starts as root and the binary drops privileges
// itself, so no template may switch user.
func TestServiceTemplatesDoNotSwitchUser(t *testing.T) {
	for name, text := range map[string]string{
		"systemd": systemdUnitTemplate,
		"openrc":  openRCScriptTemplate,
		"sysv":    sysVScriptTemplate,
		"runit":   runitRunTemplate,
	} {
		for _, forbidden := range []string{"command_user", "--chuid", "chpst -u", "\nUser=", "\nGroup="} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s template contains %q", name, forbidden)
			}
		}
	}
}

func TestSystemdUnitHardening(t *testing.T) {
	out := serviceTemplate(systemdUnitTemplate)
	for _, want := range []string{
		"ProtectSystem=strict", "ProtectHome=yes", "PrivateTmp=yes",
		"ReadWritePaths=/etc/webappsgo/wthr", "ReadWritePaths=/var/lib/webappsgo/wthr",
		"ReadWritePaths=/var/cache/webappsgo/wthr", "ReadWritePaths=/var/log/webappsgo/wthr",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("systemd unit missing %q", want)
		}
	}
}
