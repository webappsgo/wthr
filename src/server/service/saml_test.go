package service

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	secureSAML "github.com/supatype/saml"
)

func TestSAMLParseResponseRejectsNilInputs(t *testing.T) {
	service := &SAMLService{}
	if _, err := service.ParseResponse(nil, nil, nil, nil); err == nil {
		t.Fatal("ParseResponse accepted nil parser inputs")
	}
}

func TestParseSAMLResponseSafelyContainsParserPanic(t *testing.T) {
	assertion, err := parseSAMLResponseSafely(nil, nil, nil, time.Minute)
	if err == nil || assertion != nil {
		t.Fatalf("parseSAMLResponseSafely() = (%v, %v), want nil assertion and an error", assertion, err)
	}
}

func TestNormalizeSAMLAssertionMapsOnlyValidatedClaims(t *testing.T) {
	assertion := &secureSAML.Assertion{
		Signature: etree.NewElement("Signature"),
		Subject:   &secureSAML.Subject{NameID: &secureSAML.NameID{Format: "persistent", Value: "subject-1"}},
		AttributeStatements: []secureSAML.AttributeStatement{{Attributes: []secureSAML.Attribute{{
			Name:   "groups",
			Values: []secureSAML.AttributeValue{{Value: "admins"}, {Value: "operators"}},
		}}}},
		AuthnStatements: []secureSAML.AuthnStatement{{SessionIndex: "session-1"}},
	}
	got := normalizeSAMLAssertion(assertion)
	if got == nil || got.Subject == nil || got.Subject.NameID == nil {
		t.Fatal("normalized assertion is missing subject NameID")
	}
	if got.Subject.NameID.Value != "subject-1" || got.Subject.NameID.Format != "persistent" || !got.SignaturePresent {
		t.Fatalf("normalized identity = %#v, signature present = %v", got.Subject.NameID, got.SignaturePresent)
	}
	if len(got.AttributeStatements) != 1 || len(got.AttributeStatements[0].Attributes) != 1 {
		t.Fatalf("normalized attribute statements = %#v", got.AttributeStatements)
	}
	if values := got.AttributeStatements[0].Attributes[0].Values; len(values) != 2 || values[0] != "admins" || values[1] != "operators" {
		t.Fatalf("normalized attribute values = %#v", values)
	}
	if len(got.AuthnStatements) != 1 || got.AuthnStatements[0].SessionIndex != "session-1" {
		t.Fatalf("normalized authentication statements = %#v", got.AuthnStatements)
	}
}

func TestSecureSAMLAssertionParserRejectsForeignNameID(t *testing.T) {
	const input = `<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"><saml:Subject><x:NameID xmlns:x="urn:example:foreign">attacker@example.com</x:NameID></saml:Subject></saml:Assertion>`
	var assertion secureSAML.Assertion
	if err := xml.Unmarshal([]byte(input), &assertion); err != nil {
		t.Fatalf("unmarshal assertion: %v", err)
	}
	if assertion.Subject == nil {
		t.Fatal("subject was not parsed")
	}
	if assertion.Subject.NameID != nil {
		t.Fatalf("foreign-namespace NameID was accepted: %#v", assertion.Subject.NameID)
	}
}

func TestParseSAMLLogoutRequestIdentity(t *testing.T) {
	const valid = `<samlp:LogoutRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion" ID="request-1"><saml:NameID>user-1</saml:NameID></samlp:LogoutRequest>`
	const foreign = `<samlp:LogoutRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:x="urn:example:foreign" ID="request-1"><x:NameID>attacker</x:NameID></samlp:LogoutRequest>`
	t.Run("POST accepts a namespaced NameID", func(t *testing.T) {
		request := logoutRequestPOST(valid)
		identity, err := ParseSAMLLogoutRequestIdentity(request)
		if err != nil {
			t.Fatalf("ParseSAMLLogoutRequestIdentity() error = %v", err)
		}
		if identity.ID != "request-1" || identity.NameID != "user-1" {
			t.Fatalf("identity = %#v, want request-1/user-1", identity)
		}
	})
	t.Run("POST rejects a foreign namespace NameID", func(t *testing.T) {
		if _, err := ParseSAMLLogoutRequestIdentity(logoutRequestPOST(foreign)); err == nil {
			t.Fatal("ParseSAMLLogoutRequestIdentity() accepted a foreign namespace NameID")
		}
	})
	t.Run("Redirect accepts a namespaced NameID", func(t *testing.T) {
		var compressed bytes.Buffer
		writer, err := flate.NewWriter(&compressed, flate.DefaultCompression)
		if err != nil {
			t.Fatalf("create deflate writer: %v", err)
		}
		if _, err := writer.Write([]byte(valid)); err != nil {
			t.Fatalf("compress logout request: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close deflate writer: %v", err)
		}
		query := url.Values{"SAMLRequest": {base64.StdEncoding.EncodeToString(compressed.Bytes())}}
		request := httptest.NewRequest(http.MethodGet, "/slo?"+query.Encode(), nil)
		identity, err := ParseSAMLLogoutRequestIdentity(request)
		if err != nil {
			t.Fatalf("ParseSAMLLogoutRequestIdentity() error = %v", err)
		}
		if identity.ID != "request-1" || identity.NameID != "user-1" {
			t.Fatalf("identity = %#v, want request-1/user-1", identity)
		}
	})
}

func logoutRequestPOST(message string) *http.Request {
	form := url.Values{"SAMLRequest": {base64.StdEncoding.EncodeToString([]byte(message))}}
	request := httptest.NewRequest(http.MethodPost, "/slo", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return request
}

func TestValidSAMLProviderName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "okta", want: true},
		{name: "corp-idp_2", want: true},
		{name: "", want: false},
		{name: "Upper", want: false},
		{name: "2provider", want: false},
		{name: "provider.name", want: false},
		{name: "this-provider-name-is-far-too-long-for-the-provider-name-limit", want: false},
	}
	for _, test := range tests {
		if got := validSAMLProviderName(test.name); got != test.want {
			t.Errorf("validSAMLProviderName(%q) = %t, want %t", test.name, got, test.want)
		}
	}
}
