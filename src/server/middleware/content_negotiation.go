package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

// contentNegotiationWriter buffers the handler's response so the Content-Type
// can be rewritten after the handler returns, which is the only point at
// which the body is known to actually be JSON.
type contentNegotiationWriter struct {
	http.ResponseWriter
	status      int
	buf         bytes.Buffer
	intercepted bool
	wroteHeader bool
	committed   bool
}

func (c *contentNegotiationWriter) WriteHeader(status int) {
	if c.wroteHeader {
		return
	}
	c.wroteHeader = true
	c.status = status
}

func (c *contentNegotiationWriter) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	// Only buffer while the response is still a candidate for conversion.
	if c.status == http.StatusOK {
		c.intercepted = true
	}
	return c.buf.Write(b)
}

// Flush lets streaming handlers opt out of buffering. Once flushed the body has
// begun reaching the client, so the Content-Type can no longer be rewritten.
func (c *contentNegotiationWriter) Flush() {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	c.commit()
}

// Unwrap exposes the underlying writer so http.ResponseController (and the
// optional interfaces it looks for) still works through this wrapper.
func (c *contentNegotiationWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// commit writes the buffered response out, converting a JSON body to
// text/plain when the client asked for it.
func (c *contentNegotiationWriter) commit() {
	if c.committed {
		return
	}
	c.committed = true
	if !c.intercepted {
		if !c.wroteHeader {
			c.status = http.StatusOK
		}
		c.ResponseWriter.WriteHeader(c.status)
		_, _ = c.ResponseWriter.Write(c.buf.Bytes())
		return
	}

	ct := c.ResponseWriter.Header().Get("Content-Type")
	// Only JSON is convertible; anything else (HTML, already-plain text, empty)
	// passes through untouched.
	if !strings.Contains(ct, "application/json") {
		c.ResponseWriter.WriteHeader(c.status)
		_, _ = c.ResponseWriter.Write(c.buf.Bytes())
		return
	}

	var v interface{}
	if err := json.Unmarshal(c.buf.Bytes(), &v); err != nil {
		// Not decodable after all - emit the original bytes verbatim.
		c.ResponseWriter.WriteHeader(c.status)
		_, _ = c.ResponseWriter.Write(c.buf.Bytes())
		return
	}

	// AI.md PART 14 "Backend API Content Negotiation": /api/** returns raw
	// data as plain text (not HTML2Text output) for Accept: text/plain and for
	// .txt extension requests.
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		c.ResponseWriter.WriteHeader(c.status)
		_, _ = c.ResponseWriter.Write(c.buf.Bytes())
		return
	}
	out = append(out, '\n')

	c.ResponseWriter.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.ResponseWriter.WriteHeader(c.status)
	_, _ = c.ResponseWriter.Write(out)
}

// wantsPlainText reports whether the client requested plain text, via either
// the Accept header or a .txt extension on the path.
func wantsPlainText(r *http.Request) bool {
	if strings.Contains(r.Header.Get("Accept"), "text/plain") {
		return true
	}
	return strings.HasSuffix(r.URL.Path, ".txt")
}

// ContentNegotiation converts JSON responses on API routes into
// text/plain when the client asks for plain text.
//
// AI.md PART 14 requires /api/** to honor "Accept: text/plain" and a .txt
// extension, but the handlers behind it (and their shared writeJSON helper)
// have no access to the request, so they always emit application/json. Rather
// than thread the request through all ~330 call sites, this middleware
// buffers successful JSON responses and re-serializes them as plain text.
//
// It only acts on /api/ paths: frontend routes keep their own negotiation
// (AI.md's response table pins /server/{admin_path}/* to HTML).
func ContentNegotiation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || !wantsPlainText(r) {
			next.ServeHTTP(w, r)
			return
		}

		cw := &contentNegotiationWriter{ResponseWriter: w}
		defer cw.commit()
		next.ServeHTTP(cw, r)
	})
}
