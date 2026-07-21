package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Setup wizard per AI.md PART 33 line 43869-44105
// Launched on first run when no server is configured

// setupModel is the bubbletea model for the setup wizard
type setupModel struct {
	serverURL      string
	focusedField   int
	testing        bool
	testResult     string
	testSuccess    bool
	cancelled      bool
	done           bool
	saveToConfig   bool
	width          int
	height         int
}

// setupMsg is a message type for setup wizard events
type setupMsg struct {
	testSuccess bool
	testResult  string
}

// newSetupModel creates a new setup model
func newSetupModel() setupModel {
	return setupModel{
		serverURL:    "",
		saveToConfig: true,
		focusedField: 0,
	}
}

// Init initializes the setup model
func (m setupModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the setup wizard
func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case setupMsg:
		m.testing = false
		m.testSuccess = msg.testSuccess
		m.testResult = msg.testResult
		if msg.testSuccess {
			// Auto-proceed after successful test
			m.done = true
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit

		case "enter":
			if m.focusedField == 0 && m.serverURL != "" {
				// Test connection
				m.testing = true
				return m, testConnection(m.serverURL)
			} else if m.focusedField == 1 {
				// Toggle save to config
				m.saveToConfig = !m.saveToConfig
			} else if m.focusedField == 2 {
				// Connect button
				if m.serverURL != "" {
					m.testing = true
					return m, testConnection(m.serverURL)
				}
			}

		case "tab", "down":
			m.focusedField = (m.focusedField + 1) % 3

		case "shift+tab", "up":
			m.focusedField = (m.focusedField + 2) % 3

		case "backspace":
			if m.focusedField == 0 && len(m.serverURL) > 0 {
				m.serverURL = m.serverURL[:len(m.serverURL)-1]
			}

		case " ":
			if m.focusedField == 1 {
				m.saveToConfig = !m.saveToConfig
			} else if m.focusedField == 0 {
				m.serverURL += " "
			}

		default:
			if m.focusedField == 0 && len(msg.String()) == 1 {
				m.serverURL += msg.String()
			}
		}
	}

	return m, nil
}

// View renders the setup wizard
func (m setupModel) View() string {
	// Styles using Dracula theme colors
	titleStyle := lipgloss.NewStyle().
		Foreground(colorPurple).
		Bold(true).
		Align(lipgloss.Center)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorPurple).
		Padding(1, 2)

	labelStyle := lipgloss.NewStyle().
		Foreground(colorForeground)

	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorComment).
		Padding(0, 1).
		Width(50)

	focusedInputStyle := inputStyle.
		BorderForeground(colorCyan)

	buttonStyle := lipgloss.NewStyle().
		Foreground(colorForeground).
		Background(colorSelection).
		Padding(0, 2)

	focusedButtonStyle := buttonStyle.
		Background(colorPurple).
		Foreground(colorBackground)

	successStyle := lipgloss.NewStyle().
		Foreground(colorGreen)

	errorStyle := lipgloss.NewStyle().
		Foreground(colorRed)

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("WEATHER CLI SETUP"))
	b.WriteString("\n\n")

	// Description
	b.WriteString(labelStyle.Render("No server configured. Let's set one up!"))
	b.WriteString("\n\n")

	// Server URL input
	b.WriteString(labelStyle.Render("Server URL:"))
	b.WriteString("\n")

	inputVal := m.serverURL
	if inputVal == "" {
		inputVal = "https://"
	}

	if m.focusedField == 0 {
		b.WriteString(focusedInputStyle.Render(inputVal + "_"))
	} else {
		b.WriteString(inputStyle.Render(inputVal))
	}
	b.WriteString("\n\n")

	// Save to config checkbox
	checkbox := "[ ]"
	if m.saveToConfig {
		checkbox = "[x]"
	}
	checkboxText := fmt.Sprintf("%s Save to configuration (recommended)", checkbox)
	if m.focusedField == 1 {
		b.WriteString(focusedButtonStyle.Render(checkboxText))
	} else {
		b.WriteString(labelStyle.Render(checkboxText))
	}
	b.WriteString("\n\n")

	// Test result
	if m.testing {
		b.WriteString(labelStyle.Render("Testing connection..."))
		b.WriteString("\n\n")
	} else if m.testResult != "" {
		if m.testSuccess {
			b.WriteString(successStyle.Render("✓ " + m.testResult))
		} else {
			b.WriteString(errorStyle.Render("✗ " + m.testResult))
		}
		b.WriteString("\n\n")
	}

	// Buttons
	connectBtn := "[Test Connection]"
	if m.focusedField == 2 {
		b.WriteString(focusedButtonStyle.Render(connectBtn))
	} else {
		b.WriteString(buttonStyle.Render(connectBtn))
	}
	b.WriteString("\n\n")

	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(colorComment)
	b.WriteString(helpStyle.Render("Tab/↑↓: navigate • Enter: select • Esc: cancel"))

	// Wrap in box
	return boxStyle.Render(b.String())
}

// testConnection tests the connection to the server
func testConnection(serverURL string) tea.Cmd {
	return func() tea.Msg {
		// Ensure URL has scheme
		if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
			serverURL = "https://" + serverURL
		}

		// Remove trailing slash
		serverURL = strings.TrimSuffix(serverURL, "/")

		// Try to connect to the health endpoint
		client := &http.Client{
			Timeout: 10 * time.Second,
		}

		// Try /api/v1/health first
		resp, err := client.Get(serverURL + "/api/v1/health")
		if err != nil {
			return setupMsg{
				testSuccess: false,
				testResult:  fmt.Sprintf("Connection failed: %v", err),
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return setupMsg{
				testSuccess: true,
				testResult:  fmt.Sprintf("Connected to %s", serverURL),
			}
		}

		return setupMsg{
			testSuccess: false,
			testResult:  fmt.Sprintf("Server returned status %d", resp.StatusCode),
		}
	}
}

// runSetupWizard launches the setup wizard
// Per AI.md PART 33 line 43869-44105
func runSetupWizard(config *CLIConfig) error {
	m := newSetupModel()

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("setup wizard error: %w", err)
	}

	model := finalModel.(setupModel)

	if model.cancelled {
		return NewUsageError("setup cancelled")
	}

	if model.done && model.testSuccess {
		// Normalize URL
		serverURL := model.serverURL
		if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
			serverURL = "https://" + serverURL
		}
		serverURL = strings.TrimSuffix(serverURL, "/")

		// Update config
		config.Server.Primary = serverURL

		// Save to config file if requested
		if model.saveToConfig {
			if err := SaveConfig(config); err != nil {
				fmt.Printf("Warning: could not save config: %v\n", err)
			} else {
				configPath, _ := ConfigPath()
				fmt.Printf("Configuration saved to %s\n", configPath)
			}
		}

		// Launch TUI
		return runTUI(config)
	}

	return nil
}
