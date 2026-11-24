package util

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestSetupProofVerification(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	key := base64.StdEncoding.EncodeToString([]byte("setup proof test key material"))
	proof := SignSetupProof(key, now)
	if !strings.HasPrefix(proof, "1767323045.") {
		t.Fatalf("signed proof = %q, want fixed Unix timestamp prefix", proof)
	}
	if !VerifySetupProof(key, proof, now) {
		t.Fatal("valid proof did not verify")
	}
	if !VerifySetupProof(key, SignSetupProof(key, now.Add(-setupProofTTL+time.Second)), now) {
		t.Fatal("proof within the one-hour lifetime did not verify")
	}

	badSignature := proof[:strings.LastIndex(proof, ".")+1] + "invalid"
	wrongKey := base64.StdEncoding.EncodeToString([]byte("different setup proof key"))
	invalidTimestamp := "not-a-timestamp." + setupProofMAC(key, "not-a-timestamp")
	tooOld := SignSetupProof(key, now.Add(-setupProofTTL))
	future := SignSetupProof(key, now.Add(time.Second))
	tests := []struct {
		name, key, proof string
	}{
		{name: "invalid key encoding", key: "!", proof: proof},
		{name: "empty decoded key", key: "", proof: proof},
		{name: "missing separator", key: key, proof: "malformed"},
		{name: "empty signature", key: key, proof: strings.SplitN(proof, ".", 2)[0] + "."},
		{name: "wrong signature", key: key, proof: badSignature},
		{name: "wrong key", key: wrongKey, proof: proof},
		{name: "invalid timestamp", key: key, proof: invalidTimestamp},
		{name: "expired", key: key, proof: tooOld},
		{name: "future timestamp", key: key, proof: future},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if VerifySetupProof(test.key, test.proof, now) {
				t.Fatal("invalid proof verified")
			}
		})
	}
	if setupProofMAC("!", "issued") != "" {
		t.Fatal("setupProofMAC returned a signature for an invalid key")
	}
}
