package httputil

import (
	"strings"
	"testing"
)

// TestHTML2TextConverter_Headings verifies the h1/h2/h3 box-drawing rules.
func TestHTML2TextConverter_Headings(t *testing.T) {
	out := HTML2TextConverter(`<h1>Title</h1><h2>Sub</h2><h3>Sec</h3>`, 20)

	if !strings.Contains(out, strings.Repeat("═", 20)) {
		t.Errorf("h1 rule missing, got:\n%s", out)
	}
	if !strings.Contains(out, "TITLE") {
		t.Errorf("h1 text not uppercased, got:\n%s", out)
	}
	if !strings.Contains(out, "─── Sub ───") {
		t.Errorf("h2 rule missing, got:\n%s", out)
	}
	if !strings.Contains(out, "► Sec") {
		t.Errorf("h3 rule missing, got:\n%s", out)
	}
}

// TestHTML2TextConverter_CenterText verifies centering pads left of the text.
func TestHTML2TextConverter_CenterText(t *testing.T) {
	got := centerText("AB", 8)
	if got != "   AB" {
		t.Errorf("centerText = %q, want %q", got, "   AB")
	}
	if got := centerText("ABCDEFGH", 4); got != "ABCDEFGH" {
		t.Errorf("centerText on overflow = %q, want it left as-is", got)
	}
}

// TestHTML2TextConverter_InlineRules verifies the inline emphasis, code and
// link rendering rules.
func TestHTML2TextConverter_InlineRules(t *testing.T) {
	out := HTML2TextConverter(`<p>Go <strong>bold</strong> and <em>soft</em> at <code>x</code> or <a href="/u">here</a>.</p>`, 60)

	for _, want := range []string{"*bold*", "_soft_", "`x`", "here [/u]"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestHTML2TextConverter_SkipsNonInteractiveElements verifies the AI.md
// SKIP list: forms, inputs, buttons, scripts and styles never reach output.
func TestHTML2TextConverter_SkipsNonInteractiveElements(t *testing.T) {
	out := HTML2TextConverter(
		`<p>Visible</p><form action="/x"><input name="q" value="secret"><button>Send</button></form>`+
			`<script>var token = 1;</script><style>.a{color:red}</style>`, 60)

	for _, unwanted := range []string{"secret", "Send", "var token", "color:red", "<form>"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("skipped content %q leaked into output:\n%s", unwanted, out)
		}
	}
	if !strings.Contains(out, "Visible") {
		t.Errorf("surrounding text lost, got:\n%s", out)
	}
}

// TestHTML2TextConverter_Lists verifies bullet and numbered list rendering.
func TestHTML2TextConverter_Lists(t *testing.T) {
	out := HTML2TextConverter(`<ul><li>one</li><li>two</li></ul><ol><li>first</li><li>second</li></ol>`, 60)

	for _, want := range []string{"• one", "• two", "1. first", "2. second"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestHTML2TextConverter_Table verifies the ASCII grid draws all borders.
func TestHTML2TextConverter_Table(t *testing.T) {
	out := HTML2TextConverter(
		`<table><tr><th>City</th><th>Temp</th></tr><tr><td>Rome</td><td>21</td></tr></table>`, 60)

	for _, want := range []string{"┌", "┬", "┐", "├", "┼", "┤", "└", "┴", "┘", "City", "Rome"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q in:\n%s", want, out)
		}
	}
}

// TestHTML2TextConverter_BlockquotePreHrBr verifies the remaining block rules.
func TestHTML2TextConverter_BlockquotePreHrBr(t *testing.T) {
	out := HTML2TextConverter(`<hr><blockquote>quoted</blockquote><pre>line1
line2</pre><p>a<br>b</p>`, 10)

	for _, want := range []string{strings.Repeat("─", 10), "│ quoted", "    line1", "    line2", "a\nb"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestHTML2TextConverter_PreservesFullDocument verifies that the html parser
// handling of a complete page (head plus body) does not lose body content.
func TestHTML2TextConverter_PreservesFullDocument(t *testing.T) {
	out := HTML2TextConverter(
		`<!DOCTYPE html><html lang="en"><head><title>Admin</title></head>`+
			`<body><h1>Dashboard</h1><p>Welcome back</p></body></html>`, 40)

	if !strings.Contains(out, "DASHBOARD") || !strings.Contains(out, "Welcome back") {
		t.Errorf("document body lost, got:\n%s", out)
	}
}

// TestWordWrap verifies wrapping honors the column limit and never drops a
// word that is longer than the limit.
func TestWordWrap(t *testing.T) {
	got := wordWrap("aaa bbb ccc ddd", 7)
	for _, line := range strings.Split(got, "\n") {
		if len([]rune(line)) > 7 {
			t.Errorf("line %q exceeds width 7", line)
		}
	}
	if len(strings.Fields(got)) != 4 {
		t.Errorf("wordWrap dropped words: %q", got)
	}
	if got := wordWrap("supercalifragilistic", 5); got != "supercalifragilistic" {
		t.Errorf("long word = %q, want it kept intact", got)
	}
	if got := wordWrap("anything", 0); got != "anything" {
		t.Errorf("zero width = %q, want input unchanged", got)
	}
}

// TestHTML2TextConverter_NonPositiveWidthDefaults verifies the documented
// fallback instead of dividing by or wrapping to a nonsense width.
func TestHTML2TextConverter_NonPositiveWidthDefaults(t *testing.T) {
	for _, width := range []int{0, -5} {
		out := HTML2TextConverter(`<h1>T</h1>`, width)
		if !strings.Contains(out, strings.Repeat("═", defaultTextWidth)) {
			t.Errorf("width %d did not fall back to %d, got:\n%s", width, defaultTextWidth, out)
		}
	}
}

// TestHTML2TextConverter_EmptyInput verifies a safe empty result.
func TestHTML2TextConverter_EmptyInput(t *testing.T) {
	if got := HTML2TextConverter("", 80); got != "" {
		t.Errorf("empty input = %q, want empty string", got)
	}
}

// TestStripTags verifies the fallback helper removes markup and scripts.
func TestStripTags(t *testing.T) {
	got := stripTags(`<div>Hello <b>world</b><script>bad()</script></div>`)
	if strings.Contains(got, "<") || strings.Contains(got, "bad()") {
		t.Errorf("stripTags left markup or script content: %q", got)
	}
	if !strings.Contains(got, "Hello world") {
		t.Errorf("stripTags lost text: %q", got)
	}
}

// TestCollapseBlankLines verifies the whitespace normalizer.
func TestCollapseBlankLines(t *testing.T) {
	got := collapseBlankLines("a   \n\n\n\nb\n\n\n")
	if got != "a\n\nb\n" {
		t.Errorf("collapseBlankLines = %q, want %q", got, "a\n\nb\n")
	}
}
