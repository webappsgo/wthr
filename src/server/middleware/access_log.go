package middleware

import (
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

// AccessLogger creates middleware for logging HTTP requests
// TEMPLATE.md Part 25: Supports 7 log formats
func AccessLogger(logger *util.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			duration := time.Since(start)

			clientIP := util.GetClientIP(r)
			method := r.Method
			path := r.URL.Path
			protocol := r.Proto
			statusCode := ww.Status()
			bodySize := int64(ww.BytesWritten())
			referer := r.Referer()
			userAgent := r.UserAgent()

			username := ""
			if user, exists := reqctx.Get(r.Context(), UserContextKey); exists {
				if u, ok := user.(*model.User); ok && u != nil {
					username = u.Username
				}
			}

			logger.Access(clientIP, username, method, path, protocol, statusCode, bodySize, referer, userAgent)

			if duration > 1*time.Second {
				logger.Error("Slow request: %s %s took %v", method, path, duration)
			}
		})
	}
}

// AccessLoggerWithFormat creates middleware for logging HTTP requests with configurable format
// TEMPLATE.md Part 25: Support 7 log formats (apache, nginx, json, fail2ban, syslog, cef, text)
func AccessLoggerWithFormat(logger *util.Logger, formatter *service.LogFormatter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			entry := service.ExtractLogEntry(r, start, ww.Status(), ww.BytesWritten())

			if user, exists := reqctx.Get(r.Context(), UserContextKey); exists {
				if u, ok := user.(*model.User); ok && u != nil {
					entry.Username = u.Username
				}
			}

			logLine := formatter.Format(entry)
			logger.Write(logLine)

			if entry.RequestTime > 1.0 {
				logger.Error("Slow request: %s %s took %.3fs", entry.Method, entry.Path, entry.RequestTime)
			}
		})
	}
}
