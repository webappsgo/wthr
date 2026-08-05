package server

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
)

// TestGetTemplatesFS verifies the embedded templates filesystem is populated
// and contains at least one .tmpl file under the template/ tree.
func TestGetTemplatesFS(t *testing.T) {
	fsys := GetTemplatesFS()

	entries, err := fsys.ReadDir("template")
	if err != nil {
		t.Fatalf("ReadDir(template) failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry under template/, got none")
	}

	found := false
	err = fs.WalkDir(fsys, "template", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".tmpl") {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}
	if !found {
		t.Fatal("expected at least one .tmpl file embedded under template/, found none")
	}
}

// TestGetStaticFS verifies the embedded static files filesystem is populated.
func TestGetStaticFS(t *testing.T) {
	fsys := GetStaticFS()

	entries, err := fsys.ReadDir("static")
	if err != nil {
		t.Fatalf("ReadDir(static) failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry under static/, got none")
	}
}

// TestGetStaticSubFS verifies the static sub-filesystem strips the static/
// prefix and exposes files directly at their relative paths.
func TestGetStaticSubFS(t *testing.T) {
	subFS, err := GetStaticSubFS()
	if err != nil {
		t.Fatalf("GetStaticSubFS() returned error: %v", err)
	}

	entries, err := fs.ReadDir(subFS, ".")
	if err != nil {
		t.Fatalf("reading static sub-filesystem failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry in static sub-filesystem, got none")
	}
}

// TestLoadTemplates verifies the embedded templates parse without error and
// that at least one named template is registered.
func TestLoadTemplates(t *testing.T) {
	tmpl, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}
	if tmpl == nil {
		t.Fatal("LoadTemplates() returned a nil template")
	}

	templates := tmpl.Templates()
	if len(templates) == 0 {
		t.Fatal("expected at least one parsed template, got none")
	}
}

// TestLoadTemplatesFuncMap verifies the placeholder helper functions
// registered by LoadTemplates (upper, lower, title, add, sub, t) all
// produce the expected output when actually invoked through template
// execution, since parsing alone never calls their bodies.
func TestLoadTemplatesFuncMap(t *testing.T) {
	tmpl, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}

	helperTmpl, err := tmpl.New("helpers").Parse(
		`{{upper "abc"}}|{{lower "XYZ"}}|{{title "hello world"}}|{{add 2 3}}|{{sub 5 2}}|{{t "" "greeting.hello"}}`,
	)
	if err != nil {
		t.Fatalf("failed to parse helper template: %v", err)
	}

	var buf bytes.Buffer
	if err := helperTmpl.Execute(&buf, nil); err != nil {
		t.Fatalf("failed to execute helper template: %v", err)
	}

	got := buf.String()
	want := "ABC|xyz|Hello World|5|3|greeting.hello"
	if got != want {
		t.Fatalf("helper template output = %q, want %q", got, want)
	}
}
