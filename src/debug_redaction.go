package main

import (
	"encoding/json"
	"expvar"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func redactedPPROFCmdline(w http.ResponseWriter, _ *http.Request) {
	if _, err := io.WriteString(w, debugBinaryName()+"\x00"); err != nil {
		log.Printf("WARN: Could not write redacted pprof command line: %v", err)
	}
}

func redactedExpvarHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		variables := make(map[string]json.RawMessage)
		expvar.Do(func(kv expvar.KeyValue) {
			if kv.Key == "cmdline" {
				value, err := json.Marshal([]string{debugBinaryName()})
				if err != nil {
					log.Printf("WARN: Could not encode redacted expvar command line: %v", err)
					return
				}
				variables[kv.Key] = value
				return
			}

			value := json.RawMessage(kv.Value.String())
			if !json.Valid(value) {
				encodedValue, err := json.Marshal(kv.Value.String())
				if err != nil {
					log.Printf("WARN: Could not encode expvar %q: %v", kv.Key, err)
					return
				}
				value = encodedValue
			}
			variables[kv.Key] = value
		})

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(variables); err != nil {
			http.Error(w, "could not encode runtime variables", http.StatusInternalServerError)
		}
	})
}

func debugBinaryName() string {
	if len(os.Args) == 0 || os.Args[0] == "" {
		return "wthr"
	}
	return filepath.Base(os.Args[0])
}
