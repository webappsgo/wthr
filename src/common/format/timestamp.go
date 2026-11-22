package format

import (
	"os"
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	defaultTimezone = "America/New_York"
	timestampLayout = "January 02, 2006 at 15:04:05 MST"
)

// Timestamp formats a user-visible timestamp in the configured server timezone.
func Timestamp(value time.Time) string {
	zone := strings.TrimSpace(os.Getenv("TZ"))
	if zone == "" {
		zone = defaultTimezone
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		location, _ = time.LoadLocation(defaultTimezone)
	}
	return value.In(location).Format(timestampLayout)
}
