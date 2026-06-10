package main

import (
	"fmt"
	"net/url"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/casapps/wthr/src/common/terminal"
)

// Dracula theme colors per AI.md PART 16
var (
	colorBackground = lipgloss.Color("#282a36")
	colorForeground = lipgloss.Color("#f8f8f2")
	colorSelection  = lipgloss.Color("#44475a")
	colorComment    = lipgloss.Color("#6272a4")
	colorCyan       = lipgloss.Color("#8be9fd")
	colorGreen      = lipgloss.Color("#50fa7b")
	colorOrange     = lipgloss.Color("#ffb86c")
	colorPink       = lipgloss.Color("#ff79c6")
	colorPurple     = lipgloss.Color("#bd93f9")
	colorRed        = lipgloss.Color("#ff5555")
	colorYellow     = lipgloss.Color("#f1fa8c")
)

// TUI view types
type tuiView int

const (
	viewMenu tuiView = iota
	viewInput
	viewResult
	viewHelp
)

// Menu item
type menuItem struct {
	label   string
	command string
	icon    string
}

// tuiModel represents the TUI state
// Per AI.md PART 33: TUI must have 100% feature coverage and be responsive
type tuiModel struct {
	config       *CLIConfig
	view         tuiView
	previousView tuiView
	cursor       int
	menuItems    []menuItem
	input        string
	inputLabel   string
	inputPrompt  string
	result       string
	resultTitle  string
	loading      bool
	err          error
	width        int
	height       int
	sizeMode     terminal.SizeMode
	scrollOffset int
}

// apiResultMsg is returned when an API call completes
type apiResultMsg struct {
	title  string
	result string
	err    error
}

// newTUIModel creates a new TUI model
func newTUIModel(config *CLIConfig) tuiModel {
	return tuiModel{
		config: config,
		view:   viewMenu,
		cursor: 0,
		menuItems: []menuItem{
			{label: "Current Weather", command: "current", icon: "☀"},
			{label: "Forecast", command: "forecast", icon: "📅"},
			{label: "Alerts", command: "alerts", icon: "⚠"},
			{label: "Moon Phase", command: "moon", icon: "🌙"},
			{label: "Historical", command: "history", icon: "📜"},
			{label: "Earthquakes", command: "earthquakes", icon: "🌍"},
			{label: "Hurricanes", command: "hurricanes", icon: "🌀"},
		},
		width:    80,
		height:   24,
		sizeMode: terminal.SizeModeStandard,
	}
}

// Init initializes the TUI
func (m tuiModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		// Per AI.md PART 33 line 46286-46316: Window awareness
		m.width = msg.Width
		m.height = msg.Height
		m.sizeMode = terminal.GetTerminalSize().Mode
		return m, nil
	case apiResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.result = ""
		} else {
			m.err = nil
			m.result = msg.result
			m.resultTitle = msg.title
		}
		m.view = viewResult
		return m, nil
	}
	return m, nil
}


// handleKeyPress handles keyboard input
// Per AI.md PART 33 line 46564-46580: Vim-style navigation
func (m tuiModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.view {
	case viewMenu:
		return m.handleMenuKeys(msg)
	case viewInput:
		return m.handleInputKeys(msg)
	case viewResult:
		return m.handleResultKeys(msg)
	case viewHelp:
		return m.handleHelpKeys(msg)
	}
	return m, nil
}

func (m tuiModel) handleMenuKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.previousView = m.view
		m.view = viewHelp
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.menuItems)-1 {
			m.cursor++
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.menuItems) - 1
	case "enter", "l":
		return m.selectMenuItem()
	}
	return m, nil
}

func (m tuiModel) handleInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "escape", "ctrl+[":
		m.view = viewMenu
		m.input = ""
		return m, nil
	case "enter":
		return m.submitInput()
	case "backspace", "ctrl+h":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.input += msg.String()
		}
	}
	return m, nil
}

func (m tuiModel) handleResultKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "escape", "b", "h", "ctrl+[":
		m.view = viewMenu
		m.result = ""
		m.err = nil
		return m, nil
	case "?":
		m.previousView = m.view
		m.view = viewHelp
		return m, nil
	case "up", "k":
		if m.scrollOffset > 0 {
			m.scrollOffset--
		}
	case "down", "j":
		m.scrollOffset++
	case "home", "g":
		m.scrollOffset = 0
	}
	return m, nil
}

func (m tuiModel) handleHelpKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "escape", "?", "enter", "ctrl+[":
		m.view = m.previousView
		return m, nil
	}
	return m, nil
}

func (m tuiModel) selectMenuItem() (tea.Model, tea.Cmd) {
	item := m.menuItems[m.cursor]

	switch item.command {
	case "current", "forecast", "alerts":
		m.inputLabel = "Location"
		m.inputPrompt = "Enter city name or ZIP code"
		m.view = viewInput
		m.input = m.config.GetDefaultLocation()
	case "history":
		m.inputLabel = "Location,Date"
		m.inputPrompt = "Enter location,date (e.g., NYC,2024-01-15)"
		m.view = viewInput
		m.input = ""
	case "moon":
		m.inputLabel = "Date"
		m.inputPrompt = "Enter date (YYYY-MM-DD) or press Enter for today"
		m.view = viewInput
		m.input = ""
	case "earthquakes", "hurricanes":
		// No input needed, fetch directly
		m.loading = true
		return m, m.fetchData(item.command, "")
	}
	return m, nil
}

func (m tuiModel) submitInput() (tea.Model, tea.Cmd) {
	item := m.menuItems[m.cursor]
	m.loading = true
	return m, m.fetchData(item.command, m.input)
}

func (m tuiModel) fetchData(command, input string) tea.Cmd {
	return func() tea.Msg {
		client := NewHTTPClient(m.config)
		apiPath := m.config.GetAPIPath()
		var path string
		var result map[string]interface{}
		var title string

		switch command {
		case "current":
			loc := input
			if loc == "" {
				loc = m.config.GetDefaultLocation()
			}
			if loc == "" {
				return apiResultMsg{err: fmt.Errorf("please enter a location")}
			}
			path = fmt.Sprintf("%s/weather?location=%s", apiPath, url.QueryEscape(loc))
			title = "Current Weather"
		case "forecast":
			loc := input
			if loc == "" {
				loc = m.config.GetDefaultLocation()
			}
			if loc == "" {
				return apiResultMsg{err: fmt.Errorf("please enter a location")}
			}
			path = fmt.Sprintf("%s/forecasts?location=%s&days=7", apiPath, url.QueryEscape(loc))
			title = "7-Day Forecast"
		case "alerts":
			loc := input
			if loc == "" {
				loc = m.config.GetDefaultLocation()
			}
			if loc == "" {
				return apiResultMsg{err: fmt.Errorf("please enter a location")}
			}
			path = fmt.Sprintf("%s/weather/alerts?location=%s", apiPath, url.QueryEscape(loc))
			title = "Weather Alerts"
		case "history":
			parts := strings.SplitN(input, ",", 2)
			if len(parts) < 2 {
				return apiResultMsg{err: fmt.Errorf("enter location,date (e.g., NYC,2024-01-15)")}
			}
			loc := strings.TrimSpace(parts[0])
			date := strings.TrimSpace(parts[1])
			path = fmt.Sprintf("%s/weather/history?location=%s&date=%s", apiPath, url.QueryEscape(loc), url.QueryEscape(date))
			title = fmt.Sprintf("Historical Weather - %s", date)
		case "moon":
			path = fmt.Sprintf("%s/weather/moon", apiPath)
			if input != "" {
				path += "?date=" + url.QueryEscape(input)
			}
			title = "Moon Phase"
		case "earthquakes":
			path = fmt.Sprintf("%s/earthquakes?limit=10", apiPath)
			title = "Recent Earthquakes"
		case "hurricanes":
			path = fmt.Sprintf("%s/hurricanes?active=true", apiPath)
			title = "Active Hurricanes"
		}

		if err := client.GetJSON(path, &result); err != nil {
			return apiResultMsg{err: err}
		}

		formatter := NewFormatter("table", false)
		var formatted string
		switch command {
		case "current", "history":
			formatted = formatter.FormatWeatherCurrent(result)
		case "forecast":
			formatted = formatter.FormatForecast(result)
		case "alerts":
			formatted = formatter.FormatAlerts(result)
		case "moon":
			formatted = formatter.FormatMoon(result)
		default:
			formatted = formatter.FormatJSON(result)
		}

		return apiResultMsg{title: title, result: formatted}
	}
}

// View renders the TUI
func (m tuiModel) View() string {
	switch m.view {
	case viewMenu:
		return m.renderMenu()
	case viewInput:
		return m.renderInput()
	case viewResult:
		return m.renderResult()
	case viewHelp:
		return m.renderHelp()
	}
	return ""
}

func (m tuiModel) renderMenu() string {
	var s strings.Builder

	// Styles based on size mode
	titleStyle := lipgloss.NewStyle().
		Foreground(colorCyan).
		Bold(true)

	itemStyle := lipgloss.NewStyle().
		Foreground(colorForeground)

	selectedStyle := lipgloss.NewStyle().
		Foreground(colorGreen).
		Bold(true)

	helpStyle := lipgloss.NewStyle().
		Foreground(colorComment)

	// Header
	if m.sizeMode >= terminal.SizeModeCompact {
		s.WriteString(titleStyle.Render("Weather CLI"))
		s.WriteString("\n")
		s.WriteString(helpStyle.Render(m.config.Server.Primary))
		s.WriteString("\n\n")
	}

	// Menu items
	for i, item := range m.menuItems {
		var line string
		if m.sizeMode >= terminal.SizeModeStandard {
			// Full display with icon
			if i == m.cursor {
				line = selectedStyle.Render(fmt.Sprintf(" → %s %s", item.icon, item.label))
			} else {
				line = itemStyle.Render(fmt.Sprintf("   %s %s", item.icon, item.label))
			}
		} else if m.sizeMode >= terminal.SizeModeMinimal {
			// Compact display without icon
			if i == m.cursor {
				line = selectedStyle.Render(fmt.Sprintf("> %s", item.label))
			} else {
				line = itemStyle.Render(fmt.Sprintf("  %s", item.label))
			}
		} else {
			// Micro display - abbreviated
			if i == m.cursor {
				line = selectedStyle.Render(fmt.Sprintf(">%s", item.command[:3]))
			} else {
				line = itemStyle.Render(fmt.Sprintf(" %s", item.command[:3]))
			}
		}
		s.WriteString(line)
		s.WriteString("\n")
	}

	// Help footer per AI.md PART 33 line 46383-46394
	s.WriteString("\n")
	s.WriteString(helpStyle.Render(m.getHelpText()))

	return s.String()
}

func (m tuiModel) renderInput() string {
	var s strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(colorCyan).
		Bold(true)

	promptStyle := lipgloss.NewStyle().
		Foreground(colorComment)

	inputStyle := lipgloss.NewStyle().
		Foreground(colorYellow)

	helpStyle := lipgloss.NewStyle().
		Foreground(colorComment)

	s.WriteString(titleStyle.Render(m.inputLabel))
	s.WriteString("\n")
	if m.sizeMode >= terminal.SizeModeCompact {
		s.WriteString(promptStyle.Render(m.inputPrompt))
		s.WriteString("\n")
	}
	s.WriteString("\n")
	s.WriteString(inputStyle.Render("> " + m.input + "█"))
	s.WriteString("\n\n")
	s.WriteString(helpStyle.Render("Enter: submit │ Esc: back"))

	return s.String()
}

func (m tuiModel) renderResult() string {
	var s strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(colorCyan).
		Bold(true)

	errorStyle := lipgloss.NewStyle().
		Foreground(colorRed)

	helpStyle := lipgloss.NewStyle().
		Foreground(colorComment)

	if m.loading {
		s.WriteString(titleStyle.Render("Loading..."))
		return s.String()
	}

	if m.err != nil {
		s.WriteString(errorStyle.Render("Error: " + m.err.Error()))
		s.WriteString("\n\n")
		s.WriteString(helpStyle.Render("Esc/b: back │ q: quit"))
		return s.String()
	}

	s.WriteString(titleStyle.Render(m.resultTitle))
	s.WriteString("\n\n")

	// Scroll the result if needed
	lines := strings.Split(m.result, "\n")
	viewHeight := m.height - 6 // Reserve space for header and footer
	if viewHeight < 3 {
		viewHeight = 3
	}

	start := m.scrollOffset
	if start > len(lines)-viewHeight {
		start = len(lines) - viewHeight
	}
	if start < 0 {
		start = 0
	}
	end := start + viewHeight
	if end > len(lines) {
		end = len(lines)
	}

	for _, line := range lines[start:end] {
		// Truncate long lines
		if len(line) > m.width-2 {
			line = line[:m.width-5] + "..."
		}
		s.WriteString(line)
		s.WriteString("\n")
	}

	s.WriteString("\n")
	if len(lines) > viewHeight {
		s.WriteString(helpStyle.Render(fmt.Sprintf("j/k: scroll │ %d/%d", start+1, len(lines))))
	}
	s.WriteString(helpStyle.Render(" │ b: back │ q: quit"))

	return s.String()
}

func (m tuiModel) renderHelp() string {
	var s strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(colorCyan).
		Bold(true)

	keyStyle := lipgloss.NewStyle().
		Foreground(colorYellow)

	descStyle := lipgloss.NewStyle().
		Foreground(colorForeground)

	helpStyle := lipgloss.NewStyle().
		Foreground(colorComment)

	s.WriteString(titleStyle.Render("Keyboard Shortcuts"))
	s.WriteString("\n\n")

	keys := []struct{ key, desc string }{
		{"j / ↓", "Move down"},
		{"k / ↑", "Move up"},
		{"g", "Go to top"},
		{"G", "Go to bottom"},
		{"Enter / l", "Select"},
		{"Esc / b / h", "Back"},
		{"?", "Show help"},
		{"q", "Quit"},
	}

	for _, k := range keys {
		s.WriteString(keyStyle.Render(fmt.Sprintf("  %-12s", k.key)))
		s.WriteString(descStyle.Render(k.desc))
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(helpStyle.Render("Press Esc or ? to close"))

	return s.String()
}

// getHelpText returns help text appropriate for terminal size
// Per AI.md PART 33 line 46383-46394
func (m tuiModel) getHelpText() string {
	switch m.sizeMode {
	case terminal.SizeModeMicro:
		return "?:help q:quit"
	case terminal.SizeModeMinimal:
		return "j/k:nav │ Enter:select │ ?:help │ q:quit"
	default:
		return "↑/↓ or j/k: Navigate │ Enter: Select │ ?: Help │ q: Quit"
	}
}

// runTUI starts the TUI
func runTUI(config *CLIConfig) error {
	p := tea.NewProgram(
		newTUIModel(config),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}
