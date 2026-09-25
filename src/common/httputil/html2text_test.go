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
// The <pre> and blockquote line breaks come from <br>, which the converter
// emits itself; a raw newline inside the text is flattened to a space by
// sanitizeTerminalText so page content cannot forge its own line structure.
func TestHTML2TextConverter_BlockquotePreHrBr(t *testing.T) {
	out := HTML2TextConverter(`<hr><blockquote>quoted</blockquote><pre>line1<br>line2</pre><p>a<br>b</p>`, 10)

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

// TestSanitizeTerminalText verifies the control-character filter keeps the
// converter's own formatting characters and every printable rune, and drops
// C0 controls, DEL, and the C1 range.
func TestSanitizeTerminalText(t *testing.T) {
	t.Run("drops control characters", func(t *testing.T) {
		// The ESC introducer is removed, so the "[31m" that followed it is
		// inert text rather than a live SGR sequence, and the trailing ESC
		// is what actually would have left the terminal in a modified state.
		got := sanitizeTerminalText("a\x1b[31mb\x07c\x7fd\x00e\x1b")
		if got != "a[31mbcde" {
			t.Errorf("sanitizeTerminalText = %q, want %q", got, "a[31mbcde")
		}
	})

	t.Run("drops C1 controls", func(t *testing.T) {
		got := sanitizeTerminalText("a\u0085b\u009bc")
		if got != "abc" {
			t.Errorf("sanitizeTerminalText = %q, want %q", got, "abc")
		}
	})

	t.Run("spaces tab and newline, drops carriage return", func(t *testing.T) {
		got := sanitizeTerminalText("a\nb\tc\rd")
		if got != "a b cd" {
			t.Errorf("sanitizeTerminalText = %q, want %q", got, "a b cd")
		}
	})

	t.Run("keeps non-latin locales intact", func(t *testing.T) {
		// Every locale this project ships must survive the filter unchanged.
		for _, s := range []string{"não", "für", "löschen", "日本語", "العربية", "Español", "English"} {
			if got := sanitizeTerminalText(s); got != s {
				t.Errorf("sanitizeTerminalText(%q) = %q, want it unchanged", s, got)
			}
		}
	})
}

// TestHTML2TextConverter_StripsTerminalControlCharacters verifies that page
// content cannot repaint a terminal served by curl/wget/httpie. Every
// extraction point is exercised: heading, paragraph, link href, pre block,
// table cell, and the stripTags parse-error fallback.
func TestHTML2TextConverter_StripsTerminalControlCharacters(t *testing.T) {
	const dangerous = "\x1b[31m\x1b]2;pwned\x07\x7b"

	t.Run("headings and paragraphs", func(t *testing.T) {
		out := HTML2TextConverter("<h1>"+dangerous+"Title</h1><p>"+dangerous+"Body</p>", 40)
		assertNoControlChars(t, out)
		if !strings.Contains(out, "TITLE") || !strings.Contains(out, "Body") {
			t.Errorf("sanitizing dropped real text, got:\n%q", out)
		}
	})

	t.Run("link href", func(t *testing.T) {
		out := HTML2TextConverter(`<a href="https://example.com/`+dangerous+`">link</a>`, 40)
		assertNoControlChars(t, out)
		if !strings.Contains(out, "https://example.com/") {
			t.Errorf("link href lost its safe prefix, got:\n%q", out)
		}
	})

	t.Run("preformatted block", func(t *testing.T) {
		out := HTML2TextConverter("<pre>"+dangerous+"line\n</pre>", 40)
		assertNoControlChars(t, out)
		if !strings.Contains(out, "line") {
			t.Errorf("pre block lost its safe text, got:\n%q", out)
		}
	})

	t.Run("table cell", func(t *testing.T) {
		out := HTML2TextConverter(
			"<table><tr><th>"+dangerous+"Head</th></tr><tr><td>Cell</td></tr></table>", 40)
		assertNoControlChars(t, out)
		if !strings.Contains(out, "Head") || !strings.Contains(out, "Cell") {
			t.Errorf("table lost its safe text, got:\n%q", out)
		}
		// Column alignment is computed from sanitized cell text, so every
		// data row must be the same rune width as the border above it.
		var want int
		for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			switch {
			case strings.HasPrefix(line, "┌"):
				want = len([]rune(line))
			case strings.HasPrefix(line, "│"):
				if got := len([]rune(line)); got != want {
					t.Errorf("table row width = %d, want %d, line: %q", got, want, line)
				}
			}
		}
	})

	t.Run("stripTags fallback", func(t *testing.T) {
		assertNoControlChars(t, stripTags("<div>"+dangerous+"text</div>"))
	})

	t.Run("source newlines cannot forge layout", func(t *testing.T) {
		// A raw newline inside the text is page content, not converter
		// formatting, so it must not let a <pre> block or table cell
		// fabricate a converter-drawn banner or border line.
		out := HTML2TextConverter("<pre>harmless\n"+strings.Repeat("═", 40)+"</pre>", 40)
		assertNoControlChars(t, out)
		if strings.Contains(out, "harmless\n"+strings.Repeat("═", 40)) {
			t.Errorf("a source newline forged converter layout, got:\n%q", out)
		}
	})
}

// assertNoControlChars fails t if s carries any terminal control character
// other than newline, carriage return, or tab.
func assertNoControlChars(t *testing.T, s string) {
	t.Helper()
	for _, r := range s {
		switch r {
		case '\n', '\r', '\t':
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			t.Errorf("output still carries control character %U in %q", r, s)
			return
		}
	}
}
