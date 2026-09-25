package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/webappsgo/wthr/src/config"
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

// DecodeAndValidate decodes the request body into dst and enforces its
// `binding:"..."` struct tags via validator/v10. AI.md PART 16 requires the
// admin panel and every frontend route to work with JavaScript disabled, so
// `application/x-www-form-urlencoded` bodies are decoded from the parsed form
// using the structs' `form:"..."` tags instead of being rejected. On decode
// failure it writes a canonical BAD_REQUEST response; on constraint failure
// it writes a canonical VALIDATION_FAILED response naming the first failing
// field and rule. Returns true only when decode succeeded and every
// constraint passed; callers should `return` immediately on false.
func DecodeAndValidate(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		if err := decodeFormBody(r, dst); err != nil {
			RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "errors.invalid_request"))
			return false
		}
	} else if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "errors.invalid_request"))
		return false
	}

	if err := reqValidator.Struct(dst); err != nil {
		verrs, ok := err.(validator.ValidationErrors)
		if !ok || len(verrs) == 0 {
			RespondError(w, r, http.StatusBadRequest, ErrBadRequest, Translate(r, "errors.invalid_request"))
			return false
		}
		fe := verrs[0]
		field := jsonFieldName(fe)
		ValidationFailed(w, r, fmt.Sprintf("%s: %s", Translate(r, "errors.validation_failed"), field), map[string]interface{}{
			"field": field,
			"rule":  fe.Tag(),
		})
		return false
	}

	return true
}

// formFieldName resolves the submitted form field name for a struct field.
// AI.md PART 16 form posts use the same names as the JSON API, so a field
// without an explicit `form:` tag falls back to its `json:` tag and finally
// to its own lowercased name.
func formFieldName(field reflect.StructField) string {
	if tag := field.Tag.Get("form"); tag != "" {
		return strings.Split(tag, ",")[0]
	}
	if tag := field.Tag.Get("json"); tag != "" {
		return strings.Split(tag, ",")[0]
	}
	return strings.ToLower(field.Name)
}

// decodeFormBody copies an already-parsed form into dst using reflection.
// Strings, numbers, booleans, pointers, slices, maps, and nested structs are
// all supported so the form-encoded path is a peer of the JSON path rather
// than a string-only subset. Booleans always go through config.ParseBool, per
// AI.md PART 5, never strconv.ParseBool.
func decodeFormBody(r *http.Request, dst interface{}) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Ptr || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("decode destination must be a non-nil struct pointer")
	}

	elem := rv.Elem()
	formType := elem.Type()
	for i := 0; i < elem.NumField(); i++ {
		field := formType.Field(i)
		if !elem.Field(i).CanSet() || field.PkgPath != "" {
			continue
		}
		name := formFieldName(field)
		if name == "" || name == "-" {
			continue
		}
		if err := setFormField(r.Form, name, elem.Field(i)); err != nil {
			return err
		}
	}

	return nil
}

// setFormField assigns one named form value (or a slice/map of them) to the
// destination field, converting it to the field's concrete type.
func setFormField(values url.Values, name string, dest reflect.Value) error {
	// A slice field collects every repeated submission, so `altNames=a&altNames=b` fills a two-item slice.
	if dest.Kind() == reflect.Slice && dest.Type().Elem().Kind() != reflect.Uint8 {
		raw, ok := values[name]
		if !ok {
			return nil
		}
		slice := reflect.MakeSlice(dest.Type(), 0, len(raw))
		for _, item := range raw {
			itemDest := reflect.New(dest.Type().Elem()).Elem()
			if err := setFormField(url.Values{name: {item}}, name, itemDest); err != nil {
				return err
			}
			slice = reflect.Append(slice, itemDest)
		}
		dest.Set(slice)
		return nil
	}

	// A map field collects `name.key=value` submissions so structured config
	// reaches the handler without a JSON body.
	if dest.Kind() == reflect.Map {
		prefix := name + "."
		entries := make(map[string]string)
		for key, list := range values {
			if !strings.HasPrefix(key, prefix) || len(list) == 0 {
				continue
			}
			entries[strings.TrimPrefix(key, prefix)] = list[0]
		}
		if len(entries) == 0 {
			return nil
		}
		if dest.IsNil() {
			dest.Set(reflect.MakeMap(dest.Type()))
		}
		for key, value := range entries {
			entryDest := reflect.New(dest.Type().Elem()).Elem()
			if err := assignFormScalar(value, entryDest); err != nil {
				return err
			}
			dest.SetMapIndex(reflect.ValueOf(key), entryDest)
		}
		return nil
	}

	// A nested struct recurses, preferring its own `name.field` entries and
	// falling back to the flat set so a form needs no dotted paths to reach an
	// inner field when the inner names are unambiguous.
	if dest.Kind() == reflect.Struct {
		return setFormStruct(formScoped(values, name), dest)
	}

	// An unchecked checkbox submits nothing at all, which still means false.
	if dest.Kind() == reflect.Bool {
		raw, ok := values[name]
		if !ok || len(raw) == 0 {
			dest.SetBool(false)
			return nil
		}
		return assignFormScalar(raw[0], dest)
	}

	// A pointer field is only allocated when the form actually carried it, or
	// when it wraps a struct/map that any of its own sub-field supplied.
	if dest.Kind() == reflect.Ptr {
		if !formCarries(values, name) {
			return nil
		}
		ptr := reflect.New(dest.Type().Elem())
		if err := setFormField(values, name, ptr.Elem()); err != nil {
			return err
		}
		dest.Set(ptr)
		return nil
	}

	raw, ok := values[name]
	if !ok || len(raw) == 0 {
		return nil
	}
	return assignFormScalar(raw[0], dest)
}

// formCarries reports whether values holds anything a pointer field should
// decode from: the field's own key, or any of its sub-field keys.
func formCarries(values url.Values, name string) bool {
	prefix := name + "."
	for key := range values {
		if key == name {
			return true
		}
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// formScoped returns the entries belonging to a nested field: its own
// `name.field` keys with the prefix stripped, plus every flat key the nested
// struct's own fields resolve to. The flat pass is skipped when the form used
// dotted keys, so a prefixed entry never collides with a sibling of the same
// name at the parent level.
func formScoped(values url.Values, name string) url.Values {
	scoped := url.Values{}
	prefix := name + "."
	usedDots := false
	for key, list := range values {
		if strings.HasPrefix(key, prefix) {
			scoped[strings.TrimPrefix(key, prefix)] = list
			usedDots = true
		}
	}
	if usedDots {
		return scoped
	}
	for key, list := range values {
		if !strings.Contains(key, ".") {
			scoped[key] = list
		}
	}
	return scoped
}

// setFormStruct binds the fields of a nested destination struct from the value
// set its parent scoped for it.
func setFormStruct(values url.Values, dest reflect.Value) error {
	formType := dest.Type()
	for i := 0; i < dest.NumField(); i++ {
		field := formType.Field(i)
		if !dest.Field(i).CanSet() || field.PkgPath != "" {
			continue
		}
		name := formFieldName(field)
		if name == "" || name == "-" {
			continue
		}
		if err := setFormField(values, name, dest.Field(i)); err != nil {
			return err
		}
	}

	return nil
}

// assignFormScalar converts a single submitted string into dest, which may be
// a string, bool, any numeric kind, or a string-slice/map/struct destination
// already unwrapped by setFormField.
func assignFormScalar(raw string, dest reflect.Value) error {
	switch dest.Kind() {
	case reflect.String:
		dest.SetString(raw)
	case reflect.Bool:
		// AI.md PART 5 forbids strconv.ParseBool for booleans.
		parsed, err := config.ParseBool(raw, false)
		if err != nil {
			return err
		}
		dest.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, dest.Type().Bits())
		if err != nil {
			return err
		}
		dest.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		parsed, err := strconv.ParseUint(strings.TrimSpace(raw), 10, dest.Type().Bits())
		if err != nil {
			return err
		}
		dest.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), dest.Type().Bits())
		if err != nil {
			return err
		}
		dest.SetFloat(parsed)
	case reflect.Interface:
		dest.Set(reflect.ValueOf(raw))
	case reflect.Slice, reflect.Map, reflect.Struct, reflect.Ptr:
		return setFormField(url.Values{"value": {raw}}, "value", dest)
	default:
		return fmt.Errorf("unsupported form field kind %s", dest.Kind())
	}
	return nil
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
