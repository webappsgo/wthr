package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const maxRemoteJSONBytes = 32 << 20

func decodeRemoteJSON(resp *http.Response, target any) error {
	if resp.ContentLength > maxRemoteJSONBytes {
		return fmt.Errorf("remote JSON response exceeds %d bytes", maxRemoteJSONBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteJSONBytes+1))
	if err != nil {
		return fmt.Errorf("read remote JSON response: %w", err)
	}
	if len(body) > maxRemoteJSONBytes {
		return fmt.Errorf("remote JSON response exceeds %d bytes", maxRemoteJSONBytes)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode remote JSON response: %w", err)
	}
	return nil
}
