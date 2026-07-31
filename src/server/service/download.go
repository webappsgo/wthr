package service

import (
	"net/http"
	"time"
)

// downloadClient is the shared HTTP client for fetching bulk data files
// (GeoIP databases up to ~103MB, airport CSV, zipcode JSON) from
// server-controlled URLs.
//
// It intentionally sets ResponseHeaderTimeout / TLS / dial timeouts rather
// than a single Client.Timeout: a stalled upstream that never sends response
// headers is aborted quickly, while a slow-but-progressing large body download
// is not killed mid-transfer by a hard overall deadline.
var downloadClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
	},
}
