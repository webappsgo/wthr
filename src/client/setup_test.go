package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewSetupModel(t *testing.T) {
	m := newSetupModel()
	if m.serverURL != "" {
		t.Errorf("expected empty serverURL, got %q", m.serverURL)
	}
	if !m.saveToConfig {
		t.Error("expected saveToConfig to default to true")
	}
	if m.focusedField != 0 {
		t.Errorf("expected focusedField 0, got %d", m.focusedField)
	}
}

func TestSetupModelUpdateWindowSize(t *testing.T) {
	m := newSetupModel()
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	sm := updated.(setupModel)
	if sm.width != 100 || sm.height != 40 {
		t.Errorf("expected width/height 100/40, got %d/%d", sm.width, sm.height)
	}
	if cmd != nil {
		t.Error("expected nil cmd for window size update")
	}
}

func TestSetupModelUpdateTyping(t *testing.T) {
	m := newSetupModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	sm := updated.(setupModel)
	updated, _ = sm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	sm = updated.(setupModel)
	if sm.serverURL != "hi" {
		t.Errorf("expected serverURL 'hi', got %q", sm.serverURL)
	}
}

func TestSetupModelBackspace(t *testing.T) {
	m := newSetupModel()
	m.serverURL = "example.com"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	sm := updated.(setupModel)
	if sm.serverURL != "example.co" {
		t.Errorf("expected 'example.co', got %q", sm.serverURL)
	}

	t.Run("empty string backspace is no-op", func(t *testing.T) {
		m := newSetupModel()
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		sm := updated.(setupModel)
		if sm.serverURL != "" {
			t.Errorf("expected empty serverURL, got %q", sm.serverURL)
		}
	})
}

func TestSetupModelTabNavigation(t *testing.T) {
	m := newSetupModel()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	sm := updated.(setupModel)
	if sm.focusedField != 1 {
		t.Errorf("expected focusedField 1 after tab, got %d", sm.focusedField)
	}

	updated, _ = sm.Update(tea.KeyMsg{Type: tea.KeyTab})
	sm = updated.(setupModel)
	updated, _ = sm.Update(tea.KeyMsg{Type: tea.KeyTab})
	sm = updated.(setupModel)
	if sm.focusedField != 0 {
		t.Errorf("expected focusedField to wrap to 0, got %d", sm.focusedField)
	}

	// Shift-tab should go backward and wrap.
	updated, _ = sm.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	sm = updated.(setupModel)
	if sm.focusedField != 2 {
		t.Errorf("expected focusedField 2 after shift-tab wrap, got %d", sm.focusedField)
	}
}

func TestSetupModelToggleSaveToConfig(t *testing.T) {
	m := newSetupModel()
	m.focusedField = 1
	orig := m.saveToConfig

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	sm := updated.(setupModel)
	if sm.saveToConfig == orig {
		t.Error("expected saveToConfig to toggle on space")
	}

	updated, _ = sm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	sm = updated.(setupModel)
	if sm.saveToConfig != orig {
		t.Error("expected saveToConfig to toggle back on enter")
	}
}

func TestSetupModelCancel(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
	}{
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSetupModel()
			updated, cmd := m.Update(tt.key)
			sm := updated.(setupModel)
			if !sm.cancelled {
				t.Error("expected cancelled to be true")
			}
			if cmd == nil {
				t.Error("expected tea.Quit cmd")
			}
		})
	}
}

func TestSetupModelEnterTriggersTestConnection(t *testing.T) {
	m := newSetupModel()
	m.serverURL = "example.com"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	sm := updated.(setupModel)
	if !sm.testing {
		t.Error("expected testing to be true after enter with serverURL set")
	}
	if cmd == nil {
		t.Fatal("expected a test-connection cmd")
	}
}

func TestSetupModelEnterEmptyURLNoOp(t *testing.T) {
	m := newSetupModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	sm := updated.(setupModel)
	if sm.testing {
		t.Error("expected testing to remain false with empty serverURL")
	}
	if cmd != nil {
		t.Error("expected nil cmd when serverURL is empty")
	}
}

func TestSetupModelSetupMsgHandling(t *testing.T) {
	t.Run("success auto-proceeds", func(t *testing.T) {
		m := newSetupModel()
		m.testing = true
		updated, _ := m.Update(setupMsg{testSuccess: true, testResult: "Connected"})
		sm := updated.(setupModel)
		if sm.testing {
			t.Error("expected testing to be false")
		}
		if !sm.done {
			t.Error("expected done to be true on success")
		}
		if sm.testResult != "Connected" {
			t.Errorf("expected testResult 'Connected', got %q", sm.testResult)
		}
	})

	t.Run("failure does not proceed", func(t *testing.T) {
		m := newSetupModel()
		m.testing = true
		updated, _ := m.Update(setupMsg{testSuccess: false, testResult: "boom"})
		sm := updated.(setupModel)
		if sm.done {
			t.Error("expected done to remain false on failure")
		}
		if sm.testSuccess {
			t.Error("expected testSuccess false")
		}
	})
}

func TestSetupModelView(t *testing.T) {
	t.Run("renders default state", func(t *testing.T) {
		m := newSetupModel()
		view := m.View()
		if !strings.Contains(view, "WEATHER CLI SETUP") {
			t.Error("expected title in view")
		}
		if !strings.Contains(view, "https://") {
			t.Error("expected placeholder URL in view")
		}
	})

	t.Run("renders testing state", func(t *testing.T) {
		m := newSetupModel()
		m.testing = true
		view := m.View()
		if !strings.Contains(view, "Testing connection...") {
			t.Error("expected testing message in view")
		}
	})

	t.Run("renders success result", func(t *testing.T) {
		m := newSetupModel()
		m.testResult = "Connected to https://example.com"
		m.testSuccess = true
		view := m.View()
		if !strings.Contains(view, "Connected to https://example.com") {
			t.Error("expected success message in view")
		}
	})

	t.Run("renders failure result", func(t *testing.T) {
		m := newSetupModel()
		m.testResult = "Connection failed"
		m.testSuccess = false
		view := m.View()
		if !strings.Contains(view, "Connection failed") {
			t.Error("expected failure message in view")
		}
	})
}

func TestTestConnection(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/health" {
				t.Errorf("expected /api/v1/health, got %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cmd := testConnection(server.URL)
		msg := cmd().(setupMsg)
		if !msg.testSuccess {
			t.Errorf("expected success, got failure: %s", msg.testResult)
		}
	})

	t.Run("non-200 status is a failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		cmd := testConnection(server.URL)
		msg := cmd().(setupMsg)
		if msg.testSuccess {
			t.Error("expected failure for non-200 status")
		}
		if !strings.Contains(msg.testResult, "503") {
			t.Errorf("expected status code in message, got %q", msg.testResult)
		}
	})

	t.Run("connection refused is a failure", func(t *testing.T) {
		// Close the server immediately so the address is unreachable.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		addr := server.URL
		server.Close()

		cmd := testConnection(addr)
		msg := cmd().(setupMsg)
		if msg.testSuccess {
			t.Error("expected failure for unreachable server")
		}
		if !strings.Contains(msg.testResult, "Connection failed") {
			t.Errorf("expected connection failure message, got %q", msg.testResult)
		}
	})

	t.Run("adds https scheme and strips trailing slash", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Strip the scheme to force testConnection to add https://, which will
		// fail against a plain-HTTP httptest server - this documents that
		// behavior rather than asserting success.
		bare := strings.TrimPrefix(server.URL, "http://") + "/"
		cmd := testConnection(bare)
		msg := cmd().(setupMsg)
		if msg.testSuccess {
			t.Error("expected failure since https:// was forced onto a plain http test server")
		}
	})
}
