package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/mode"
	"github.com/webappsgo/wthr/src/server/lifecycle"
	"github.com/webappsgo/wthr/src/util"
)

func TestRunServerWithHooksStartsAndShutsDown(t *testing.T) {
	portProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := portProbe.Addr().(*net.TCPAddr).Port
	if err := portProbe.Close(); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	dataDir := filepath.Join(root, "data")
	cacheDir := filepath.Join(root, "cache")
	logDir := filepath.Join(root, "log")
	databaseDir := filepath.Join(dataDir, "db")
	backupDir := filepath.Join(dataDir, "backups")
	for _, dir := range []string{configDir, filepath.Join(dataDir, "db"), cacheDir, logDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	settings := fmt.Sprintf("server:\n  address: 127.0.0.1\n  port: %d\n  daemonize: false\n  pidfile: false\n  ssl:\n    enabled: false\n  tor:\n    enabled: false\n  i2p:\n    enabled: false\n  cache:\n    type: memory\n  database:\n    driver: sqlite\n", port)
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	configAreaDir := filepath.Join(configDir, "yaml.d")
	if err := os.MkdirAll(configAreaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configAreaDir, "notifications.yml"), []byte("server:\n  notifications:\n    email:\n      smtp:\n        host: 127.0.0.1\n        port: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(databaseDir, "server.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	geoIPDir := filepath.Join(dataDir, "security", "geoip")
	if err := os.MkdirAll(geoIPDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dbip-city-ipv4.mmdb", "dbip-city-ipv6.mmdb", "iptoasn-country.mmdb", "iptoasn-asn.mmdb"} {
		if err := os.WriteFile(filepath.Join(geoIPDir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stdout, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	previousStdout := os.Stdout
	os.Stdout = stdout
	t.Cleanup(func() {
		os.Stdout = previousStdout
		_ = stdout.Close()
	})

	for key, value := range map[string]string{
		"CONFIG_DIR": configDir, "DATA_DIR": dataDir, "CACHE_DIR": cacheDir,
		"LOG_DIR": logDir, "DATABASE_DIR": databaseDir, "BACKUP_DIR": backupDir,
		"MODE": "production", "DEBUG": "false", "PORT": "", "LISTEN": "",
		"SERVER_ADDRESS": "", "CLI_COMMAND_HANDLED": "", "CLI_HEALTHCHECK_FLAG": "",
		"CLI_STATUS_FLAG": "", "CLUSTER_JOIN_URL": "", "CLUSTER_JOIN_TOKEN": "",
		"SMTP_HOST": "", "SMTP_PORT": "", "SMTP_USERNAME": "", "SMTP_PASSWORD": "",
		"SMTP_FROM_EMAIL": "", "SMTP_FROM_NAME": "", "SMTP_TLS": "",
		"DAEMON": "", "_DAEMON_CHILD": "", "REVERSE_PROXY": "", "NO_COLOR": "1",
	} {
		t.Setenv(key, value)
	}
	originalArgs := os.Args
	os.Args = []string{originalArgs[0]}
	t.Cleanup(func() { os.Args = originalArgs })

	previousConfig := config.GetGlobalConfig()
	previousDB := database.GetGlobalDualDB()
	previousMode := mode.GetCurrentAppMode()
	previousDebug := mode.IsDebugEnabled()
	previousRestart := restartPending.Swap(false)
	stop := make(chan struct{})
	close(stop)
	lifecycle.Reset()
	config.SetGlobalConfig(nil)
	database.SetGlobalDualDB(nil)
	config.SetSecretGeneratedAuditHook(nil)
	t.Cleanup(func() {
		restartPending.Store(previousRestart)
		lifecycle.Reset()
		config.SetGlobalConfig(previousConfig)
		database.SetGlobalDualDB(previousDB)
		mode.SetAppMode(previousMode.String())
		mode.SetDebugEnabled(previousDebug)
		config.SetSecretGeneratedAuditHook(nil)
	})

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("runServerWithHooks() exited during successful startup: %v", recovered)
		}
	}()
	runServerWithHooks(serverRunHooks{
		exitProcess:  func(code int) { panic(fmt.Errorf("unexpected exit %d", code)) },
		runRootPhase: func(*util.DirectoryPaths, util.StartupPorts, string) {},
		serviceStop:  stop,
	})
	if !lifecycle.IsShuttingDown() {
		t.Fatal("graceful stop did not enter the shutdown state")
	}
}

func TestRunServerWiresProductionHooks(t *testing.T) {
	previousRunner := runServerWithHooksRunner
	t.Cleanup(func() { runServerWithHooksRunner = previousRunner })
	called := false
	var got serverRunHooks
	runServerWithHooksRunner = func(hooks serverRunHooks) {
		called = true
		got = hooks
	}

	runServer()

	if !called {
		t.Fatal("runServer did not invoke the server runner")
	}
	if got.exitProcess == nil || got.runRootPhase == nil || got.serviceStop != serviceStopRequested || !got.installSignals {
		t.Fatalf("runServer hooks = %#v, want production exit, root phase, stop channel, and signal handling", got)
	}
}

func TestRunServerWithHooksRequestsSupervisorRestart(t *testing.T) {
	t.Setenv("INVOCATION_ID", "startup-test")
	t.Setenv("CLI_HEALTHCHECK_FLAG", "")
	previousArgs := os.Args
	os.Args = []string{previousArgs[0], "--shell", "init"}
	t.Cleanup(func() { os.Args = previousArgs })
	previousRestart := restartPending.Swap(true)
	t.Cleanup(func() { restartPending.Store(previousRestart) })

	defer func() {
		recovered := recover()
		exit, ok := recovered.(serverRunExitPanic)
		if !ok {
			t.Fatalf("runServerWithHooks() panic = %#v, want supervisor restart exit", recovered)
		}
		if exit.code != exitRestart {
			t.Fatalf("supervisor restart exit code = %d, want %d", exit.code, exitRestart)
		}
	}()
	runServerWithHooks(serverRunHooks{
		exitProcess: func(code int) { panic(serverRunExitPanic{code: code}) },
	})
	t.Fatal("runServerWithHooks() returned without requesting restart")
}
