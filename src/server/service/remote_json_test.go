package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDecodeRemoteJSON(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		resp := &http.Response{Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}
		var decoded struct {
			OK bool `json:"ok"`
		}
		if err := decodeRemoteJSON(resp, &decoded); err != nil {
			t.Fatalf("decodeRemoteJSON() error = %v", err)
		}
		if !decoded.OK {
			t.Fatal("decoded JSON did not preserve the true value")
		}
	})

	t.Run("rejects advertised oversized response", func(t *testing.T) {
		resp := &http.Response{
			Body:          io.NopCloser(strings.NewReader(`{}`)),
			ContentLength: maxRemoteJSONBytes + 1,
		}
		if err := decodeRemoteJSON(resp, &struct{}{}); err == nil {
			t.Fatal("decodeRemoteJSON() accepted an oversized response")
		}
	})

	t.Run("rejects streamed oversized response", func(t *testing.T) {
		resp := &http.Response{
			Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", maxRemoteJSONBytes+1))),
		}
		if err := decodeRemoteJSON(resp, &struct{}{}); err == nil {
			t.Fatal("decodeRemoteJSON() accepted a streamed oversized response")
		}
	})
}
