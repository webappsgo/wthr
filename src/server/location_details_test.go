package server

import (
	"html/template"
	"strings"
	"testing"
)

// TestLocationDetailsRendersTranslatedLabels executes the shared partial with
// the map shape the weather handler builds and checks that labels are
// translated keys, not hardcoded English, and that missing fields are omitted.
func TestLocationDetailsRendersTranslatedLabels(t *testing.T) {
	raw, err := GetTemplatesFS().ReadFile("template/component/location_details.tmpl")
	if err != nil {
		t.Fatalf("read partial: %v", err)
	}
	funcs := template.FuncMap{
		"t":    func(lang, key string) string { return lang + ":" + key },
		"dict": TemplateDict,
	}
	tmpl, err := template.New("root").Funcs(funcs).Parse(string(raw))
	if err != nil {
		t.Fatalf("parse partial: %v", err)
	}
	full := map[string]interface{}{"Location": map[string]interface{}{
		"Name": "Summerville", "Country": "United States", "ShortName": "Summerville, SC",
		"Latitude": 33.0185, "Longitude": -80.17565, "Timezone": "America/New_York",
		"Population": 48848, "PopulationFormatted": "48,848",
	}}
	var out strings.Builder
	data, _ := TemplateDict("Data", full, "Lang", "fr")
	if err := tmpl.ExecuteTemplate(&out, "location_details", data); err != nil {
		t.Fatalf("execute full: %v", err)
	}
	for _, want := range []string{"fr:location.label", "fr:location.country", "fr:location.coordinates", "fr:location.timezone", "fr:location.population", "48,848", "33.0185, -80.17565"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	for _, bad := range []string{">Location:<", ">Country:<", "Coordinates:"} {
		if strings.Contains(out.String(), bad) {
			t.Errorf("hardcoded English %q still present", bad)
		}
	}

	sparse := map[string]interface{}{"Location": map[string]interface{}{"Name": "X", "Country": "Y"}}
	out.Reset()
	data, _ = TemplateDict("Data", sparse, "Lang", "en")
	if err := tmpl.ExecuteTemplate(&out, "location_details", data); err != nil {
		t.Fatalf("execute sparse: %v", err)
	}
	if strings.Contains(out.String(), "location.population") || strings.Contains(out.String(), "location.timezone") {
		t.Errorf("empty fields must be omitted: %s", out.String())
	}
	if !strings.Contains(out.String(), "en:location.unknown") {
		t.Errorf("missing coordinates must show the unknown label: %s", out.String())
	}
	if _, err := TemplateDict("odd"); err == nil {
		t.Error("odd argument count must error")
	}
	if _, err := TemplateDict(1, 2); err == nil {
		t.Error("non-string key must error")
	}
}
