package display

import (
	"testing"
)

// TestAutoDetectDisplayMode covers the full Headless/CLI/TUI/GUI decision
// matrix in autoDetectDisplayMode by constructing DisplayEnv literals
// directly, avoiding any dependency on the real (unmockable) terminal/env
// probing done by DetectDisplayEnv.
func TestAutoDetectDisplayMode(t *testing.T) {
	tests := []struct {
		name string
		env  DisplayEnv
		want DisplayMode
	}{
		{
			name: "no terminal no display is headless",
			env:  DisplayEnv{IsTerminal: false, HasDisplay: false},
			want: DisplayModeHeadless,
		},
		{
			name: "no terminal no display TERM dumb still headless (terminal check wins first)",
			env:  DisplayEnv{IsTerminal: false, HasDisplay: false, TerminalType: "dumb"},
			want: DisplayModeHeadless,
		},
		{
			name: "TERM dumb forces CLI even with display and terminal",
			env:  DisplayEnv{IsTerminal: true, HasDisplay: true, TerminalType: "dumb"},
			want: DisplayModeCLI,
		},
		{
			name: "TERM dumb forces CLI over SSH",
			env:  DisplayEnv{IsTerminal: true, HasDisplay: false, TerminalType: "dumb", IsSSH: true},
			want: DisplayModeCLI,
		},
		{
			name: "display without SSH or mosh is GUI",
			env:  DisplayEnv{IsTerminal: false, HasDisplay: true, TerminalType: "xterm"},
			want: DisplayModeGUI,
		},
		{
			name: "display with terminal without SSH or mosh is still GUI",
			env:  DisplayEnv{IsTerminal: true, HasDisplay: true, TerminalType: "xterm"},
			want: DisplayModeGUI,
		},
		{
			name: "display but over SSH falls through to terminal check (TUI)",
			env:  DisplayEnv{IsTerminal: true, HasDisplay: true, TerminalType: "xterm", IsSSH: true},
			want: DisplayModeTUI,
		},
		{
			name: "display but over SSH and not a terminal falls to CLI",
			env:  DisplayEnv{IsTerminal: false, HasDisplay: true, TerminalType: "xterm", IsSSH: true},
			want: DisplayModeCLI,
		},
		{
			name: "display but over mosh falls through to terminal check (TUI)",
			env:  DisplayEnv{IsTerminal: true, HasDisplay: true, TerminalType: "xterm", IsMosh: true},
			want: DisplayModeTUI,
		},
		{
			name: "display but over mosh and not a terminal falls to CLI",
			env:  DisplayEnv{IsTerminal: false, HasDisplay: true, TerminalType: "xterm", IsMosh: true},
			want: DisplayModeCLI,
		},
		{
			name: "terminal only (no display) is TUI",
			env:  DisplayEnv{IsTerminal: true, HasDisplay: false, TerminalType: "xterm"},
			want: DisplayModeTUI,
		},
		{
			name: "terminal only over SSH is still TUI",
			env:  DisplayEnv{IsTerminal: true, HasDisplay: false, TerminalType: "xterm", IsSSH: true},
			want: DisplayModeTUI,
		},
		{
			name: "neither terminal nor display, SSH set, is headless",
			env:  DisplayEnv{IsTerminal: false, HasDisplay: false, IsSSH: true},
			want: DisplayModeHeadless,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := tt.env
			got := env.autoDetectDisplayMode()
			if got != tt.want {
				t.Errorf("autoDetectDisplayMode() = %v, want %v (env=%+v)", got, tt.want, tt.env)
			}
		})
	}
}

func TestDetectDisplayEnv(t *testing.T) {
	t.Run("headless docker: everything unset", func(t *testing.T) {
		t.Setenv("TERM", "")
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("MOSH", "")
		t.Setenv("STY", "")
		t.Setenv("TMUX", "")

		env := DetectDisplayEnv()

		if env.IsSSH {
			t.Errorf("IsSSH = true, want false")
		}
		if env.IsMosh {
			t.Errorf("IsMosh = true, want false")
		}
		if env.IsScreen {
			t.Errorf("IsScreen = true, want false")
		}
		if env.HasDisplay {
			t.Errorf("HasDisplay = true, want false")
		}
		if env.DisplayType != "none" {
			t.Errorf("DisplayType = %q, want %q", env.DisplayType, "none")
		}
		switch env.Mode {
		case DisplayModeHeadless, DisplayModeCLI, DisplayModeTUI, DisplayModeGUI:
			// valid
		default:
			t.Errorf("Mode = %v is not a valid DisplayMode", env.Mode)
		}
	})

	t.Run("explicit dumb terminal", func(t *testing.T) {
		t.Setenv("TERM", "dumb")
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("MOSH", "")
		t.Setenv("STY", "")
		t.Setenv("TMUX", "")

		env := DetectDisplayEnv()

		if env.TerminalType != "dumb" {
			t.Errorf("TerminalType = %q, want %q", env.TerminalType, "dumb")
		}
		if !env.IsDumbTerminal() {
			t.Errorf("IsDumbTerminal() = false, want true")
		}
	})

	t.Run("SSH env vars detected", func(t *testing.T) {
		t.Setenv("TERM", "xterm")
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("SSH_CLIENT", "10.0.0.1 22 22")
		t.Setenv("SSH_TTY", "")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("MOSH", "")
		t.Setenv("STY", "")
		t.Setenv("TMUX", "")

		env := DetectDisplayEnv()

		if !env.IsSSH {
			t.Errorf("IsSSH = false, want true")
		}
	})

	t.Run("screen/tmux env vars detected", func(t *testing.T) {
		t.Setenv("TERM", "screen")
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("MOSH", "")
		t.Setenv("STY", "12345.pts-0.host")
		t.Setenv("TMUX", "")

		env := DetectDisplayEnv()

		if !env.IsScreen {
			t.Errorf("IsScreen = false, want true")
		}
	})

	t.Run("X11 DISPLAY set gives x11 display type", func(t *testing.T) {
		t.Setenv("TERM", "xterm")
		t.Setenv("DISPLAY", ":0")
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("MOSH", "")
		t.Setenv("STY", "")
		t.Setenv("TMUX", "")

		env := DetectDisplayEnv()

		if !env.HasDisplay {
			t.Errorf("HasDisplay = false, want true")
		}
		if env.DisplayType != "x11" {
			t.Errorf("DisplayType = %q, want %q", env.DisplayType, "x11")
		}
	})

	t.Run("Wayland takes precedence over X11 when both set", func(t *testing.T) {
		t.Setenv("TERM", "xterm")
		t.Setenv("DISPLAY", ":0")
		t.Setenv("WAYLAND_DISPLAY", "wayland-0")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("MOSH", "")
		t.Setenv("STY", "")
		t.Setenv("TMUX", "")

		env := DetectDisplayEnv()

		if !env.HasDisplay {
			t.Errorf("HasDisplay = false, want true")
		}
		if env.DisplayType != "wayland" {
			t.Errorf("DisplayType = %q, want %q (Wayland must take precedence over X11)", env.DisplayType, "wayland")
		}
	})
}

func TestIsDumbTerminal(t *testing.T) {
	tests := []struct {
		name string
		env  DisplayEnv
		want bool
	}{
		{"dumb", DisplayEnv{TerminalType: "dumb"}, true},
		{"xterm", DisplayEnv{TerminalType: "xterm"}, false},
		{"empty", DisplayEnv{TerminalType: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := tt.env
			if got := env.IsDumbTerminal(); got != tt.want {
				t.Errorf("IsDumbTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAutoDetectDisplayModeHelpers(t *testing.T) {
	tests := []struct {
		name       string
		mode       DisplayMode
		wantGUI    bool
		wantTUI    bool
		wantCLI    bool
		wantHeadl  bool
	}{
		{"headless", DisplayModeHeadless, false, false, false, true},
		{"cli", DisplayModeCLI, false, false, true, false},
		{"tui", DisplayModeTUI, false, true, false, false},
		{"gui", DisplayModeGUI, true, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := DisplayEnv{Mode: tt.mode}

			if got := env.IsAutoDetectDisplayModeGUI(); got != tt.wantGUI {
				t.Errorf("IsAutoDetectDisplayModeGUI() = %v, want %v", got, tt.wantGUI)
			}
			if got := env.IsAutoDetectDisplayModeTUI(); got != tt.wantTUI {
				t.Errorf("IsAutoDetectDisplayModeTUI() = %v, want %v", got, tt.wantTUI)
			}
			if got := env.IsAutoDetectDisplayModeCLI(); got != tt.wantCLI {
				t.Errorf("IsAutoDetectDisplayModeCLI() = %v, want %v", got, tt.wantCLI)
			}
			if got := env.IsAutoDetectDisplayModeHeadless(); got != tt.wantHeadl {
				t.Errorf("IsAutoDetectDisplayModeHeadless() = %v, want %v", got, tt.wantHeadl)
			}
		})
	}
}

func TestSupportsUnicode(t *testing.T) {
	tests := []struct {
		name string
		term string
		want bool
	}{
		{"empty terminal type", "", false},
		{"dumb", "dumb", false},
		{"xterm-256color", "xterm-256color", true},
		{"xterm", "xterm", true},
		{"vt100", "vt100", true},
		{"screen", "screen", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := DisplayEnv{TerminalType: tt.term}
			if got := env.SupportsUnicode(); got != tt.want {
				t.Errorf("SupportsUnicode() with TerminalType=%q = %v, want %v", tt.term, got, tt.want)
			}
		})
	}
}
