package cluster

import (
	"bytes"
	"testing"
)

func TestJoinTokenFormatAndUniqueness(t *testing.T) {
	first, err := newJoinToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newJoinToken()
	if err != nil {
		t.Fatal(err)
	}
	if !joinTokenPattern.MatchString(first) {
		t.Fatalf("generated token has invalid format")
	}
	if first == second {
		t.Fatal("generated tokens must be unique")
	}
	if hashJoinToken(first) == first {
		t.Fatal("token storage value must be a hash")
	}
}

func TestJoinPayloadEncryptionAuthenticatesToken(t *testing.T) {
	plain := []byte(`{"database":{"password":"private"}}`)
	encrypted, err := encryptJoinPayload(plain, "node_0123456789abcdefghijklmnopqrstuv")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decryptJoinPayload(encrypted, "node_0123456789abcdefghijklmnopqrstuv")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, plain) {
		t.Fatal("decrypted bootstrap differs from original")
	}
	if _, err := decryptJoinPayload(encrypted, "node_1123456789abcdefghijklmnopqrstuv"); err == nil {
		t.Fatal("bootstrap decrypted with a different token")
	}
}

func TestJoinPayloadRejectsMalformedCiphertext(t *testing.T) {
	if _, err := decryptJoinPayload("not-base64!", "node_0123456789abcdefghijklmnopqrstuv"); err == nil {
		t.Fatal("malformed ciphertext was accepted")
	}
}
