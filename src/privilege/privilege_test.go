//go:build !windows

package privilege

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIsReservedID(t *testing.T) {
	for _, id := range []int{65534, 999, 980, 110, 101, 179, 170} {
		if !IsReservedID(id) {
			t.Errorf("ID %d must be reserved", id)
		}
	}
	for _, id := range []int{899, 500, 200, 111, 169, 180} {
		if IsReservedID(id) {
			t.Errorf("ID %d must not be reserved", id)
		}
	}
}

func TestFindAvailableSystemIDStartsAtTheTop(t *testing.T) {
	none := func(int) bool { return false }
	id, err := FindAvailableSystemID(899, 200, none, none)
	if err != nil || id != 899 {
		t.Errorf("got %d, %v; want 899", id, err)
	}
}

func TestFindAvailableSystemIDSkipsTakenAndReserved(t *testing.T) {
	uidTaken := func(id int) bool { return id == 899 }
	gidTaken := func(id int) bool { return id == 898 }
	id, err := FindAvailableSystemID(899, 200, uidTaken, gidTaken)
	if err != nil || id != 897 {
		t.Errorf("got %d, %v; want 897 (899 UID taken, 898 GID taken)", id, err)
	}

	id, err = FindAvailableSystemID(110, 100, func(int) bool { return false }, func(int) bool { return false })
	if err != nil || id != 100 {
		t.Errorf("got %d, %v; want 100 (101-110 are reserved)", id, err)
	}
}

func TestFindAvailableSystemIDExhausted(t *testing.T) {
	all := func(int) bool { return true }
	if id, err := FindAvailableSystemID(899, 200, all, all); err == nil {
		t.Errorf("expected an error when every ID is taken, got %d", id)
	}
}

func TestAccountCommandsLinuxShadowTools(t *testing.T) {
	has := func(name string) bool { return name == "groupadd" || name == "useradd" }
	got := accountCommands("linux", "wthr", 899, "/etc/webappsgo/wthr", "/sbin/nologin", has)
	want := [][]string{
		{"groupadd", "--system", "--gid", "899", "wthr"},
		{"useradd", "--system", "--uid", "899", "--gid", "899", "--home-dir", "/etc/webappsgo/wthr", "--shell", "/sbin/nologin", "--comment", "wthr service account", "wthr"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestAccountCommandsBusyBox(t *testing.T) {
	got := accountCommands("linux", "wthr", 898, "/var/lib/webappsgo/wthr", "/sbin/nologin", func(string) bool { return false })
	if got[0][0] != "addgroup" || got[1][0] != "adduser" {
		t.Fatalf("BusyBox hosts must use addgroup and adduser, got %v", got)
	}
	if got[0][3] != "898" || got[1][5] != "898" {
		t.Errorf("UID and GID must be the same value, got %v", got)
	}
}

func TestAccountCommandsFreeBSDAndDarwin(t *testing.T) {
	bsd := accountCommands("freebsd", "wthr", 897, "/var/lib/webappsgo/wthr", "/usr/sbin/nologin", func(name string) bool { return name == "pw" })
	if bsd[0][0] != "pw" || bsd[0][1] != "groupadd" || bsd[1][1] != "useradd" {
		t.Errorf("FreeBSD must use pw, got %v", bsd)
	}

	mac := accountCommands("darwin", "wthr", 399, "/usr/local/var/webappsgo/wthr", "/usr/bin/false", func(string) bool { return true })
	joined := ""
	for _, command := range mac {
		joined += strings.Join(command, " ") + "\n"
	}
	for _, want := range []string{"PrimaryGroupID 399", "UniqueID 399", "UserShell /usr/bin/false", "IsHidden 1", "NFSHomeDirectory /usr/local/var/webappsgo/wthr"} {
		if !strings.Contains(joined, want) {
			t.Errorf("macOS commands missing %q:\n%s", want, joined)
		}
	}
}

func TestLookupAccountUnknown(t *testing.T) {
	if _, err := LookupAccount("wthr-no-such-account-xyz"); err == nil {
		t.Error("an unknown account must return an error")
	}
}

func TestLookupAccountRoot(t *testing.T) {
	account, err := LookupAccount("root")
	if err != nil {
		t.Skipf("no root account to look up: %v", err)
	}
	if account.UID != 0 || account.GID != 0 || account.Name != "root" {
		t.Errorf("root account = %+v", account)
	}
}

func TestChownTreeChangesEverythingBelow(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink("/etc/hostname", link); err != nil {
		t.Fatal(err)
	}

	own := &Account{Name: "self", UID: os.Getuid(), GID: os.Getgid()}
	if err := ChownTree(root, own); err != nil {
		t.Fatalf("ChownTree: %v", err)
	}
}

func TestChownTreeMissingRoot(t *testing.T) {
	own := &Account{Name: "self", UID: os.Getuid(), GID: os.Getgid()}
	if err := ChownTree(filepath.Join(t.TempDir(), "missing"), own); err == nil {
		t.Error("a missing root must return an error")
	}
}

func TestBindPrivilegedPortsIgnoresUnprivilegedPorts(t *testing.T) {
	if err := BindPrivilegedPorts("127.0.0.1", []int{0, 1024, 8080, 64000, -1}); err != nil {
		t.Fatalf("unprivileged ports must be ignored: %v", err)
	}
	if TakeListener(8080) != nil || TakeListener(1024) != nil {
		t.Error("unprivileged ports must not be pre-bound")
	}
}

func TestTakeListenerUnknownPort(t *testing.T) {
	if TakeListener(1) != nil {
		t.Error("a port that was never pre-bound must return nil")
	}
}

func TestBindPrivilegedPortsHandsOverOnce(t *testing.T) {
	if !IsRoot() {
		t.Skip("binding a port below 1024 needs root")
	}

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = probe.Close()

	const port = 1019
	if err := BindPrivilegedPorts("127.0.0.1", []int{port}); err != nil {
		t.Skipf("port %d is not free here: %v", port, err)
	}
	first := TakeListener(port)
	if first == nil {
		t.Fatal("the pre-bound listener must be handed over")
	}
	defer first.Close()
	if TakeListener(port) != nil {
		t.Error("the listener must be handed over only once")
	}
}

func TestBindPrivilegedPortsFailureClosesEarlierSockets(t *testing.T) {
	if !IsRoot() {
		t.Skip("binding a port below 1024 needs root")
	}

	blocker, err := net.Listen("tcp", "127.0.0.1:1018")
	if err != nil {
		t.Skipf("port 1018 is not free here: %v", err)
	}
	defer blocker.Close()

	if err := BindPrivilegedPorts("127.0.0.1", []int{1017, 1018}); err == nil {
		t.Fatal("binding an occupied port must fail")
	}
	if TakeListener(1017) != nil {
		t.Error("sockets bound before the failure must be closed and forgotten")
	}
}

func TestStartedAsRootIsDecidedAtStart(t *testing.T) {
	if StartedAsRoot() != (os.Geteuid() == 0) {
		t.Errorf("StartedAsRoot() = %v, but the test process euid is %d", StartedAsRoot(), os.Geteuid())
	}
}

func TestAccountRemoveCommands(t *testing.T) {
	shadow := accountRemoveCommands("linux", "wthr", func(name string) bool { return name == "userdel" || name == "groupdel" })
	if shadow[0][0] != "userdel" || shadow[1][0] != "groupdel" {
		t.Errorf("shadow tools must remove the user before the group, got %v", shadow)
	}

	busybox := accountRemoveCommands("linux", "wthr", func(string) bool { return false })
	if busybox[0][0] != "deluser" || busybox[1][0] != "delgroup" {
		t.Errorf("BusyBox hosts must use deluser and delgroup, got %v", busybox)
	}

	bsd := accountRemoveCommands("freebsd", "wthr", func(name string) bool { return name == "pw" })
	if bsd[0][1] != "userdel" || bsd[1][1] != "groupdel" {
		t.Errorf("FreeBSD must use pw userdel and groupdel, got %v", bsd)
	}

	mac := accountRemoveCommands("darwin", "wthr", func(string) bool { return true })
	if mac[0][3] != "/Users/wthr" || mac[1][3] != "/Groups/wthr" {
		t.Errorf("macOS must delete the dscl user and group records, got %v", mac)
	}
}

func TestRemoveAccountMissingIsNoError(t *testing.T) {
	if err := RemoveAccount("wthr-no-such-account-xyz"); err != nil {
		t.Errorf("removing an account that does not exist must succeed, got %v", err)
	}
}
