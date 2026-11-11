//go:build windows

package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

type windowsUpdatePayload struct {
	Target  string `json:"target"`
	NewPath string `json:"new_path"`
	TempDir string `json:"temp_dir"`
	PID     int    `json:"pid"`
	Args    string `json:"args"`
}

func replaceClientBinary(currentPath, newPath string) error {
	payload, err := json.Marshal(windowsUpdatePayload{
		Target:  currentPath,
		NewPath: newPath,
		TempDir: filepath.Dir(newPath),
		PID:     os.Getpid(),
		Args:    joinWindowsArgs(os.Args[1:]),
	})
	if err != nil {
		return err
	}
	payload64 := base64.StdEncoding.EncodeToString(payload)
	script := "$ErrorActionPreference='Stop'\n" +
		"$p=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + payload64 + "')) | ConvertFrom-Json\n" +
		"try {\n" +
		"  Wait-Process -Id $p.PID\n" +
		"  $staged=$p.Target+'.'+[guid]::NewGuid().ToString('N')+'.new'\n" +
		"  Copy-Item -LiteralPath $p.NewPath -Destination $staged -Force\n" +
		"  Move-Item -Force -LiteralPath $staged -Destination $p.Target\n" +
		"  $psi=New-Object System.Diagnostics.ProcessStartInfo\n" +
		"  $psi.FileName=$p.Target\n" +
		"  $psi.Arguments=$p.Args\n" +
		"  $psi.UseShellExecute=$false\n" +
		"  $psi.EnvironmentVariables['WTHR_CLI_UPDATE_REEXEC']='1'\n" +
		"  if (-not [Diagnostics.Process]::Start($psi)) { throw 'failed to restart the updated CLI' }\n" +
		"} catch {\n" +
		"  if ($_.Exception -is [UnauthorizedAccessException] -or $_.CategoryInfo.Category -eq 'PermissionDenied') {\n" +
		"    [Console]::Error.WriteLine(\"you do not have permission to update $($p.Target); ask your admin or move the binary to a writable path\")\n" +
		"  } else {\n" +
		"    [Console]::Error.WriteLine(\"CLI update failed: $($_.Exception.Message)\")\n" +
		"  }\n" +
		"  exit 1\n" +
		"} finally {\n" +
		"  Remove-Item -Force -Recurse -LiteralPath $p.TempDir -ErrorAction SilentlyContinue\n" +
		"}\n"
	encoded := utf16LEBase64(script)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encoded)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

func restartClient() error {
	return nil
}

func utf16LEBase64(value string) string {
	encoded := utf16.Encode([]rune(value))
	bytes := make([]byte, len(encoded)*2)
	for index, unit := range encoded {
		binary.LittleEndian.PutUint16(bytes[index*2:], unit)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}

func quoteWindowsArg(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n\v\"") {
		return value
	}
	var result strings.Builder
	result.WriteByte('"')
	backslashes := 0
	for _, char := range value {
		if char == '\\' {
			backslashes++
			continue
		}
		if char == '"' {
			result.WriteString(strings.Repeat("\\", backslashes*2+1))
			result.WriteRune(char)
			backslashes = 0
			continue
		}
		result.WriteString(strings.Repeat("\\", backslashes))
		backslashes = 0
		result.WriteRune(char)
	}
	result.WriteString(strings.Repeat("\\", backslashes*2))
	result.WriteByte('"')
	return result.String()
}

func joinWindowsArgs(args []string) string {
	quoted := make([]string, len(args))
	for index, arg := range args {
		quoted[index] = quoteWindowsArg(arg)
	}
	return strings.Join(quoted, " ")
}
