package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type formNestedPayload struct {
	Default string            `json:"default"`
	Bots    map[string]string `json:"bots"`
}

type formPayload struct {
	Name     string            `json:"name" binding:"required"`
	Count    int               `json:"count"`
	Ratio    float64           `json:"ratio"`
	Active   bool              `json:"active"`
	AltNames []string          `json:"alt_names"`
	Config   map[string]string `json:"config"`
	Nested   *formNestedPayload `json:"nested"`
	Optional *string           `json:"optional"`
	Ignored  string            `json:"-"`
	private  string
}

func formRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/ssl", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func TestDecodeAndValidateFormBindsEveryFieldKind(t *testing.T) {
	rec := httptest.NewRecorder()
	dst := &formPayload{}

	body := "name=example&count=42&ratio=1.5&active=yes" +
		"&alt_names=a.example&alt_names=b.example" +
		"&config.mode=strict&config.retries=not-a-number" +
		"&nested.default=helper&nested.bots.iris=IRIS.md" +
		"&optional=set&ignored=nope&private=hidden"
	if !DecodeAndValidate(rec, formRequest(body), dst) {
		t.Fatalf("expected decode to succeed, got %d %s", rec.Code, rec.Body.String())
	}

	if dst.Name != "example" {
		t.Errorf("Name = %q, want example", dst.Name)
	}
	if dst.Count != 42 {
		t.Errorf("Count = %d, want 42", dst.Count)
	}
	if dst.Ratio != 1.5 {
		t.Errorf("Ratio = %v, want 1.5", dst.Ratio)
	}
	if !dst.Active {
		t.Error("Active = false, want true for the truthy value \"yes\"")
	}
	if len(dst.AltNames) != 2 || dst.AltNames[0] != "a.example" || dst.AltNames[1] != "b.example" {
		t.Errorf("AltNames = %#v, want both repeated submissions", dst.AltNames)
	}
	if dst.Config["mode"] != "strict" {
		t.Errorf("Config = %#v, want mode=strict", dst.Config)
	}
	if dst.Nested == nil || dst.Nested.Default != "helper" {
		t.Fatalf("Nested = %#v, want a pointer with default=helper", dst.Nested)
	}
	if dst.Nested.Bots["iris"] != "IRIS.md" {
		t.Errorf("Nested.Bots = %#v, want iris=IRIS.md", dst.Nested.Bots)
	}
	if dst.Optional == nil || *dst.Optional != "set" {
		t.Errorf("Optional = %#v, want a pointer to \"set\"", dst.Optional)
	}
	if dst.Ignored != "" {
		t.Errorf("Ignored = %q, want the json:\"-\" field left alone", dst.Ignored)
	}
	if dst.private != "" {
		t.Errorf("private = %q, want unexported fields left alone", dst.private)
	}
}

func TestDecodeAndValidateFormUncheckedCheckboxIsFalse(t *testing.T) {
	rec := httptest.NewRecorder()
	dst := &formPayload{Active: true}

	if !DecodeAndValidate(rec, formRequest("name=example"), dst) {
		t.Fatalf("expected decode to succeed, got %d %s", rec.Code, rec.Body.String())
	}
	if dst.Active {
		t.Error("Active = true, want false when the checkbox submitted nothing")
	}
	if dst.Optional != nil {
		t.Error("Optional must stay nil when the form did not carry the key")
	}
	if dst.Nested != nil {
		t.Error("Nested must stay nil when the form carried no sub-field")
	}
}

func TestDecodeAndValidateFormInvalidBoolIsBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()

	if DecodeAndValidate(rec, formRequest("name=example&active=perhaps"), &formPayload{}) {
		t.Fatal("expected an unparsable boolean to be rejected")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDecodeAndValidateFormInvalidNumberIsBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()

	if DecodeAndValidate(rec, formRequest("name=example&count=lots"), &formPayload{}) {
		t.Fatal("expected an unparsable integer to be rejected")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDecodeAndValidateFormRunsBindingTags(t *testing.T) {
	rec := httptest.NewRecorder()

	if DecodeAndValidate(rec, formRequest("name="), &formPayload{}) {
		t.Fatal("expected the required binding tag to reject a blank name")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDecodeAndValidateRejectsNonStructDestination(t *testing.T) {
	rec := httptest.NewRecorder()
	target := ""

	if DecodeAndValidate(rec, formRequest("name=example"), &target) {
		t.Fatal("expected a non-struct destination to be rejected")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDecodeAndValidateJSONPathUnchanged(t *testing.T) {
	rec := httptest.NewRecorder()
	dst := &formPayload{}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"name":"json","count":7,"active":true}`))
	r.Header.Set("Content-Type", "application/json")

	if !DecodeAndValidate(rec, r, dst) {
		t.Fatalf("expected decode to succeed, got %d %s", rec.Code, rec.Body.String())
	}
	if dst.Name != "json" || dst.Count != 7 || !dst.Active {
		t.Errorf("unexpected decode result %#v", dst)
	}
}

func TestDecodeAndValidateJSONMalformedIsBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader("{not json"))
	r.Header.Set("Content-Type", "application/json")

	if DecodeAndValidate(rec, r, &formPayload{}) {
		t.Fatal("expected malformed JSON to be rejected")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDecodeAndValidateJSONMissingRequiredIsRejected(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"count":7}`))
	r.Header.Set("Content-Type", "application/json")

	if DecodeAndValidate(rec, r, &formPayload{}) {
		t.Fatal("expected the required binding tag to reject a missing name")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestFormFieldNamePrefersFormTag(t *testing.T) {
	dst := struct {
		Value string `form:"chosen" json:"value"`
	}{}
	rec := httptest.NewRecorder()

	if !DecodeAndValidate(rec, formRequest("chosen=from-form-tag"), &dst) {
		t.Fatalf("expected decode to succeed, got %d %s", rec.Code, rec.Body.String())
	}
	if dst.Value != "from-form-tag" {
		t.Errorf("Value = %q, want the form-tagged name to win", dst.Value)
	}
}
