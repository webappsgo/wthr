package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
)

// reqValidator is the shared validator/v10 instance used to enforce request
// struct constraints. AI.md PART 3 approves go-playground/validator/v10 for
// input validation. SetTagName reuses the "binding" tag key already present
// on every request struct converted from gin's ShouldBindJSON (which used
// validator/v10 internally under the same tag name), so no struct
// definitions need to change to gain enforcement back.
var reqValidator = newRequestValidator()

func newRequestValidator() *validator.Validate {
	v := validator.New()
	v.SetTagName("binding")
	return v
}

// DecodeAndValidate decodes the JSON request body into dst and enforces its
// `binding:"..."` struct tags via validator/v10. On decode failure it writes
// a canonical BAD_REQUEST response; on constraint failure it writes a
// canonical VALIDATION_FAILED response naming the first failing field and
// rule. Returns true only when decode succeeded and every constraint
// passed; callers should `return` immediately on false.
func DecodeAndValidate(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		RespondError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid request format")
		return false
	}

	if err := reqValidator.Struct(dst); err != nil {
		verrs, ok := err.(validator.ValidationErrors)
		if !ok || len(verrs) == 0 {
			RespondError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid request format")
			return false
		}
		fe := verrs[0]
		field := jsonFieldName(fe)
		ValidationFailed(w, r, fmt.Sprintf("Validation failed: %s", field), map[string]interface{}{
			"field": field,
			"rule":  fe.Tag(),
		})
		return false
	}

	return true
}

// jsonFieldName lowercases and snake-cases a validator field name so error
// details match the request's own json tag convention (e.g. "Identifier"
// from a struct field reports as "identifier").
func jsonFieldName(fe validator.FieldError) string {
	name := fe.Field()
	var b strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}
