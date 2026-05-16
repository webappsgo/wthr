package renderer

import (
	"fmt"
	"math"
	"strings"

	"github.com/apimgr/weather/src/util"
)

// OneLineRenderer handles one-line format weather displays
type OneLineRenderer struct{}

// NewOneLineRenderer creates a new one-line renderer
func NewOneLineRenderer() *OneLineRenderer {
	return &OneLineRenderer{}
}

// RenderOneLine renders a complete one-line weather display
func (r *OneLineRenderer) RenderOneLine(location utils.LocationData, current utils.CurrentData, units string) string {
	temp := int(math.Round(current.Temperature))
	description := current.Condition
	icon := current.Icon
	tempUnit := getTemperatureUnit(units)
	windSpeed := int(math.Round(current.WindSpeed))
	speedUnit := getSpeedUnit(units)
	windDir := r.getWindDirection(current.WindDirection)

	// One-line format for status bars: location temp condition wind
	oneLine := fmt.Sprintf("%s: %s %s %s %s %s",
		colorize(location.Name, "#8be9fd", false),
		colorize(fmt.Sprintf("%d%s", temp, tempUnit), "#f1fa8c", false),
		colorize(icon, "#50fa7b", false),
		colorize(description, "#bd93f9", false),
		colorize(fmt.Sprintf("%d%s %s", windSpeed, speedUnit, windDir), "#ff79c6", false),
		colorize(fmt.Sprintf("%d%%", current.Humidity), "#ffb86c", false))

	return oneLine + "\n\n"
}

// RenderFormat1 renders format 1: Just weather icon + temp: "🌦  +11°C"
func (r *OneLineRenderer) RenderFormat1(current utils.CurrentData, units string, noColors bool) string {
	temp := int(math.Round(current.Temperature))
	icon := current.Icon
	tempUnit := strings.Replace(getTemperatureUnit(units), "°", "", 1)

	var tempStr string
	if noColors {
		tempStr = fmt.Sprintf("+%d°%s", temp, tempUnit)
	} else {
		tempStr = colorize(fmt.Sprintf("+%d°%s", temp, tempUnit), "#f1fa8c", false)
	}

	return fmt.Sprintf("%s  %s\n", icon, tempStr)
}

// RenderFormat2 renders format 2: Weather + details: "🌦  🌡️ +11°C  🌬️ ↓ 4km/h"
func (r *OneLineRenderer) RenderFormat2(current utils.CurrentData, units string, noColors bool) string {
	temp := int(math.Round(current.Temperature))
	icon := current.Icon
	tempUnit := getTemperatureUnit(units)
	windSpeed := int(math.Round(current.WindSpeed))
	speedUnit := getSpeedUnit(units)
	windArrow := getWindArrow(current.WindDirection)

	var tempStr, windStr string
	if noColors {
		tempStr = fmt.Sprintf("+%d%s", temp, tempUnit)
		windStr = fmt.Sprintf("%s %d%s", windArrow, windSpeed, speedUnit)
	} else {
		tempStr = colorize(fmt.Sprintf("+%d%s", temp, tempUnit), "#f1fa8c", false)
		windStr = colorize(fmt.Sprintf("%s %d%s", windArrow, windSpeed, speedUnit), "#50fa7b", false)
	}

	return fmt.Sprintf("%s  🌡️ %s  🌬️ %s\n", icon, tempStr, windStr)
}

// RenderFormat3 renders format 3: Location + weather: "London, GB:  🌦  +11°C"
func (r *OneLineRenderer) RenderFormat3(location utils.LocationData, current utils.CurrentData, units string, noColors bool) string {
	temp := int(math.Round(current.Temperature))
	icon := current.Icon
	tempUnit := strings.Replace(getTemperatureUnit(units), "°", "", 1)
	locationName := r.getShortLocationName(location)

	var locationStr, tempStr string
	if noColors {
		locationStr = locationName
		tempStr = fmt.Sprintf("+%d°%s", temp, tempUnit)
	} else {
		locationStr = colorize(locationName, "#8be9fd", false)
		tempStr = colorize(fmt.Sprintf("+%d°%s", temp, tempUnit), "#f1fa8c", false)
	}

	return fmt.Sprintf("%s: %s  %s\n", locationStr, icon, tempStr)
}

// RenderFormat4 renders format 4: Location + weather + details: "London, GB:  🌦  🌡️+11°C  🌬️↓4km/h"
func (r *OneLineRenderer) RenderFormat4(location utils.LocationData, current utils.CurrentData, units string, noColors bool) string {
	temp := int(math.Round(current.Temperature))
	icon := current.Icon
	tempUnit := getTemperatureUnit(units)
	windSpeed := int(math.Round(current.WindSpeed))
	speedUnit := getSpeedUnit(units)
	windArrow := getWindArrow(current.WindDirection)
	locationName := r.getShortLocationName(location)

	var locationStr, tempStr, windStr string
	if noColors {
		locationStr = locationName
		tempStr = fmt.Sprintf("+%d%s", temp, tempUnit)
		windStr = fmt.Sprintf("%s%d%s", windArrow, windSpeed, speedUnit)
	} else {
		locationStr = colorize(locationName, "#8be9fd", false)
		tempStr = colorize(fmt.Sprintf("+%d%s", temp, tempUnit), "#f1fa8c", false)
		windStr = colorize(fmt.Sprintf("%s%d%s", windArrow, windSpeed, speedUnit), "#50fa7b", false)
	}

	return fmt.Sprintf("%s: %s  🌡️ %s  🌬️ %s\n", locationStr, icon, tempStr, windStr)
}

// getWindDirection returns wind direction abbreviation (N, NE, E, SE, etc.)
func (r *OneLineRenderer) getWindDirection(degrees int) string {
	directions := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	index := int(math.Round(float64(degrees)/22.5)) % 16
	return directions[index]
}

// getShortLocationName gets the short location name for display
func (r *OneLineRenderer) getShortLocationName(location utils.LocationData) string {
	// Use the pre-built short name from enhanced location data
	if location.ShortName != "" {
		return location.ShortName
	}

	// Fallback: build short name using available data
	cityName := location.Name
	countryCode := location.CountryCode
	if countryCode == "" {
		countryCode = "XX"
	}

	return fmt.Sprintf("%s, %s", cityName, countryCode)
}

// getFullLocationName gets the full location name for display
func (r *OneLineRenderer) getFullLocationName(location utils.LocationData) string {
	// Use the pre-built full name from enhanced location data
	if location.FullName != "" {
		return location.FullName
	}

	// Fallback: basic format
	return fmt.Sprintf("%s, %s", location.Name, location.Country)
}
