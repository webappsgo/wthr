// Package httputil provides HTTP-facing helpers shared across the server,
// including the AI.md PART 14 HTML-to-terminal-text converter.
package httputil

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// defaultTextWidth is the column width used for rendered frontend text when
// the caller has no better signal for one.
const defaultTextWidth = 80

// scriptStylePattern matches the contents of <script>/<style> elements, which
// never render as visible text and are stripped before parsing.
var scriptStylePattern = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</\s*(script|style)\s*>`)

// tagPattern matches any HTML tag and is used by the parse-error fallback.
var tagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

// sanitizeTerminalText strips terminal control characters from text extracted
// out of rendered HTML. Content served to curl/wget/httpie is terminal output,
// so an unescaped ESC/BEL/DEL byte in a page (a title, a link target, a table
// cell) would otherwise let page content repaint the user's screen, clear it,
// or retitle their terminal. Tab and newline both become a plain space (a
// literal tab jumps the cursor, a source newline can forge a converter-looking
// banner or table border) and carriage return is dropped (it returns the
// cursor to the start of the line and overwrites it). Every remaining
// structural newline in the output is written by the converter itself —
// the <br> case, the block rules, wordWrap — never copied from page content.
// Printable Unicode, including every non-Latin locale this project ships,
// passes through unchanged.
func sanitizeTerminalText(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\t':
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		if r >= 0x80 && r <= 0x9f {
			return -1
		}
		return r
	}, s)
}

// HTML2TextConverter converts rendered HTML to terminal-friendly text per
// AI.md PART 14. Non-interactive HTTP tools (curl, wget, httpie) receive this
// output so a terminal dump stays readable. Interactive-only elements
// (<form>, <input>, <button>) and non-visible elements (<script>, <style>)
// are skipped because a non-interactive client cannot act on them.
//
// A width of zero or less falls back to defaultTextWidth.
func HTML2TextConverter(htmlSrc string, width int) string {
	if width <= 0 {
		width = defaultTextWidth
	}

	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return stripTags(htmlSrc)
	}

	var buf strings.Builder
	convertNode(&buf, doc, width, 0)
	return collapseBlankLines(buf.String())
}

// convertNode recursively converts a single HTML node into formatted text.
func convertNode(buf *strings.Builder, n *html.Node, width, indent int) {
	switch n.Type {
	case html.ElementNode:
		convertElement(buf, n, width, indent)
	case html.TextNode:
		text := strings.TrimSpace(sanitizeTerminalText(n.Data))
		if text != "" {
			buf.WriteString(text)
		}
	default:
		convertChildren(buf, n, width, indent)
	}
}

// convertElement dispatches on the element name, applying the AI.md PART 14
// conversion rules. Unknown elements recurse into their children.
func convertElement(buf *strings.Builder, n *html.Node, width, indent int) {
	switch n.Data {
	case "h1":
		text := getTextContent(n)
		line := strings.Repeat("═", width)
		buf.WriteString(line + "\n")
		buf.WriteString(centerText(strings.ToUpper(text), width) + "\n")
		buf.WriteString(line + "\n\n")
	case "h2":
		buf.WriteString("─── " + getTextContent(n) + " ───\n\n")
	case "h3":
		buf.WriteString("► " + getTextContent(n) + "\n\n")
	case "p":
		// Recurse into the children rather than flattening them, so the inline
		// rules (strong, em, code, a) and the br newline still apply inside a
		// paragraph. getTextContent would collapse all of them into bare words.
		var inner strings.Builder
		convertChildren(&inner, n, width, indent)
		buf.WriteString(wordWrap(inner.String(), width-indent) + "\n\n")
	case "ul":
		convertList(buf, n, width, indent, false)
	case "ol":
		convertList(buf, n, width, indent, true)
	case "a":
		href := sanitizeTerminalText(getAttr(n, "href"))
		buf.WriteString(getTextContent(n) + " [" + href + "]")
	case "strong", "b":
		buf.WriteString("*" + getTextContent(n) + "*")
	case "em", "i":
		buf.WriteString("_" + getTextContent(n) + "_")
	case "code":
		buf.WriteString("`" + getTextContent(n) + "`")
	case "pre":
		for _, line := range strings.Split(getPreformattedText(n), "\n") {
			buf.WriteString("    " + line + "\n")
		}
		buf.WriteString("\n")
	case "table":
		convertTable(buf, n, width)
	case "hr":
		buf.WriteString(strings.Repeat("─", width) + "\n\n")
	case "blockquote":
		for _, line := range strings.Split(getPreformattedText(n), "\n") {
			buf.WriteString("│ " + line + "\n")
		}
		buf.WriteString("\n")
	case "br":
		buf.WriteString("\n")
	case "form", "input", "button", "script", "style":
		return
	default:
		convertChildren(buf, n, width, indent)
	}
}

// convertChildren recurses into every child of n, preserving the current
// indent level.
func convertChildren(buf *strings.Builder, n *html.Node, width, indent int) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		convertNode(buf, c, width, indent)
	}
}

// convertList renders <ul> as bullets and <ol> as numbers, one item per <li>.
func convertList(buf *strings.Builder, n *html.Node, width, indent int, ordered bool) {
	item := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "li" {
			continue
		}
		marker := "  • "
		if ordered {
			marker = "  " + strconv.Itoa(item) + ". "
			item++
		}
		buf.WriteString(strings.Repeat(" ", indent) + marker + wordWrap(getTextContent(c), width-indent-4) + "\n")
	}
	buf.WriteString("\n")
}

// convertTable renders a <table> as an ASCII grid using │, ─ and ┼ so the
// structure survives a plain terminal dump.
func convertTable(buf *strings.Builder, n *html.Node, width int) {
	rows := collectTableRows(n)
	if len(rows) == 0 {
		return
	}

	colCount := 0
	for _, row := range rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	widths := make([]int, colCount)
	for _, row := range rows {
		for i, cell := range row {
			if len([]rune(cell))+2 > widths[i] {
				widths[i] = len([]rune(cell)) + 2
			}
		}
	}

	buf.WriteString(tableBorder(widths, "┌", "┬", "┐") + "\n")
	for ri, row := range rows {
		cells := make([]string, colCount)
		for i := range cells {
			value := ""
			if i < len(row) {
				value = row[i]
			}
			cells[i] = " " + value + strings.Repeat(" ", widths[i]-len([]rune(value))-1)
		}
		buf.WriteString("│" + strings.Join(cells, "│") + "│\n")
		if ri == 0 {
			buf.WriteString(tableBorder(widths, "├", "┼", "┤") + "\n")
		}
	}
	buf.WriteString(tableBorder(widths, "└", "┴", "┘") + "\n\n")
}

// tableBorder builds one horizontal border line of an ASCII table.
func tableBorder(widths []int, left, middle, right string) string {
	var sb strings.Builder
	sb.WriteString(left)
	for i, w := range widths {
		if i > 0 {
			sb.WriteString(middle)
		}
		sb.WriteString(strings.Repeat("─", w))
	}
	sb.WriteString(right)
	return sb.String()
}

// collectTableRows flattens a <table> into rows of cell text, descending
// through <thead>/<tbody>/<tfoot> section wrappers.
func collectTableRows(n *html.Node) [][]string {
	var rows [][]string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			switch c.Data {
			case "tr":
				row := make([]string, 0, 4)
				for cell := c.FirstChild; cell != nil; cell = cell.NextSibling {
					if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
						row = append(row, getTextContent(cell))
					}
				}
				rows = append(rows, row)
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return rows
}

// getTextContent returns the concatenated, space-collapsed text of every
// descendant of n, so markup nesting does not leak into the result.
func getTextContent(n *html.Node) string {
	var parts []string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			if text := strings.TrimSpace(sanitizeTerminalText(node.Data)); text != "" {
				parts = append(parts, text)
			}
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script", "style", "form", "input", "button":
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(parts, " ")
}

// getPreformattedText returns the text of every descendant of n with its
// original line breaks intact. getTextContent is the opposite: it collapses
// every run of text into single spaces, which is right for inline content but
// destroys the line structure that <pre> and <blockquote> exist to preserve.
func getPreformattedText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		switch node.Type {
		case html.TextNode:
			sb.WriteString(sanitizeTerminalText(node.Data))
			return
		case html.ElementNode:
			switch node.Data {
			case "script", "style", "form", "input", "button":
				return
			case "br":
				sb.WriteString("\n")
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}

// getAttr returns the value of the named attribute on n, or an empty string
// when the attribute is absent.
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// wordWrap breaks text into lines no longer than width columns, splitting on
// whitespace and never dropping a word that is itself longer than the limit.
// Existing line breaks (for example from a <br> already converted to "\n")
// are hard boundaries: each source line is wrapped independently so a
// deliberate break is not folded back into a single space.
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	if strings.TrimSpace(text) == "" {
		return ""
	}

	var out []string
	for _, sourceLine := range strings.Split(text, "\n") {
		words := strings.Fields(sourceLine)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			if len([]rune(current))+1+len([]rune(word)) <= width {
				current += " " + word
				continue
			}
			out = append(out, current)
			current = word
		}
		out = append(out, current)
	}
	return strings.Join(out, "\n")
}

// centerText centers text within width columns, clamping to the left edge
// when the text is wider than the field.
func centerText(text string, width int) string {
	runes := []rune(text)
	if len(runes) >= width {
		return text
	}
	left := (width - len(runes)) / 2
	return strings.Repeat(" ", left) + text
}

// collapseBlankLines trims trailing spaces on each line and caps runs of
// blank lines at one, so inline markup never leaves ragged gaps.
func collapseBlankLines(s string) string {
	lines := strings.Split(strings.TrimRight(s, " \n"), "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, trimmed)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// stripTags is the parse-error fallback: it removes script/style blocks and
// all remaining tags, then collapses leftover whitespace.
func stripTags(s string) string {
	cleaned := scriptStylePattern.ReplaceAllString(s, " ")
	cleaned = tagPattern.ReplaceAllString(cleaned, " ")
	return collapseBlankLines(sanitizeTerminalText(strings.Join(strings.Fields(cleaned), " ")))
}
