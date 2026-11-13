package server

import (
	"html/template"
	"strings"
	"testing"
)

// TestAlertSectionRendersNativeDetails renders the shared alert partial and
// checks that details expand without JavaScript, severity maps to a class the
// stylesheet defines, and empty sections render nothing.
func TestAlertSectionRendersNativeDetails(t *testing.T) {
	raw, err := GetTemplatesFS().ReadFile("template/component/alert_section.tmpl")
	if err != nil {
		t.Fatalf("read partial: %v", err)
	}
	funcs := template.FuncMap{
		"t": func(lang, key string) string { return lang + ":" + key },
	}
	tmpl, err := template.New("root").Funcs(funcs).Parse(string(raw))
	if err != nil {
		t.Fatalf("parse partial: %v", err)
	}
	alert := map[string]interface{}{
		"Event": "Tornado Warning", "Severity": "Extreme", "Urgency": "Immediate", "Country": "US",
		"Headline": "Take cover", "AreaDesc": "Charleston County", "DistanceMiles": 12.4,
		"Description": "Rotation detected", "Instruction": "Move indoors",
	}
	var out strings.Builder
	data, _ := TemplateDict("Items", []map[string]interface{}{alert}, "Icon", "T", "Title", "Tornadoes", "Lang", "de")
	if err := tmpl.ExecuteTemplate(&out, "alert_section", data); err != nil {
		t.Fatalf("execute: %v", err)
	}
	html := out.String()
	for _, want := range []string{"severity-extreme", "<details", "<summary", "de:units.mi", "de:severe.weather_instructions_label", "Move indoors"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
	for _, bad := range []string{"display-none", "data-action", " mi<"} {
		if strings.Contains(html, bad) {
			t.Errorf("unexpected %q in %s", bad, html)
		}
	}

	out.Reset()
	empty, _ := TemplateDict("Items", []map[string]interface{}{}, "Icon", "T", "Title", "X", "Lang", "en")
	if err := tmpl.ExecuteTemplate(&out, "alert_section", empty); err != nil {
		t.Fatalf("execute empty: %v", err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Errorf("empty section must render nothing, got %q", out.String())
	}
}
