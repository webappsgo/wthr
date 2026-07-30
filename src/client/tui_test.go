package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/webappsgo/wthr/src/common/terminal"
)

func TestNewTUIModel(t *testing.T) {
	config := DefaultConfig()
	m := newTUIModel(config)

	if m.view != viewMenu {
		t.Errorf("expected initial view to be viewMenu, got %v", m.view)
	}
	if m.cursor != 0 {
		t.Errorf("expected initial cursor 0, got %d", m.cursor)
	}
	if len(m.menuItems) != 7 {
		t.Errorf("expected 7 menu items, got %d", len(m.menuItems))
	}
}

func TestTUIModelUpdateWindowSize(t *testing.T) {
	m := newTUIModel(DefaultConfig())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	tm := updated.(tuiModel)
	if tm.width != 120 || tm.height != 50 {
		t.Errorf("expected width/height 120/50, got %d/%d", tm.width, tm.height)
	}
}

func TestTUIModelUpdateAPIResultMsg(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		m := newTUIModel(DefaultConfig())
		m.loading = true
		updated, _ := m.Update(apiResultMsg{title: "Current Weather", result: "72F"})
		tm := updated.(tuiModel)
		if tm.loading {
			t.Error("expected loading false")
		}
		if tm.view != viewResult {
			t.Errorf("expected viewResult, got %v", tm.view)
		}
		if tm.result != "72F" || tm.resultTitle != "Current Weather" {
			t.Errorf("unexpected result/title: %q/%q", tm.result, tm.resultTitle)
		}
		if tm.err != nil {
			t.Errorf("expected nil err, got %v", tm.err)
		}
	})

	t.Run("error clears result", func(t *testing.T) {
		m := newTUIModel(DefaultConfig())
		m.result = "stale"
		updated, _ := m.Update(apiResultMsg{err: errBoom})
		tm := updated.(tuiModel)
		if tm.err == nil {
			t.Error("expected err to be set")
		}
		if tm.result != "" {
			t.Errorf("expected result cleared, got %q", tm.result)
		}
		if tm.view != viewResult {
			t.Errorf("expected viewResult, got %v", tm.view)
		}
	})
}

var errBoom = &ExitError{Message: "boom", Code: ExitGeneralError}

func TestTUIModelMenuNavigation(t *testing.T) {
	m := newTUIModel(DefaultConfig())

	t.Run("down moves cursor forward", func(t *testing.T) {
		updated, _ := m.handleMenuKeys(tea.KeyMsg{Type: tea.KeyDown})
		tm := updated.(tuiModel)
		if tm.cursor != 1 {
			t.Errorf("expected cursor 1, got %d", tm.cursor)
		}
	})

	t.Run("up at top stays at 0", func(t *testing.T) {
		updated, _ := m.handleMenuKeys(tea.KeyMsg{Type: tea.KeyUp})
		tm := updated.(tuiModel)
		if tm.cursor != 0 {
			t.Errorf("expected cursor to stay at 0, got %d", tm.cursor)
		}
	})

	t.Run("end goes to last item", func(t *testing.T) {
		updated, _ := m.handleMenuKeys(tea.KeyMsg{Type: tea.KeyEnd})
		tm := updated.(tuiModel)
		if tm.cursor != len(tm.menuItems)-1 {
			t.Errorf("expected cursor %d, got %d", len(tm.menuItems)-1, tm.cursor)
		}
	})

	t.Run("down at bottom stays at last", func(t *testing.T) {
		m2 := m
		m2.cursor = len(m2.menuItems) - 1
		updated, _ := m2.handleMenuKeys(tea.KeyMsg{Type: tea.KeyDown})
		tm := updated.(tuiModel)
		if tm.cursor != len(tm.menuItems)-1 {
			t.Errorf("expected cursor to stay at last item, got %d", tm.cursor)
		}
	})

	t.Run("home resets cursor", func(t *testing.T) {
		m2 := m
		m2.cursor = 3
		updated, _ := m2.handleMenuKeys(tea.KeyMsg{Type: tea.KeyHome})
		tm := updated.(tuiModel)
		if tm.cursor != 0 {
			t.Errorf("expected cursor 0, got %d", tm.cursor)
		}
	})

	t.Run("quit returns tea.Quit", func(t *testing.T) {
		_, cmd := m.handleMenuKeys(tea.KeyMsg{Type: tea.KeyCtrlC})
		if cmd == nil {
			t.Error("expected tea.Quit cmd")
		}
	})

	t.Run("help key switches view", func(t *testing.T) {
		updated, _ := m.handleMenuKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
		tm := updated.(tuiModel)
		if tm.view != viewHelp {
			t.Errorf("expected viewHelp, got %v", tm.view)
		}
		if tm.previousView != viewMenu {
			t.Errorf("expected previousView viewMenu, got %v", tm.previousView)
		}
	})
}

func TestTUIModelSelectMenuItem(t *testing.T) {
	t.Run("current selects input view with default location", func(t *testing.T) {
		config := DefaultConfig()
		config.Location = "Denver"
		m := newTUIModel(config)
		m.cursor = 0 // current

		updated, cmd := m.selectMenuItem()
		tm := updated.(tuiModel)
		if tm.view != viewInput {
			t.Errorf("expected viewInput, got %v", tm.view)
		}
		if tm.input != "Denver" {
			t.Errorf("expected input prefilled with 'Denver', got %q", tm.input)
		}
		if cmd != nil {
			t.Error("expected nil cmd for input-collecting items")
		}
	})

	t.Run("history selects input view with empty input", func(t *testing.T) {
		m := newTUIModel(DefaultConfig())
		m.cursor = 4 // history
		updated, _ := m.selectMenuItem()
		tm := updated.(tuiModel)
		if tm.view != viewInput {
			t.Errorf("expected viewInput, got %v", tm.view)
		}
		if tm.input != "" {
			t.Errorf("expected empty input, got %q", tm.input)
		}
	})

	t.Run("earthquakes fetches directly without input", func(t *testing.T) {
		m := newTUIModel(DefaultConfig())
		m.cursor = 5 // earthquakes
		updated, cmd := m.selectMenuItem()
		tm := updated.(tuiModel)
		if !tm.loading {
			t.Error("expected loading true")
		}
		if cmd == nil {
			t.Error("expected a fetch cmd")
		}
	})
}

func TestTUIModelInputKeys(t *testing.T) {
	m := newTUIModel(DefaultConfig())
	m.view = viewInput

	t.Run("typing appends to input", func(t *testing.T) {
		updated, _ := m.handleInputKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
		tm := updated.(tuiModel)
		if tm.input != "N" {
			t.Errorf("expected input 'N', got %q", tm.input)
		}
	})

	t.Run("backspace removes last char", func(t *testing.T) {
		m2 := m
		m2.input = "NYC"
		updated, _ := m2.handleInputKeys(tea.KeyMsg{Type: tea.KeyBackspace})
		tm := updated.(tuiModel)
		if tm.input != "NY" {
			t.Errorf("expected 'NY', got %q", tm.input)
		}
	})

	t.Run("escape returns to menu and clears input", func(t *testing.T) {
		m2 := m
		m2.input = "NYC"
		updated, _ := m2.handleInputKeys(tea.KeyMsg{Type: tea.KeyEsc})
		tm := updated.(tuiModel)
		if tm.view != viewMenu {
			t.Errorf("expected viewMenu, got %v", tm.view)
		}
		if tm.input != "" {
			t.Errorf("expected input cleared, got %q", tm.input)
		}
	})

	t.Run("enter submits and sets loading", func(t *testing.T) {
		m2 := m
		m2.input = "NYC"
		updated, cmd := m2.handleInputKeys(tea.KeyMsg{Type: tea.KeyEnter})
		tm := updated.(tuiModel)
		if !tm.loading {
			t.Error("expected loading true after submit")
		}
		if cmd == nil {
			t.Error("expected a fetch cmd")
		}
	})
}

func TestTUIModelResultKeys(t *testing.T) {
	m := newTUIModel(DefaultConfig())
	m.view = viewResult
	m.result = "some weather data"

	t.Run("escape returns to menu and clears result", func(t *testing.T) {
		updated, _ := m.handleResultKeys(tea.KeyMsg{Type: tea.KeyEsc})
		tm := updated.(tuiModel)
		if tm.view != viewMenu {
			t.Errorf("expected viewMenu, got %v", tm.view)
		}
		if tm.result != "" {
			t.Errorf("expected result cleared, got %q", tm.result)
		}
	})

	t.Run("down increases scroll offset", func(t *testing.T) {
		updated, _ := m.handleResultKeys(tea.KeyMsg{Type: tea.KeyDown})
		tm := updated.(tuiModel)
		if tm.scrollOffset != 1 {
			t.Errorf("expected scrollOffset 1, got %d", tm.scrollOffset)
		}
	})

	t.Run("up at zero stays at zero", func(t *testing.T) {
		updated, _ := m.handleResultKeys(tea.KeyMsg{Type: tea.KeyUp})
		tm := updated.(tuiModel)
		if tm.scrollOffset != 0 {
			t.Errorf("expected scrollOffset to stay 0, got %d", tm.scrollOffset)
		}
	})

	t.Run("home resets scroll", func(t *testing.T) {
		m2 := m
		m2.scrollOffset = 5
		updated, _ := m2.handleResultKeys(tea.KeyMsg{Type: tea.KeyHome})
		tm := updated.(tuiModel)
		if tm.scrollOffset != 0 {
			t.Errorf("expected scrollOffset 0, got %d", tm.scrollOffset)
		}
	})

	t.Run("quit returns tea.Quit", func(t *testing.T) {
		_, cmd := m.handleResultKeys(tea.KeyMsg{Type: tea.KeyCtrlC})
		if cmd == nil {
			t.Error("expected tea.Quit cmd")
		}
	})
}

func TestTUIModelHelpKeys(t *testing.T) {
	m := newTUIModel(DefaultConfig())
	m.view = viewHelp
	m.previousView = viewResult

	updated, _ := m.handleHelpKeys(tea.KeyMsg{Type: tea.KeyEsc})
	tm := updated.(tuiModel)
	if tm.view != viewResult {
		t.Errorf("expected to return to viewResult, got %v", tm.view)
	}

	_, cmd := m.handleHelpKeys(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestTUIModelFetchData(t *testing.T) {
	newServerConfig := func(url string) *CLIConfig {
		return &CLIConfig{
			Server: ServerConfig{Primary: url, APIVersion: "v1"},
		}
	}

	tests := []struct {
		name        string
		command     string
		input       string
		wantPathHas string
	}{
		{"current", "current", "NYC", "/weather?location=NYC"},
		{"forecast", "forecast", "NYC", "/forecasts?location=NYC&days=7"},
		{"alerts", "alerts", "NYC", "/weather/alerts?location=NYC"},
		{"moon with date", "moon", "2024-01-01", "/weather/moon?date=2024-01-01"},
		{"moon without date", "moon", "", "/weather/moon"},
		{"earthquakes", "earthquakes", "", "/earthquakes?limit=10"},
		{"hurricanes", "hurricanes", "", "/hurricanes?active=true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.String()
				w.Write([]byte(`{"temperature":70}`))
			}))
			defer server.Close()

			m := newTUIModel(newServerConfig(server.URL))
			cmd := m.fetchData(tt.command, tt.input)
			msg := cmd().(apiResultMsg)
			if msg.err != nil {
				t.Fatalf("unexpected error: %v", msg.err)
			}
			if !strings.Contains(gotPath, tt.wantPathHas) {
				t.Errorf("expected path to contain %q, got %q", tt.wantPathHas, gotPath)
			}
		})
	}

	t.Run("history requires location,date format", func(t *testing.T) {
		m := newTUIModel(newServerConfig("http://example.invalid"))
		cmd := m.fetchData("history", "no-comma-here")
		msg := cmd().(apiResultMsg)
		if msg.err == nil {
			t.Fatal("expected error for malformed history input")
		}
	})

	t.Run("history splits location and date", func(t *testing.T) {
		var gotPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.String()
			w.Write([]byte(`{}`))
		}))
		defer server.Close()

		m := newTUIModel(newServerConfig(server.URL))
		cmd := m.fetchData("history", "NYC,2024-01-15")
		msg := cmd().(apiResultMsg)
		if msg.err != nil {
			t.Fatalf("unexpected error: %v", msg.err)
		}
		if !strings.Contains(gotPath, "location=NYC") || !strings.Contains(gotPath, "date=2024-01-15") {
			t.Errorf("unexpected path: %s", gotPath)
		}
	})

	t.Run("current with no input and no default location errors", func(t *testing.T) {
		m := newTUIModel(newServerConfig("http://example.invalid"))
		cmd := m.fetchData("current", "")
		msg := cmd().(apiResultMsg)
		if msg.err == nil {
			t.Fatal("expected error when no location available")
		}
	})

	t.Run("server error surfaces as apiResultMsg error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		m := newTUIModel(newServerConfig(server.URL))
		cmd := m.fetchData("current", "NYC")
		msg := cmd().(apiResultMsg)
		if msg.err == nil {
			t.Fatal("expected error from server failure")
		}
	})
}

func TestTUIModelSubmitInput(t *testing.T) {
	m := newTUIModel(DefaultConfig())
	m.cursor = 0
	m.input = "NYC"

	updated, cmd := m.submitInput()
	tm := updated.(tuiModel)
	if !tm.loading {
		t.Error("expected loading true")
	}
	if cmd == nil {
		t.Error("expected a fetch cmd")
	}
}

func TestTUIModelView(t *testing.T) {
	config := DefaultConfig()

	t.Run("menu view", func(t *testing.T) {
		m := newTUIModel(config)
		view := m.View()
		if !strings.Contains(view, "Current Weather") {
			t.Error("expected menu items in view")
		}
	})

	t.Run("input view", func(t *testing.T) {
		m := newTUIModel(config)
		m.view = viewInput
		m.inputLabel = "Location"
		m.input = "NYC"
		view := m.View()
		if !strings.Contains(view, "NYC") {
			t.Error("expected input value in view")
		}
	})

	t.Run("result view loading", func(t *testing.T) {
		m := newTUIModel(config)
		m.view = viewResult
		m.loading = true
		view := m.View()
		if !strings.Contains(view, "Loading...") {
			t.Error("expected loading text in view")
		}
	})

	t.Run("result view error", func(t *testing.T) {
		m := newTUIModel(config)
		m.view = viewResult
		m.err = errBoom
		view := m.View()
		if !strings.Contains(view, "Error:") {
			t.Error("expected error text in view")
		}
	})

	t.Run("result view with data", func(t *testing.T) {
		m := newTUIModel(config)
		m.view = viewResult
		m.resultTitle = "Current Weather"
		m.result = "72F sunny"
		m.width = 80
		m.height = 24
		view := m.View()
		if !strings.Contains(view, "72F sunny") {
			t.Error("expected result content in view")
		}
	})

	t.Run("help view", func(t *testing.T) {
		m := newTUIModel(config)
		m.view = viewHelp
		view := m.View()
		if !strings.Contains(view, "Keyboard Shortcuts") {
			t.Error("expected help title in view")
		}
	})
}

func TestGetHelpText(t *testing.T) {
	tests := []struct {
		mode terminal.SizeMode
		want string
	}{
		{terminal.SizeModeMicro, "?:help q:quit"},
		{terminal.SizeModeMinimal, "j/k:nav │ Enter:select │ ?:help │ q:quit"},
		{terminal.SizeModeStandard, "↑/↓ or j/k: Navigate │ Enter: Select │ ?: Help │ q: Quit"},
	}

	for _, tt := range tests {
		m := newTUIModel(DefaultConfig())
		m.sizeMode = tt.mode
		if got := m.getHelpText(); got != tt.want {
			t.Errorf("getHelpText(%v) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}
