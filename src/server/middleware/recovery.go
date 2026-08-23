package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/webappsgo/wthr/src/mode"
)

// panicLogger is the minimal logging surface Recovery needs. util.Logger
// satisfies this; a narrow interface keeps this file testable without a
// real logger and avoids importing util's full surface into the signature.
type panicLogger interface {
	Error(format string, v ...interface{})
}

// Recovery returns middleware that recovers from a panic in any downstream
// handler, logs it, and returns a canonical INTERNAL_ERROR response instead
// of crashing the server or leaking a stack trace to the client. Equivalent
// to gin.Recovery(), converted to the chi func(http.Handler) http.Handler
// shape. Per AI.md PART 11, stack traces are never sent in the response body
// (log only); the response stays plain-text/JSON per Accept header, mirroring
// handler.RespondError's canonical shape without importing the handler
// package (would create an import cycle — see admin_auth.go's RenderHTML doc
// comment for the same constraint).
func Recovery(logger panicLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					stack := debug.Stack()
					if logger != nil {
						logger.Error("panic recovered: %v\nrequest: %s %s\n%s", rec, r.Method, r.URL.Path, stack)
					}
					respondPanic(w, r, rec)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// respondPanic writes the canonical error response after a recovered panic.
// Text/plain (per .txt extension or Accept header) gets a short line; every
// other request gets the canonical JSON error shape from AI.md PART 14.
// Debug mode adds a "_debug" field with the panic value only — never the
// stack trace, which stays log-only per PART 11's Tier 3 rules.
func respondPanic(w http.ResponseWriter, r *http.Request, rec interface{}) {
	const message = "An internal error occurred"

	if isTextRequest(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("INTERNAL_ERROR: " + message + "\n"))
		return
	}

	body := map[string]interface{}{
		"ok":      false,
		"error":   "INTERNAL_ERROR",
		"message": message,
	}
	if mode.IsDebugEnabled() {
		body["_debug"] = map[string]interface{}{"panic": recToString(rec)}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(body)
}

// isTextRequest is a minimal, local reimplementation of handler.shouldRespondText
// (path .txt suffix or Accept: text/plain) — duplicated rather than imported
// to avoid the middleware->handler import cycle.
func isTextRequest(r *http.Request) bool {
	if strings.HasSuffix(r.URL.Path, ".txt") {
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "text/plain")
}

// recToString renders a recovered panic value as a string for the optional
// debug-only response field.
func recToString(rec interface{}) string {
	if err, ok := rec.(error); ok {
		return err.Error()
	}
	if s, ok := rec.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", rec)
}
