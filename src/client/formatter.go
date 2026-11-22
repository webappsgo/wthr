package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Formatter handles output formatting
type Formatter struct {
	Format  string
	NoColor bool
}

// NewFormatter creates a new formatter
func NewFormatter(format string, noColor bool) *Formatter {
	return &Formatter{
		Format:  format,
		NoColor: noColor,
	}
}

// FormatWeatherCurrent formats current weather data
func (f *Formatter) FormatWeatherCurrent(data map[string]interface{}) string {
	switch f.Format {
	case "json":
		return f.formatJSON(data)
	case "plain":
		return f.formatPlainWeather(data)
	// table
	default:
		return f.formatTableWeather(data)
	}
}

// FormatForecast formats forecast data
func (f *Formatter) FormatForecast(data map[string]interface{}) string {
	switch f.Format {
	case "json":
		return f.formatJSON(data)
	case "plain":
		return f.formatPlainForecast(data)
	// table
	default:
		return f.formatTableForecast(data)
	}
}

// FormatAlerts formats alert data
func (f *Formatter) FormatAlerts(data map[string]interface{}) string {
	switch f.Format {
	case "json":
		return f.formatJSON(data)
	case "plain":
		return f.formatPlainAlerts(data)
	// table
	default:
		return f.formatTableAlerts(data)
	}
}

// FormatMoon formats moon phase data
func (f *Formatter) FormatMoon(data map[string]interface{}) string {
	switch f.Format {
	case "json":
		return f.formatJSON(data)
	case "plain":
		return f.formatPlainMoon(data)
	// table
	default:
		return f.formatTableMoon(data)
	}
}

// FormatJSON formats data as JSON (public method)
func (f *Formatter) FormatJSON(data interface{}) string {
	return f.formatJSON(data)
}

// formatJSON formats data as JSON (internal)
func (f *Formatter) formatJSON(data interface{}) string {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error formatting JSON: %v", err)
	}
	return string(jsonData)
}

// formatPlainWeather formats weather as plain text
func (f *Formatter) formatPlainWeather(data map[string]interface{}) string {
	var sb strings.Builder

	if location, ok := data["location"].(string); ok {
		fmt.Fprintf(&sb, "Location: %s\n", location)
	}

	if temp, ok := data["temperature"].(float64); ok {
		fmt.Fprintf(&sb, "Temperature: %.1f°F\n", temp)
	}

	if feels, ok := data["feels_like"].(float64); ok {
		fmt.Fprintf(&sb, "Feels Like: %.1f°F\n", feels)
	}

	if cond, ok := data["condition"].(string); ok {
		fmt.Fprintf(&sb, "Condition: %s\n", cond)
	}

	if humid, ok := data["humidity"].(float64); ok {
		fmt.Fprintf(&sb, "Humidity: %.0f%%\n", humid)
	}

	if wind, ok := data["wind_speed"].(float64); ok {
		fmt.Fprintf(&sb, "Wind Speed: %.1f mph\n", wind)
	}

	return sb.String()
}

// formatTableWeather formats weather as a table
func (f *Formatter) formatTableWeather(data map[string]interface{}) string {
	var sb strings.Builder

	// Header
	sb.WriteString("┌────────────────────────────────────────────────────────┐\n")
	if location, ok := data["location"].(string); ok {
		fmt.Fprintf(&sb, "│  %-52s│\n", location)
	}
	sb.WriteString("├────────────────────────────────────────────────────────┤\n")

	// Data
	if temp, ok := data["temperature"].(float64); ok {
		fmt.Fprintf(&sb, "│  Temperature:   %-38s│\n", fmt.Sprintf("%.1f°F", temp))
	}
	if feels, ok := data["feels_like"].(float64); ok {
		fmt.Fprintf(&sb, "│  Feels Like:    %-38s│\n", fmt.Sprintf("%.1f°F", feels))
	}
	if cond, ok := data["condition"].(string); ok {
		fmt.Fprintf(&sb, "│  Condition:     %-38s│\n", cond)
	}
	if humid, ok := data["humidity"].(float64); ok {
		fmt.Fprintf(&sb, "│  Humidity:      %-38s│\n", fmt.Sprintf("%.0f%%", humid))
	}
	if wind, ok := data["wind_speed"].(float64); ok {
		fmt.Fprintf(&sb, "│  Wind Speed:    %-38s│\n", fmt.Sprintf("%.1f mph", wind))
	}

	sb.WriteString("└────────────────────────────────────────────────────────┘\n")

	return sb.String()
}

// formatPlainForecast formats forecast as plain text
func (f *Formatter) formatPlainForecast(data map[string]interface{}) string {
	var sb strings.Builder

	if location, ok := data["location"].(string); ok {
		fmt.Fprintf(&sb, "Location: %s\n\n", location)
	}

	if forecast, ok := data["forecast"].([]interface{}); ok {
		for i, day := range forecast {
			dayData := day.(map[string]interface{})
			if date, ok := dayData["date"].(string); ok {
				fmt.Fprintf(&sb, "Day %d - %s\n", i+1, date)
			}
			if high, ok := dayData["high"].(float64); ok {
				fmt.Fprintf(&sb, "  High: %.1f°F\n", high)
			}
			if low, ok := dayData["low"].(float64); ok {
				fmt.Fprintf(&sb, "  Low: %.1f°F\n", low)
			}
			if cond, ok := dayData["condition"].(string); ok {
				fmt.Fprintf(&sb, "  Condition: %s\n", cond)
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// formatTableForecast formats forecast as a table
func (f *Formatter) formatTableForecast(data map[string]interface{}) string {
	var sb strings.Builder

	if location, ok := data["location"].(string); ok {
		fmt.Fprintf(&sb, "Forecast for: %s\n\n", location)
	}

	sb.WriteString("┌──────────────┬────────────┬───────────┬──────────────────────┐\n")
	sb.WriteString("│     Date     │    High    │    Low    │      Condition       │\n")
	sb.WriteString("├──────────────┼────────────┼───────────┼──────────────────────┤\n")

	if forecast, ok := data["forecast"].([]interface{}); ok {
		for _, day := range forecast {
			dayData := day.(map[string]interface{})
			date := getStringValue(dayData, "date", "N/A")
			high := getFloatValue(dayData, "high", 0)
			low := getFloatValue(dayData, "low", 0)
			condition := getStringValue(dayData, "condition", "N/A")

			fmt.Fprintf(&sb, "│  %-10s  │  %6.1f°F  │  %5.1f°F  │  %-18s  │\n",
				date, high, low, condition)
		}
	}

	sb.WriteString("└──────────────┴────────────┴───────────┴──────────────────────┘\n")

	return sb.String()
}

// formatPlainAlerts formats alerts as plain text
func (f *Formatter) formatPlainAlerts(data map[string]interface{}) string {
	var sb strings.Builder

	if alerts, ok := data["alerts"].([]interface{}); ok {
		if len(alerts) == 0 {
			return "No active weather alerts.\n"
		}

		for i, alert := range alerts {
			alertData := alert.(map[string]interface{})
			fmt.Fprintf(&sb, "Alert %d:\n", i+1)
			if event, ok := alertData["event"].(string); ok {
				fmt.Fprintf(&sb, "  Event: %s\n", event)
			}
			if severity, ok := alertData["severity"].(string); ok {
				fmt.Fprintf(&sb, "  Severity: %s\n", severity)
			}
			if desc, ok := alertData["description"].(string); ok {
				fmt.Fprintf(&sb, "  Description: %s\n", desc)
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// formatTableAlerts formats alerts as a table
func (f *Formatter) formatTableAlerts(data map[string]interface{}) string {
	var sb strings.Builder

	if alerts, ok := data["alerts"].([]interface{}); ok {
		if len(alerts) == 0 {
			return "No active weather alerts.\n"
		}

		sb.WriteString("Active Weather Alerts:\n\n")

		for i, alert := range alerts {
			alertData := alert.(map[string]interface{})
			fmt.Fprintf(&sb, "┌─ Alert %d ──────────────────────────────────────────────────┐\n", i+1)

			if event, ok := alertData["event"].(string); ok {
				fmt.Fprintf(&sb, "│  Event: %-48s│\n", event)
			}
			if severity, ok := alertData["severity"].(string); ok {
				fmt.Fprintf(&sb, "│  Severity: %-44s│\n", severity)
			}
			if desc, ok := alertData["description"].(string); ok {
				// Wrap long descriptions
				lines := wrapText(desc, 50)
				for _, line := range lines {
					fmt.Fprintf(&sb, "│  %-52s│\n", line)
				}
			}

			sb.WriteString("└────────────────────────────────────────────────────────────┘\n\n")
		}
	}

	return sb.String()
}

// formatPlainMoon formats moon phase as plain text
func (f *Formatter) formatPlainMoon(data map[string]interface{}) string {
	var sb strings.Builder

	if phase, ok := data["phase"].(string); ok {
		fmt.Fprintf(&sb, "Moon Phase: %s\n", phase)
	}
	if illumination, ok := data["illumination"].(float64); ok {
		fmt.Fprintf(&sb, "Illumination: %.1f%%\n", illumination*100)
	}
	if age, ok := data["age"].(float64); ok {
		fmt.Fprintf(&sb, "Age: %.1f days\n", age)
	}

	return sb.String()
}

// formatTableMoon formats moon phase as a table
func (f *Formatter) formatTableMoon(data map[string]interface{}) string {
	var sb strings.Builder

	sb.WriteString("┌──────────────── Moon Phase ────────────────────┐\n")

	if phase, ok := data["phase"].(string); ok {
		fmt.Fprintf(&sb, "│  Phase:         %-28s│\n", phase)
	}
	if illumination, ok := data["illumination"].(float64); ok {
		fmt.Fprintf(&sb, "│  Illumination:  %-26.1f%%│\n", illumination*100)
	}
	if age, ok := data["age"].(float64); ok {
		fmt.Fprintf(&sb, "│  Age:           %-23.1f days│\n", age)
	}

	sb.WriteString("└────────────────────────────────────────────────┘\n")

	return sb.String()
}

// Helper functions

func getStringValue(data map[string]interface{}, key string, defaultValue string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return defaultValue
}

func getFloatValue(data map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := data[key].(float64); ok {
		return val
	}
	return defaultValue
}

func wrapText(text string, width int) []string {
	words := strings.Fields(text)
	var lines []string
	var currentLine string

	for _, word := range words {
		if len(currentLine)+len(word)+1 <= width {
			if currentLine != "" {
				currentLine += " "
			}
			currentLine += word
		} else {
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}
