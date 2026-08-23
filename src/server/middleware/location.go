package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// SaveLocationCookies saves user location to persistent cookies
func SaveLocationCookies(w http.ResponseWriter, r *http.Request, latitude, longitude float64, locationName string) {
	// Cookie settings: 30 days expiration, accessible to all paths
	// 30 days in seconds
	maxAge := 30 * 24 * 60 * 60

	http.SetCookie(w, &http.Cookie{Name: "user_lat", Value: fmt.Sprintf("%.6f", latitude), MaxAge: maxAge, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "user_lon", Value: fmt.Sprintf("%.6f", longitude), MaxAge: maxAge, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "user_location_name", Value: locationName, MaxAge: maxAge, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "user_location_set_at", Value: time.Now().Format(time.RFC3339), MaxAge: maxAge, Path: "/"})
}

// GetLocationFromCookies retrieves user location from cookies
func GetLocationFromCookies(r *http.Request) (latitude, longitude float64, locationName string, found bool) {
	latCookie, err1 := r.Cookie("user_lat")
	lonCookie, err2 := r.Cookie("user_lon")
	nameCookie, err3 := r.Cookie("user_location_name")

	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, "", false
	}

	lat, err := strconv.ParseFloat(latCookie.Value, 64)
	if err != nil {
		return 0, 0, "", false
	}

	lon, err := strconv.ParseFloat(lonCookie.Value, 64)
	if err != nil {
		return 0, 0, "", false
	}

	return lat, lon, nameCookie.Value, true
}

// ClearLocationCookies removes location cookies
func ClearLocationCookies(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "user_lat", Value: "", MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "user_lon", Value: "", MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "user_location_name", Value: "", MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "user_location_set_at", Value: "", MaxAge: -1, Path: "/"})
}
