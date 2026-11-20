//go:build !windows

package privilege

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
)

// accountComment is the gecos field of the service account (AI.md PART 24).
const accountComment = " service account"

// isUIDTaken reports whether a UID is already assigned on this system.
func isUIDTaken(id int) bool {
	_, err := user.LookupId(strconv.Itoa(id))
	return err == nil
}

// isGIDTaken reports whether a GID is already assigned on this system.
func isGIDTaken(id int) bool {
	_, err := user.LookupGroupId(strconv.Itoa(id))
	return err == nil
}

// LookupAccount returns the service account when it already exists.
func LookupAccount(name string) (*Account, error) {
	found, err := user.Lookup(name)
	if err != nil {
		return nil, err
	}
	uid, err := strconv.Atoi(found.Uid)
	if err != nil {
		return nil, fmt.Errorf("service account %s has a non-numeric UID %q", name, found.Uid)
	}
	gid, err := strconv.Atoi(found.Gid)
	if err != nil {
		return nil, fmt.Errorf("service account %s has a non-numeric GID %q", name, found.Gid)
	}
	return &Account{Name: name, UID: uid, GID: gid, Home: found.HomeDir}, nil
}

// loginShellPath returns the no-login shell of this system.
func loginShellPath() string {
	for _, candidate := range []string{"/sbin/nologin", "/usr/sbin/nologin"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "/bin/false"
}

// accountCommands lists the commands that create the service group and user with the
// same numeric ID on the given OS (AI.md PART 24 "Platform-Specific Commands").
// hasCommand reports whether an executable is on PATH; shell is the no-login shell.
func accountCommands(goos, name string, id int, home, shell string, hasCommand func(string) bool) [][]string {
	idText := strconv.Itoa(id)
	comment := name + accountComment

	switch {
	case goos == "darwin":
		return [][]string{
			{"dscl", ".", "-create", "/Groups/" + name},
			{"dscl", ".", "-create", "/Groups/" + name, "PrimaryGroupID", idText},
			{"dscl", ".", "-create", "/Groups/" + name, "Password", "*"},
			{"dscl", ".", "-create", "/Users/" + name},
			{"dscl", ".", "-create", "/Users/" + name, "UniqueID", idText},
			{"dscl", ".", "-create", "/Users/" + name, "PrimaryGroupID", idText},
			{"dscl", ".", "-create", "/Users/" + name, "UserShell", "/usr/bin/false"},
			{"dscl", ".", "-create", "/Users/" + name, "RealName", comment},
			{"dscl", ".", "-create", "/Users/" + name, "NFSHomeDirectory", home},
			{"dscl", ".", "-create", "/Users/" + name, "Password", "*"},
			{"dscl", ".", "-create", "/Users/" + name, "IsHidden", "1"},
		}
	case hasCommand("pw"):
		return [][]string{
			{"pw", "groupadd", "-n", name, "-g", idText},
			{"pw", "useradd", "-n", name, "-u", idText, "-g", idText, "-d", home, "-s", shell, "-c", comment},
		}
	case hasCommand("groupadd") && hasCommand("useradd"):
		return [][]string{
			{"groupadd", "--system", "--gid", idText, name},
			{"useradd", "--system", "--uid", idText, "--gid", idText, "--home-dir", home, "--shell", shell, "--comment", comment, name},
		}
	default:
		// BusyBox (Alpine runtime image) ships addgroup and adduser instead of the shadow tools
		return [][]string{
			{"addgroup", "-S", "-g", idText, name},
			{"adduser", "-S", "-D", "-H", "-u", idText, "-G", name, "-h", home, "-s", shell, "-g", comment, name},
		}
	}
}

// hasCommandOnPath reports whether an executable is found on PATH.
func hasCommandOnPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// EnsureAccount returns the service account, creating the group and user first when
// they do not exist. The home directory is created before the user, as AI.md PART 24
// "Home Directory Selection" requires. Only root can create an account.
func EnsureAccount(name, home string) (*Account, error) {
	if existing, err := LookupAccount(name); err == nil {
		return existing, nil
	}

	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, fmt.Errorf("creating home directory %s: %w", home, err)
	}

	high, low := systemIDHigh, systemIDLow
	if runtime.GOOS == "darwin" {
		high, low = macOSIDHigh, macOSIDLow
	}
	id, err := FindAvailableSystemID(high, low, isUIDTaken, isGIDTaken)
	if err != nil {
		return nil, err
	}

	for _, command := range accountCommands(runtime.GOOS, name, id, home, loginShellPath(), hasCommandOnPath) {
		if output, err := exec.Command(command[0], command[1:]...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("creating service account with %s: %w: %s", command[0], err, output)
		}
	}

	return LookupAccount(name)
}

// accountRemoveCommands lists the commands that delete the service user and group on the
// given OS, user first so the group is empty when it is removed. hasCommand reports whether
// an executable is on PATH.
func accountRemoveCommands(goos, name string, hasCommand func(string) bool) [][]string {
	switch {
	case goos == "darwin":
		return [][]string{
			{"dscl", ".", "-delete", "/Users/" + name},
			{"dscl", ".", "-delete", "/Groups/" + name},
		}
	case hasCommand("pw"):
		return [][]string{
			{"pw", "userdel", "-n", name},
			{"pw", "groupdel", "-n", name},
		}
	case hasCommand("userdel") && hasCommand("groupdel"):
		return [][]string{
			{"userdel", name},
			{"groupdel", name},
		}
	default:
		// BusyBox (Alpine) deluser removes the user; delgroup removes the now-empty group
		return [][]string{
			{"deluser", name},
			{"delgroup", name},
		}
	}
}

// RemoveAccount deletes the service user and group when they exist. A missing account is
// not an error, so an uninstall can be repeated; a command that fails only matters when the
// user or group still exists afterwards (BusyBox deluser can remove the group on its own).
// Only root can remove an account.
func RemoveAccount(name string) error {
	if !accountExists(name) {
		return nil
	}

	var failures []string
	for _, command := range accountRemoveCommands(runtime.GOOS, name, hasCommandOnPath) {
		if output, err := exec.Command(command[0], command[1:]...).CombinedOutput(); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v: %s", command[0], err, strings.TrimSpace(string(output))))
		}
	}

	if accountExists(name) {
		return fmt.Errorf("removing service account %s: %s", name, strings.Join(failures, "; "))
	}

	return nil
}

// accountExists reports whether the user or the group of this name exists.
func accountExists(name string) bool {
	if _, err := user.Lookup(name); err == nil {
		return true
	}
	_, err := user.LookupGroup(name)

	return err == nil
}
