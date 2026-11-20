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
		code := withoutCommentLines(text)
		for _, forbidden := range []string{"command_user", "--chuid", "chpst -u", "\nUser=", "\nGroup="} {
			if strings.Contains(code, forbidden) {
				t.Errorf("%s template contains %q", name, forbidden)
			}
		}
	}
}

// withoutCommentLines drops shell and unit comment lines so a comment that explains
// why an option is absent is not mistaken for the option itself.
func withoutCommentLines(text string) string {
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func TestSystemdUnitHardening(t *testing.T) {
	out := serviceTemplate(systemdUnitTemplate)
	for _, want := range []string{
		"ProtectSystem=strict", "ProtectHome=yes", "PrivateTmp=yes",
		"ReadWritePaths=-/etc/webappsgo/wthr", "ReadWritePaths=-/var/lib/webappsgo/wthr",
		"ReadWritePaths=-/var/cache/webappsgo/wthr", "ReadWritePaths=-/var/log/webappsgo/wthr",
		"ReadWritePaths=-/var/run/webappsgo/wthr", "ReadWritePaths=-/mnt/Backups/webappsgo/wthr",
		"ExecStartPre=+/usr/bin/install -d -m 0755 /etc/webappsgo/wthr",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("systemd unit missing %q", want)
		}
	}
}

// /var/run is cleared at boot and checkpath creates only the last path component, so the
// organization directory is created before the per-service one.
func TestOpenRCScriptCreatesRunParentFirst(t *testing.T) {
	out := serviceTemplate(openRCScriptTemplate)
	parent := strings.Index(out, "checkpath -d -m 0755 /var/run/webappsgo\n")
	child := strings.Index(out, "-o wthr:wthr /var/run/webappsgo/wthr")
	if parent < 0 || child < 0 || parent > child {
		t.Errorf("start_pre must create /var/run/webappsgo before /var/run/webappsgo/wthr:\n%s", out)
	}
}
