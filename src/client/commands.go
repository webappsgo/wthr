package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// handleCurrentCommand handles the current weather command
func handleCurrentCommand(config *CLIConfig, args []string) error {
	flagSet := flag.NewFlagSet("current", flag.ContinueOnError)
	lat := flagSet.Float64("lat", 0, "Latitude")
	lon := flagSet.Float64("lon", 0, "Longitude")
	zip := flagSet.String("zip", "", "ZIP code")
	location := flagSet.String("location", "", "Location name")

	if err := flagSet.Parse(args); err != nil {
		return NewUsageError(err.Error())
	}

	// Build query parameters
	apiPath := config.GetAPIPath()
	path := buildWeatherPath(apiPath, "/weather", *lat, *lon, *zip, *location, config)
	if path == "" {
		return NewUsageError("must specify --lat/--lon, --zip, or --location (or set MYLOCATION_NAME, MYLOCATION_ZIP, or MYLOCATION_CITY_ID)")
	}

	// Make API request
	client := NewHTTPClient(config)
	var result map[string]interface{}
	if err := client.GetJSON(path, &result); err != nil {
		return err
	}

	// Format and print output
	formatter := NewFormatter(config.Output.Format, config.Output.Color == "no")
	fmt.Println(formatter.FormatWeatherCurrent(result))

	return nil
}

// handleForecastCommand handles the forecast command
func handleForecastCommand(config *CLIConfig, args []string) error {
	flagSet := flag.NewFlagSet("forecast", flag.ContinueOnError)
	lat := flagSet.Float64("lat", 0, "Latitude")
	lon := flagSet.Float64("lon", 0, "Longitude")
	zip := flagSet.String("zip", "", "ZIP code")
	location := flagSet.String("location", "", "Location name")
	days := flagSet.Int("days", 7, "Number of days (default: 7)")

	if err := flagSet.Parse(args); err != nil {
		return NewUsageError(err.Error())
	}

	// Build query parameters
	// Use /forecasts endpoint (alias for /weather/forecast)
	apiPath := config.GetAPIPath()
	path := buildWeatherPath(apiPath, "/forecasts", *lat, *lon, *zip, *location, config)
	if path == "" {
		return NewUsageError("must specify --lat/--lon, --zip, or --location (or set MYLOCATION_NAME, MYLOCATION_ZIP, or MYLOCATION_CITY_ID)")
	}
	path += fmt.Sprintf("&days=%d", *days)

	// Make API request
	client := NewHTTPClient(config)
	var result map[string]interface{}
	if err := client.GetJSON(path, &result); err != nil {
		return err
	}

	// Format and print output
	formatter := NewFormatter(config.Output.Format, config.Output.Color == "no")
	fmt.Println(formatter.FormatForecast(result))

	return nil
}

// handleAlertsCommand handles the alerts command
func handleAlertsCommand(config *CLIConfig, args []string) error {
	flagSet := flag.NewFlagSet("alerts", flag.ContinueOnError)
	lat := flagSet.Float64("lat", 0, "Latitude")
	lon := flagSet.Float64("lon", 0, "Longitude")
	zip := flagSet.String("zip", "", "ZIP code")
	location := flagSet.String("location", "", "Location name")

	if err := flagSet.Parse(args); err != nil {
		return NewUsageError(err.Error())
	}

	// Build query parameters
	apiPath := config.GetAPIPath()
	path := buildWeatherPath(apiPath, "/weather/alerts", *lat, *lon, *zip, *location, config)
	if path == "" {
		return NewUsageError("must specify --lat/--lon, --zip, or --location (or set MYLOCATION_NAME, MYLOCATION_ZIP, or MYLOCATION_CITY_ID)")
	}

	// Make API request
	client := NewHTTPClient(config)
	var result map[string]interface{}
	if err := client.GetJSON(path, &result); err != nil {
		return err
	}

	// Format and print output
	formatter := NewFormatter(config.Output.Format, config.Output.Color == "no")
	fmt.Println(formatter.FormatAlerts(result))

	return nil
}

// handleMoonCommand handles the moon phase command
func handleMoonCommand(config *CLIConfig, args []string) error {
	flagSet := flag.NewFlagSet("moon", flag.ContinueOnError)
	date := flagSet.String("date", "", "Date (YYYY-MM-DD, default: today)")

	if err := flagSet.Parse(args); err != nil {
		return NewUsageError(err.Error())
	}

	// Build query parameters
	apiPath := config.GetAPIPath()
	path := fmt.Sprintf("%s/weather/moon", apiPath)
	if *date != "" {
		path = fmt.Sprintf("%s?date=%s", path, *date)
	}

	// Make API request
	client := NewHTTPClient(config)
	var result map[string]interface{}
	if err := client.GetJSON(path, &result); err != nil {
		return err
	}

	// Format and print output
	formatter := NewFormatter(config.Output.Format, config.Output.Color == "no")
	fmt.Println(formatter.FormatMoon(result))

	return nil
}

// handleHistoryCommand handles the historical weather command
func handleHistoryCommand(config *CLIConfig, args []string) error {
	flagSet := flag.NewFlagSet("history", flag.ContinueOnError)
	lat := flagSet.Float64("lat", 0, "Latitude")
	lon := flagSet.Float64("lon", 0, "Longitude")
	zip := flagSet.String("zip", "", "ZIP code")
	location := flagSet.String("location", "", "Location name")
	date := flagSet.String("date", "", "Date (YYYY-MM-DD)")

	if err := flagSet.Parse(args); err != nil {
		return NewUsageError(err.Error())
	}

	if *date == "" {
		return NewUsageError("--date is required for history command")
	}

	// Build query parameters
	// Use /weather/history endpoint
	apiPath := config.GetAPIPath()
	path := buildWeatherPath(apiPath, "/weather/history", *lat, *lon, *zip, *location, config)
	if path == "" {
		return NewUsageError("must specify --lat/--lon, --zip, or --location (or set MYLOCATION_NAME, MYLOCATION_ZIP, or MYLOCATION_CITY_ID)")
	}
	path += fmt.Sprintf("&date=%s", url.QueryEscape(*date))

	// Make API request
	client := NewHTTPClient(config)
	var result map[string]interface{}
	if err := client.GetJSON(path, &result); err != nil {
		return err
	}

	// Format and print output (reuse current weather formatter for historical data)
	formatter := NewFormatter(config.Output.Format, config.Output.Color == "no")
	fmt.Println(formatter.FormatWeatherCurrent(result))

	return nil
}

// buildWeatherPath builds a URL path with proper URL encoding
// Priority: lat/lon > zip > location > city_id (from env) > location (from config/env)
func buildWeatherPath(apiPath, endpoint string, lat, lon float64, zip, location string, config *CLIConfig) string {
	if lat != 0 && lon != 0 {
		return fmt.Sprintf("%s%s?lat=%f&lon=%f", apiPath, endpoint, lat, lon)
	}
	if zip != "" {
		return fmt.Sprintf("%s%s?zip=%s", apiPath, endpoint, url.QueryEscape(zip))
	}
	if location != "" {
		return fmt.Sprintf("%s%s?location=%s", apiPath, endpoint, url.QueryEscape(location))
	}

	// Check for default location from config/env vars
	// Priority per AI.md: config.Location > MYLOCATION_NAME > MYLOCATION_ZIP > MYLOCATION_CITY_ID
	if config.Location != "" {
		return fmt.Sprintf("%s%s?location=%s", apiPath, endpoint, url.QueryEscape(config.Location))
	}

	// Check MYLOCATION_NAME env var
	if loc := getEnv("MYLOCATION_NAME"); loc != "" {
		return fmt.Sprintf("%s%s?location=%s", apiPath, endpoint, url.QueryEscape(loc))
	}

	// Check MYLOCATION_ZIP env var
	if zipCode := getEnv("MYLOCATION_ZIP"); zipCode != "" {
		return fmt.Sprintf("%s%s?zip=%s", apiPath, endpoint, url.QueryEscape(zipCode))
	}

	// Check MYLOCATION_CITY_ID env var - use city_id parameter
	if cityID := getEnv("MYLOCATION_CITY_ID"); cityID != "" {
		// Validate it's a number
		if _, err := strconv.Atoi(cityID); err == nil {
			return fmt.Sprintf("%s%s?city_id=%s", apiPath, endpoint, cityID)
		}
	}

	return ""
}

// getEnv is a helper to get environment variables
// Wraps os.Getenv for easier testing
func getEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
