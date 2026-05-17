package scheduler

import (
	"fmt"
	"log"
	"time"

	"github.com/casapps/wthr/src/server/service"
)

// DataSourceRefresher holds references to all data sources that need periodic updates
type DataSourceRefresher struct {
	locationEnhancer *service.LocationEnhancer
	zipcodeService   *service.ZipcodeService
	airportService   *service.AirportService
	geoipService     *service.GeoIPService
}

// NewDataSourceRefresher creates a new data source refresher
func NewDataSourceRefresher(
	locationEnhancer *service.LocationEnhancer,
	zipcodeService *service.ZipcodeService,
	airportService *service.AirportService,
	geoipService *service.GeoIPService,
) *DataSourceRefresher {
	return &DataSourceRefresher{
		locationEnhancer: locationEnhancer,
		zipcodeService:   zipcodeService,
		airportService:   airportService,
		geoipService:     geoipService,
	}
}

// RefreshAllDataSources refreshes all location data sources
func (dsr *DataSourceRefresher) RefreshAllDataSources() error {
	log.Println("🔄 Starting data source refresh...")
	startTime := time.Now()

	errors := make([]error, 0)

	// Refresh countries and cities (location enhancer)
	if err := dsr.locationEnhancer.Reload(); err != nil {
		log.Printf("❌ Failed to refresh countries/cities: %v", err)
		errors = append(errors, fmt.Errorf("countries/cities: %w", err))
	}

	// Refresh zipcodes
	if err := dsr.zipcodeService.Reload(); err != nil {
		log.Printf("❌ Failed to refresh zipcodes: %v", err)
		errors = append(errors, fmt.Errorf("zipcodes: %w", err))
	}

	// Refresh airports
	if err := dsr.airportService.Reload(); err != nil {
		log.Printf("❌ Failed to refresh airports: %v", err)
		errors = append(errors, fmt.Errorf("airports: %w", err))
	}

	// Refresh GeoIP database
	if err := dsr.geoipService.Reload(); err != nil {
		log.Printf("❌ Failed to refresh GeoIP: %v", err)
		errors = append(errors, fmt.Errorf("geoip: %w", err))
	}

	elapsed := time.Since(startTime)

	if len(errors) > 0 {
		log.Printf("⚠️  Data source refresh completed with %d error(s) in %.2f seconds", len(errors), elapsed.Seconds())
		return fmt.Errorf("refresh completed with errors: %v", errors)
	}

	log.Printf("✅ All data sources refreshed successfully in %.2f seconds", elapsed.Seconds())
	return nil
}

// CalculateNextRunTime calculates the next occurrence of the specified time (HH:MM format)
func CalculateNextRunTime(targetTime string) time.Duration {
	now := time.Now()

	// Parse target time (format: "HH:MM")
	var hour, minute int
	fmt.Sscanf(targetTime, "%d:%d", &hour, &minute)

	// Create target time for today
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())

	// If target time has already passed today, schedule for tomorrow
	if target.Before(now) {
		target = target.Add(24 * time.Hour)
	}

	duration := target.Sub(now)
	return duration
}

// ScheduleDataSourceRefresh schedules data source refresh at specific time daily
func (s *Scheduler) ScheduleDataSourceRefresh(refresher *DataSourceRefresher, targetTime string) {
	// Calculate initial delay until target time
	initialDelay := CalculateNextRunTime(targetTime)

	log.Printf("📅 Data source refresh scheduled for %s (next run in %v)", targetTime, initialDelay)

	// Start a goroutine that waits for the initial delay, then runs every 24 hours
	go func() {
		// Wait until target time
		time.Sleep(initialDelay)

		// Run the first refresh
		if err := refresher.RefreshAllDataSources(); err != nil {
			log.Printf("⚠️  Scheduled data refresh failed: %v", err)
		}

		// Then run every 24 hours
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := refresher.RefreshAllDataSources(); err != nil {
					log.Printf("⚠️  Scheduled data refresh failed: %v", err)
				}
			}
		}
	}()
}
