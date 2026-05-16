package utils

import (
	"errors"
	"strconv"
	"strings"
)

// ParseCoordinates parses a coordinate string in format "lat,lon"
func ParseCoordinates(coords string) (float64, float64, error) {
	parts := strings.Split(coords, ",")
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid coordinate format, expected 'lat,lon'")
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, errors.New("invalid latitude")
	}

	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, errors.New("invalid longitude")
	}

	// Validate ranges
	if lat < -90 || lat > 90 {
		return 0, 0, errors.New("latitude must be between -90 and 90")
	}
	if lon < -180 || lon > 180 {
		return 0, 0, errors.New("longitude must be between -180 and 180")
	}

	return lat, lon, nil
}
