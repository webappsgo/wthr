package renderer

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/util"
)

// ASCIIRenderer handles ASCII art weather display with terminal formatting
type ASCIIRenderer struct {
	width    int
	noColors bool
}

// NewASCIIRenderer creates a new ASCII renderer
func NewASCIIRenderer() *ASCIIRenderer {
	return &ASCIIRenderer{
		width: 120,
	}
}

// RenderFull renders the full weather display with ASCII art
func (r *ASCIIRenderer) RenderFull(weather *util.WeatherData, params util.RenderParams) string {
	// Set noColors flag for this render
	r.noColors = params.NoColors

	var lines []string

	// Header (skip if quiet mode)
	if !params.Quiet {
		lines = append(lines, r.renderHeader(weather.Location))
		lines = append(lines, "")
	}

	// Current weather with ASCII art
	lines = append(lines, r.renderCurrentWeatherArt(weather.Current, params)...)

	// Add forecast based on format
	if params.Days > 0 {
		lines = append(lines, "")
		// Extra spacing before forecast table
		lines = append(lines, "")

		// If Width is specified, let renderForecastTable handle adaptive sizing
		// Otherwise, respect params.Days
		forecastDays := weather.Forecast
		if params.Width == 0 && len(forecastDays) > params.Days {
			// Only limit by params.Days if width is not specified
			forecastDays = forecastDays[:params.Days]
		}
		lines = append(lines, r.renderForecastTable(forecastDays, params)...)
	} else if params.Days == -1 {
		// Default: show full forecast
		lines = append(lines, "")
		// Extra spacing before forecast table
		lines = append(lines, "")
		lines = append(lines, r.renderForecastTable(weather.Forecast, params)...)
	}

	if !params.NoFooter {
		lines = append(lines, "")
		lines = append(lines, r.renderFooter(weather.Location))
	}

	// Add two newlines at the end for proper terminal spacing
	return strings.Join(lines, "\n") + "\n\n"
}

// renderHeader renders the weather report header
func (r *ASCIIRenderer) renderHeader(location util.LocationData) string {
	fullLocation := r.getFullLocationName(location)
	header := fmt.Sprintf("Weather report: %s", r.capitalizeLocation(fullLocation))
	// Yellow, bold
	return r.colorize(header, "#f1fa8c", true)
}

// capitalizeLocation capitalizes first letter of each location part
func (r *ASCIIRenderer) capitalizeLocation(location string) string {
	// Split by comma to get individual parts
	parts := strings.Split(location, ",")
	for i, part := range parts {
		part = strings.TrimSpace(part)

		// If it's a 2-letter country/state code, capitalize entirely
		if len(part) == 2 && strings.ToUpper(part) == part {
			parts[i] = strings.ToUpper(part)
		} else {
			// Capitalize first letter of each word
			words := strings.Fields(part)
			for j, word := range words {
				if len(word) > 0 {
					// Check if it's a short abbreviation (2-3 letters all caps)
					if len(word) <= 3 && strings.ToUpper(word) == word {
						words[j] = strings.ToUpper(word)
					} else {
						words[j] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
					}
				}
			}
			parts[i] = strings.Join(words, " ")
		}
	}
	return strings.Join(parts, ", ")
}

// getFullLocationName gets the full location display name with coordinates
func (r *ASCIIRenderer) getFullLocationName(location util.LocationData) string {
	var baseName string

	// Use the pre-built full name from geocoding API if available
	if location.FullName != "" {
		baseName = location.FullName
	} else if location.ShortName != "" {
		baseName = location.ShortName
	} else {
		// Fallback: build basic name
		baseName = fmt.Sprintf("%s, %s", location.Name, location.Country)
	}

	// Append coordinates for reference
	return fmt.Sprintf("%s [%.7f,%.7f]", baseName, location.Latitude, location.Longitude)
}

// renderCurrentWeatherArt renders the current weather with ASCII art
func (r *ASCIIRenderer) renderCurrentWeatherArt(current util.CurrentData, params util.RenderParams) []string {
	temp := int(math.Round(current.Temperature))
	feelsLike := int(math.Round(current.FeelsLike))
	tempUnit := getTemperatureUnit(params.Units)
	speedUnit := getSpeedUnit(params.Units)
	precipUnit := getPrecipitationUnit(params.Units)
	windSpeed := int(math.Round(current.WindSpeed))
	description := current.Condition
	precipitation := fmt.Sprintf("%.1f", current.Precipitation)

	art := r.getColoredWeatherArt(current.WeatherCode, params.NoColors)
	windDir := getWindArrow(current.WindDirection)

	// Determine if it's day or night based on the icon (if available)
	// isDay variable for future use
	_ = !strings.Contains(current.Icon, "🌙")

	// Create current weather display with Dracula colors
	lines := []string{
		fmt.Sprintf("%s     %s", art[0], r.colorize(description, "#8be9fd", false)),
		fmt.Sprintf("%s     %s", art[1], r.colorize(fmt.Sprintf("+%d(%d) %s", temp, feelsLike, tempUnit), "#f1fa8c", false)),
		fmt.Sprintf("%s     %s", art[2], r.colorize(fmt.Sprintf("%s %d %s", windDir, windSpeed, speedUnit), "#50fa7b", false)),
		fmt.Sprintf("%s     %s", art[3], r.colorize(fmt.Sprintf("%d %s", getPressure(current.Pressure, params.Units), getPressureUnit(params.Units)), "#bd93f9", false)),
		fmt.Sprintf("%s     %s", art[4], r.colorize(fmt.Sprintf("%s %s", precipitation, precipUnit), "#ff79c6", false)),
	}

	return lines
}

// renderForecastTable renders the forecast table with adaptive columns
func (r *ASCIIRenderer) renderForecastTable(forecast []util.ForecastData, params util.RenderParams) []string {
	var lines []string

	if len(forecast) == 0 {
		return []string{"No forecast data available"}
	}

	// Adaptive day count based on terminal width
	// Also respect params.Days as an upper bound
	numDays := r.calculateDaysToShow(params.Width, len(forecast))
	if params.Days > 0 && params.Days < numDays {
		// User explicitly limited days
		numDays = params.Days
	}
	days := forecast
	if len(days) > numDays {
		days = days[:numDays]
	}

	// Column width per time period (Morning, Noon, Evening, Night)
	// For 120 cols: 4 periods × 30 cols = 120 total
	colWidth := 30
	periodsPerDay := 4

	// Render each day as a separate table
	for dayIndex, day := range days {
		// Parse date for header
		date, _ := time.Parse("2006-01-02", day.Date)
		dayHeader := date.Format("Mon 2 Jan")

		// Day header (centered)
		headerWidth := periodsPerDay*colWidth + periodsPerDay + 1
		padding := (headerWidth - len(dayHeader) - 4) / 2
		lines = append(lines, strings.Repeat(" ", padding)+r.colorize("┌─"+dayHeader+"─┐", "#bd93f9", false))

		// Top border
		border := r.colorize("┌", "#bd93f9", false)
		for p := 0; p < periodsPerDay; p++ {
			border += r.colorize(strings.Repeat("─", colWidth), "#bd93f9", false)
			if p < periodsPerDay-1 {
				border += r.colorize("┬", "#bd93f9", false)
			}
		}
		border += r.colorize("┐", "#bd93f9", false)
		lines = append(lines, border)

		// Time period headers
		periodNames := []string{"Morning", "Noon", "Evening", "Night"}
		headerLine := r.colorize("│", "#bd93f9", false)
		for _, name := range periodNames {
			headerLine += centerInWidth(r.colorize(name, "#ffb86c", false), colWidth)
			headerLine += r.colorize("│", "#bd93f9", false)
		}
		lines = append(lines, headerLine)

		// Header separator
		sep := r.colorize("├", "#bd93f9", false)
		for p := 0; p < periodsPerDay; p++ {
			sep += r.colorize(strings.Repeat("─", colWidth), "#bd93f9", false)
			if p < periodsPerDay-1 {
				sep += r.colorize("┼", "#bd93f9", false)
			}
		}
		sep += r.colorize("┤", "#bd93f9", false)
		lines = append(lines, sep)

		// Generate period data
		periods := r.generateDayPeriods(day, params)
		allPeriods := [][]string{periods.morning, periods.noon, periods.evening, periods.night}

		// Render data rows (all periods have same height)
		numLines := len(periods.morning)
		for lineIdx := 0; lineIdx < numLines; lineIdx++ {
			dataLine := r.colorize("│", "#bd93f9", false)
			for _, period := range allPeriods {
				dataLine += padToWidth(period[lineIdx], colWidth)
				dataLine += r.colorize("│", "#bd93f9", false)
			}
			lines = append(lines, dataLine)
		}

		// Bottom border
		bottom := r.colorize("└", "#bd93f9", false)
		for p := 0; p < periodsPerDay; p++ {
			bottom += r.colorize(strings.Repeat("─", colWidth), "#bd93f9", false)
			if p < periodsPerDay-1 {
				bottom += r.colorize("┴", "#bd93f9", false)
			}
		}
		bottom += r.colorize("┘", "#bd93f9", false)
		lines = append(lines, bottom)

		// Add blank line between days (except last)
		if dayIndex < len(days)-1 {
			lines = append(lines, "")
		}
	}

	return lines
}

// calculateDaysToShow determines how many days to show based on terminal width
func (r *ASCIIRenderer) calculateDaysToShow(termWidth, availableDays int) int {
	// Default to 3 days if width is not specified or sufficient
	// Each day needs: 4 periods × 30 cols + 5 borders = 125 cols
	if termWidth == 0 || termWidth >= 125 {
		// Wide enough for default 3 days
		if availableDays >= 3 {
			return 3
		}
		return availableDays
	}

	// Narrow terminals: show fewer days
	if termWidth < 80 {
		// Very narrow: 1 day only
		return 1
	}

	// Medium width: 2 days
	if availableDays >= 2 {
		return 2
	}
	return availableDays
}

// generateDayPeriods generates forecast data for different periods of the day
// IMPORTANT: All periods MUST return exactly the same number of lines for proper alignment
func (r *ASCIIRenderer) generateDayPeriods(day util.ForecastData, params util.RenderParams) struct {
	morning []string
	noon    []string
	evening []string
	night   []string
} {
	tempUnit := getTemperatureUnit(params.Units)
	speedUnit := getSpeedUnit(params.Units)
	precipUnit := getPrecipitationUnit(params.Units)

	// Generate temperatures for different periods
	tempLow := int(math.Round(day.TempMin))
	tempHigh := int(math.Round(day.TempMax))
	tempMorn := int(math.Round(float64(tempLow) + float64(tempHigh-tempLow)*0.3))
	tempNoon := tempHigh
	tempEven := int(math.Round(float64(tempLow) + float64(tempHigh-tempLow)*0.7))
	tempNight := tempLow

	windSpeed := int(math.Round(day.WindSpeed))
	windDir := getWindArrow(day.WindDirection)
	precipitation := fmt.Sprintf("%.1f", day.Precipitation)
	// Default if not available
	precipProb := 0

	description := day.Condition

	// Create properly aligned content - EXACTLY 7 lines per period for perfect alignment
	createPeriodLines := func(temp, feels int, windMod float64) []string {
		tempLine := fmt.Sprintf("+%d(%d) %s", temp, feels, tempUnit)
		windLine := fmt.Sprintf("%s %d %s", windDir, int(math.Max(1, float64(windSpeed)*windMod)), speedUnit)
		precipLine := fmt.Sprintf("%s %s | %d%%", precipitation, precipUnit, precipProb)

		// EXACTLY 7 lines - NEVER change this count or alignment will break!
		return []string{
			// Line 1: blank for spacing
			centerInWidth("", 30),
			// Line 2: condition
			centerInWidth(r.colorize(description, "#8be9fd", false), 30),
			// Line 3: temperature
			centerInWidth(r.colorize(tempLine, "#f1fa8c", false), 30),
			// Line 4: wind
			centerInWidth(r.colorize(windLine, "#50fa7b", false), 30),
			// Line 5: visibility
			centerInWidth(r.colorize("6 mi", "#bd93f9", false), 30),
			// Line 6: precipitation
			centerInWidth(r.colorize(precipLine, "#ff79c6", false), 30),
			// Line 7: blank for spacing
			centerInWidth("", 30),
		}
	}

	return struct {
		morning []string
		noon    []string
		evening []string
		night   []string
	}{
		morning: createPeriodLines(tempMorn, tempMorn+2, 0.8),
		noon:    createPeriodLines(tempNoon, tempNoon+3, 1.0),
		evening: createPeriodLines(tempEven, tempEven+2, 1.2),
		night:   createPeriodLines(tempNight, tempNight+1, 0.6),
	}
}

// getColoredWeatherArt returns colored weather ASCII art
func (r *ASCIIRenderer) getColoredWeatherArt(weatherCode int, noColors bool) []string {
	// isDay is determined from icon
	baseArt := r.getWeatherArt(weatherCode, true)
	artColor := r.getWeatherColor(weatherCode, true)

	if noColors {
		return baseArt
	}

	colored := make([]string, len(baseArt))
	for i, line := range baseArt {
		colored[i] = r.colorize(line, artColor, false)
	}
	return colored
}

// getWeatherColor returns the Dracula theme color for a weather condition
func (r *ASCIIRenderer) getWeatherColor(weatherCode int, isDay bool) string {
	// Dracula theme colors for different weather conditions
	if weatherCode == 0 {
		if isDay {
			// Clear: yellow
			return "#f1fa8c"
		}
		// Clear night: purple
		return "#bd93f9"
	}
	if weatherCode <= 3 {
		// Cloudy: comment gray
		return "#6272a4"
	}
	if weatherCode >= 45 && weatherCode <= 48 {
		// Fog: foreground
		return "#f8f8f2"
	}
	if weatherCode >= 51 && weatherCode <= 67 {
		// Rain: cyan
		return "#8be9fd"
	}
	if weatherCode >= 71 && weatherCode <= 86 {
		// Snow: white
		return "#f8f8f2"
	}
	if weatherCode >= 95 {
		// Storms: red
		return "#ff5555"
	}

	// Default: foreground
	return "#f8f8f2"
}

// getWeatherArt returns ASCII art for a weather condition
func (r *ASCIIRenderer) getWeatherArt(weatherCode int, isDay bool) []string {
	// Clear sky
	if weatherCode == 0 {
		if isDay {
			return []string{
				"    \\   /    ",
				"     .-.     ",
				"  ― (   ) ―  ",
				"     `-'     ",
				"    /   \\    ",
			}
		}
		return []string{
			"             ",
			"     .--.    ",
			"  .-(    ).  ",
			" (___.__)_)  ",
			"             ",
		}
	}

	// Mainly clear
	if weatherCode == 1 {
		return []string{
			"   \\  /      ",
			" _ /\"\".-.    ",
			"   \\_(   ).  ",
			"   /(_(__)   ",
			"             ",
		}
	}

	// Partly cloudy / Overcast
	if weatherCode <= 3 {
		return []string{
			"             ",
			"     .--.    ",
			"  .-(    ).  ",
			" (___.__)_)  ",
			"             ",
		}
	}

	// Rain
	if weatherCode >= 61 && weatherCode <= 67 {
		return []string{
			"     .-.     ",
			"    (   ).   ",
			"   (___(__)  ",
			"    ‚'‚'‚'‚' ",
			"    ‚'‚'‚'‚' ",
		}
	}

	// Default to cloudy
	return []string{
		"             ",
		"     .--.    ",
		"  .-(    ).  ",
		" (___.__)_)  ",
		"             ",
	}
}

// renderFooter renders the footer with attribution
func (r *ASCIIRenderer) renderFooter(location util.LocationData) string {
	footer := r.colorize("Weather • Free weather data from Open-Meteo.com", "#6272a4", false)
	return footer
}

// Helper functions

// colorize applies ANSI color codes to text (method on renderer to access noColors)
func (r *ASCIIRenderer) colorize(text, hexColor string, bold bool) string {
	return colorizeWithFlag(text, hexColor, bold, r.noColors)
}

// colorizeWithFlag is a package-level function for rendering without renderer context
func colorizeWithFlag(text, hexColor string, bold bool, noColors bool) string {
	// If colors are disabled, return plain text
	if noColors {
		return text
	}

	// Convert hex to RGB
	red, green, blue := hexToRGB(hexColor)

	// ANSI escape code for 24-bit color
	colorCode := fmt.Sprintf("\033[38;2;%d;%d;%dm", red, green, blue)
	resetCode := "\033[0m"

	if bold {
		colorCode = "\033[1m" + colorCode
	}

	return colorCode + text + resetCode
}

// colorize is the original package function for backward compatibility
func colorize(text, hexColor string, bold bool) string {
	return colorizeWithFlag(text, hexColor, bold, false)
}

// hexToRGB converts hex color to RGB
func hexToRGB(hex string) (int, int, int) {
	// Remove # if present
	hex = strings.TrimPrefix(hex, "#")

	var r, g, b int
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

// stripAnsiCodes removes ANSI color codes from text
func stripAnsiCodes(text string) string {
	// Simple regex-like replacement for ANSI codes
	result := ""
	inEscape := false
	for _, ch := range text {
		if ch == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if ch == 'm' {
				inEscape = false
			}
			continue
		}
		result += string(ch)
	}
	return result
}

// padToWidth pads text to a specific width (accounting for ANSI codes)
func padToWidth(text string, width int) string {
	cleanText := stripAnsiCodes(text)
	actualLength := len(cleanText)
	padding := width - actualLength
	if padding < 0 {
		padding = 0
	}
	return text + strings.Repeat(" ", padding)
}

// centerInWidth centers text within a specific width (accounting for ANSI codes)
func centerInWidth(text string, width int) string {
	cleanText := stripAnsiCodes(text)
	textLength := len(cleanText)

	// If text is too long, truncate it
	if textLength > width {
		// Find the color codes in the original text
		hasColors := strings.Contains(text, "\x1b[")
		if hasColors {
			// Extract color prefix and reset code
			colorStart := text[:strings.Index(text, "m")+1]
			resetCode := "\x1b[0m"
			truncated := cleanText[:width]
			return colorStart + truncated + resetCode
		}
		return cleanText[:width]
	}

	totalPadding := width - textLength
	leftPadding := totalPadding / 2
	rightPadding := totalPadding - leftPadding

	return strings.Repeat(" ", leftPadding) + text + strings.Repeat(" ", rightPadding)
}

// getWindArrow returns wind direction arrow
func getWindArrow(degrees int) string {
	arrows := []string{"↓", "↙", "←", "↖", "↑", "↗", "→", "↘"}
	index := int(math.Round(float64(degrees)/45.0)) % 8
	return arrows[index]
}

// Unit conversion helpers

func getTemperatureUnit(units string) string {
	if units == "metric" || units == "M" {
		return "°C"
	}
	return "°F"
}

func getSpeedUnit(units string) string {
	if units == "M" {
		return "m/s"
	}
	if units == "metric" {
		return "km/h"
	}
	return "mph"
}

func getPrecipitationUnit(units string) string {
	if units == "metric" || units == "M" {
		return "mm"
	}
	return "in"
}

func getPressureUnit(units string) string {
	if units == "imperial" {
		return "inHg"
	}
	return "hPa"
}

func getPressure(pressure float64, units string) int {
	if units == "imperial" {
		return int(math.Round(pressure * 0.02953))
	}
	return int(math.Round(pressure))
}
