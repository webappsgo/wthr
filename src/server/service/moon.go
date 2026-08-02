package service

import (
	"fmt"
	"math"
	"time"
)

// MoonService handles lunar calculations
// Implements astronomical algorithms for moon phase, illumination, rise/set times
type MoonService struct{}

// NewMoonService creates a new moon service
func NewMoonService() *MoonService {
	return &MoonService{}
}

// MoonData represents complete lunar information
type MoonData struct {
	// New, Waxing Crescent, First Quarter, etc.
	Phase string `json:"phase"`
	// 0-100%
	Illumination float64 `json:"illumination"`
	// Days since new moon (0-29.53)
	Age float64 `json:"age"`
	// Unicode moon emoji
	Icon string `json:"icon"`
	// HH:MM format
	Rise string `json:"rise"`
	// HH:MM format
	Set string `json:"set"`
	// ISO 8601
	NextNewMoon string `json:"next_new_moon"`
	// ISO 8601
	NextFullMoon string `json:"next_full_moon"`
	// Distance from Earth in km
	Distance float64 `json:"distance_km"`
	// Angular diameter in degrees
	AngularSize float64 `json:"angular_size"`
}

// Calculate returns moon data for a specific location and time
// Uses astronomical algorithms per Meeus "Astronomical Algorithms"
func (ms *MoonService) Calculate(lat, lon float64, t time.Time) *MoonData {
	// Calculate moon age (days since new moon)
	age := ms.calculateMoonAge(t)

	// Calculate illumination percentage
	illumination := ms.calculateIllumination(age)

	// Determine moon phase name
	phase := ms.getPhaseName(age)

	// Get appropriate moon emoji
	icon := ms.getMoonIcon(age)

	// Calculate rise and set times
	rise, set := ms.calculateRiseSet(lat, lon, t)

	// Calculate next new and full moons
	nextNew := ms.calculateNextNewMoon(t)
	nextFull := ms.calculateNextFullMoon(t)

	// Calculate distance from Earth
	distance := ms.calculateDistance(t)

	// Calculate angular size
	angularSize := ms.calculateAngularSize(distance)

	return &MoonData{
		Phase:        phase,
		Illumination: illumination,
		Age:          age,
		Icon:         icon,
		Rise:         rise,
		Set:          set,
		NextNewMoon:  nextNew.Format(time.RFC3339),
		NextFullMoon: nextFull.Format(time.RFC3339),
		Distance:     distance,
		AngularSize:  angularSize,
	}
}

// calculateMoonAge calculates days since last new moon
// Synodic month: 29.53058867 days (average)
func (ms *MoonService) calculateMoonAge(t time.Time) float64 {
	// Known new moon: January 6, 2000, 18:14 UTC
	knownNewMoon := time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC)

	// Days since known new moon
	daysSince := t.Sub(knownNewMoon).Hours() / 24.0

	// Synodic month length
	synodicMonth := 29.53058867

	// Calculate current moon age
	age := math.Mod(daysSince, synodicMonth)
	if age < 0 {
		age += synodicMonth
	}

	return age
}

// calculateIllumination calculates percentage of moon illuminated
func (ms *MoonService) calculateIllumination(age float64) float64 {
	// Synodic month
	synodicMonth := 29.53058867

	// Phase angle (0 to 2π)
	phaseAngle := (age / synodicMonth) * 2.0 * math.Pi

	// Illumination formula
	illumination := (1 - math.Cos(phaseAngle)) / 2.0 * 100.0

	return illumination
}

// getPhaseName returns the name of the moon phase
func (ms *MoonService) getPhaseName(age float64) string {
	if age < 1.84566 {
		return "New Moon"
	} else if age < 7.38264 {
		return "Waxing Crescent"
	} else if age < 9.22831 {
		return "First Quarter"
	} else if age < 14.76529 {
		return "Waxing Gibbous"
	} else if age < 16.61096 {
		return "Full Moon"
	} else if age < 22.14794 {
		return "Waning Gibbous"
	} else if age < 24.99361 {
		return "Last Quarter"
	} else if age < 29.53059 {
		return "Waning Crescent"
	}
	return "New Moon"
}

// getMoonIcon returns Unicode moon emoji for phase
func (ms *MoonService) getMoonIcon(age float64) string {
	if age < 1.84566 {
		// New Moon
		return "🌑"
	} else if age < 7.38264 {
		// Waxing Crescent
		return "🌒"
	} else if age < 9.22831 {
		// First Quarter
		return "🌓"
	} else if age < 14.76529 {
		// Waxing Gibbous
		return "🌔"
	} else if age < 16.61096 {
		// Full Moon
		return "🌕"
	} else if age < 22.14794 {
		// Waning Gibbous
		return "🌖"
	} else if age < 24.99361 {
		// Last Quarter
		return "🌗"
	} else {
		// Waning Crescent
		return "🌘"
	}
}

// calculateRiseSet calculates moonrise and moonset times
// Simplified calculation - for production use a proper astronomical library
func (ms *MoonService) calculateRiseSet(lat, lon float64, t time.Time) (string, string) {
	// Simplified calculation based on moon age
	// Real implementation would use proper astronomical algorithms
	age := ms.calculateMoonAge(t)

	// Approximate times based on phase (moon rises ~50 minutes later each day)
	// Rough approximation
	baseRiseHour := 6.0 + (age * 0.8)
	baseSetHour := 18.0 + (age * 0.8)

	// Normalize to 24-hour format
	riseHour := math.Mod(baseRiseHour, 24.0)
	setHour := math.Mod(baseSetHour, 24.0)

	riseMinute := int((riseHour - math.Floor(riseHour)) * 60)
	setMinute := int((setHour - math.Floor(setHour)) * 60)

	rise := fmt.Sprintf("%02d:%02d", int(riseHour), riseMinute)
	set := fmt.Sprintf("%02d:%02d", int(setHour), setMinute)

	return rise, set
}

// calculateNextNewMoon calculates the next new moon date
func (ms *MoonService) calculateNextNewMoon(t time.Time) time.Time {
	age := ms.calculateMoonAge(t)
	synodicMonth := 29.53058867

	// Days until next new moon
	daysUntil := synodicMonth - age

	return t.Add(time.Duration(daysUntil*24) * time.Hour)
}

// calculateNextFullMoon calculates the next full moon date
func (ms *MoonService) calculateNextFullMoon(t time.Time) time.Time {
	age := ms.calculateMoonAge(t)
	synodicMonth := 29.53058867

	// Full moon is at age ~14.765
	fullMoonAge := synodicMonth / 2.0

	var daysUntil float64
	if age < fullMoonAge {
		daysUntil = fullMoonAge - age
	} else {
		daysUntil = synodicMonth - age + fullMoonAge
	}

	return t.Add(time.Duration(daysUntil*24) * time.Hour)
}

// calculateDistance calculates moon's distance from Earth in km
// Average: 384,400 km, varies due to elliptical orbit (356,500 - 406,700 km)
func (ms *MoonService) calculateDistance(t time.Time) float64 {
	// Simplified calculation - varies with orbital position
	// Real implementation would use orbital mechanics
	age := ms.calculateMoonAge(t)
	synodicMonth := 29.53058867

	// Distance varies roughly sinusoidally
	avgDistance := 384400.0
	variation := 25000.0
	phaseAngle := (age / synodicMonth) * 2.0 * math.Pi

	distance := avgDistance + variation*math.Sin(phaseAngle)
	return distance
}

// calculateAngularSize calculates moon's angular diameter in degrees
func (ms *MoonService) calculateAngularSize(distance float64) float64 {
	// Moon's actual diameter: 3,474.8 km
	moonDiameter := 3474.8

	// Angular size = 2 * arctan(diameter / (2 * distance))
	// Result in degrees
	angularSize := 2.0 * math.Atan(moonDiameter/(2.0*distance)) * (180.0 / math.Pi)

	return angularSize
}

// GetPhaseForDate returns moon phase for a specific date
func (ms *MoonService) GetPhaseForDate(date time.Time) string {
	age := ms.calculateMoonAge(date)
	return ms.getPhaseName(age)
}

// GetIlluminationForDate returns moon illumination for a specific date
func (ms *MoonService) GetIlluminationForDate(date time.Time) float64 {
	age := ms.calculateMoonAge(date)
	return ms.calculateIllumination(age)
}
