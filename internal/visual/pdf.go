package visual

import (
	"bytes"
	"fmt"
	"strings"
)

// PDF renders the same model as a PDF, with no external converter and no headless browser: a
// report that only builds where Chrome is installed does not build in CI.
//
// ⚠️ Text stays text. The base-14 fonts (Helvetica, Helvetica-Bold, Courier) need no embedding, so
// the file has no dependency and its content is selectable, searchable and readable by a screen
// reader — a rasterised picture of a report is none of those. Content streams are left
// uncompressed on purpose: the output is then byte-identical for the same input, which is what
// lets a report be diffed in CI instead of eyeballed.
//
// The layout comes from the same Model as the HTML: see the note at the top of model.go.
type pdfDoc struct {
	pages   []string // one content stream per page
	current strings.Builder
	y       float64
}

// Page geometry, in points (A4). Everything is laid out top-down from `top`.
const (
	pageW, pageH = 595.0, 842.0
	marginX      = 48.0
	top          = pageH - 56
	bottom       = 56.0
)

// The light palette, as PDF has no notion of a colour scheme. Same hues as the HTML, validated
// against the light surface.
var pdfColours = map[string][3]float64{
	"allow":  {0x2a / 255.0, 0x78 / 255.0, 0xd6 / 255.0},
	"block":  {0xd0 / 255.0, 0x3b / 255.0, 0x3b / 255.0},
	"review": {0xed / 255.0, 0xa1 / 255.0, 0x00 / 255.0},
	"ignore": {0x52 / 255.0, 0x51 / 255.0, 0x4e / 255.0},
}

func PDF(m Model) []byte {
	d := &pdfDoc{y: top}

	d.text("Helvetica-Bold", 18, marginX, "robots.txt advice - "+m.Host)
	d.gap(6)
	d.text("Helvetica", 10, marginX, fmt.Sprintf(
		"Derived from %s observed requests across %d distinct user-agents. Every line says why.",
		m.TotalReadable, m.UserAgents))
	d.gap(14)

	d.text("Helvetica-Bold", 11, marginX, "The three numbers")
	d.gap(4)
	d.text("Courier", 9, marginX, fmt.Sprintf("%-14s requests observed", m.TotalReadable))
	d.text("Courier", 9, marginX, fmt.Sprintf("%-14s touched by the advised blocks",
		fmt.Sprintf("%.1f%%", m.SavingShare)))
	d.text("Courier", 9, marginX, fmt.Sprintf("%-14s this method cannot judge",
		fmt.Sprintf("%.1f%%", m.Unjudged)))
	d.gap(4)
	d.text("Helvetica", 9, marginX, fmt.Sprintf(
		"The last one is not a footnote: %.1f%% of the traffic declares a browser user-agent, and", m.Unjudged))
	d.text("Helvetica", 9, marginX,
		"telling a person from a disguised scraper needs the per-IP rate, which this method does not look at.")
	d.gap(16)

	if len(m.Bars) > 0 {
		d.text("Helvetica-Bold", 11, marginX, "What the traffic costs, by crawler")
		d.gap(4)
		d.text("Helvetica", 8, marginX, "+ ALLOW brings visits    x BLOCK takes without giving    ? REVIEW a person decides")
		d.gap(10)
		d.bars(m)
		d.gap(12)
	}

	d.text("Helvetica-Bold", 11, marginX, "Every decision, and why")
	d.gap(6)
	// ⚠️ The widths are chosen so the longest possible line fits inside the margins: Courier
	// advances 0.6 em, so `columns * size * 0.6` must stay under the printable width, and
	// TestPDFLinesStayInsideTheMargins does that arithmetic instead of trusting this comment.
	for _, r := range m.Rows {
		d.text("Courier", 8.5, marginX, fmt.Sprintf("%s %-6s %-20s %9s %7.2f%%  %s",
			r.Glyph, r.PolicyLabel, truncate(r.Name, 20), grouped(r.Requests), r.Share,
			truncate(r.Why, 42)))
		if r.FromPolicy {
			d.text("Courier", 8, marginX+14, "your policy: "+r.Rule)
		}
	}
	d.gap(12)

	if len(m.Movements) > 0 {
		d.text("Helvetica-Bold", 11, marginX, "What moved since the previous run")
		d.gap(6)
		for _, mv := range m.Movements {
			d.text("Courier", 8.5, marginX, fmt.Sprintf("%-9s %-22s %6.2f%% -> %6.2f%%  %s",
				mv.Direction, truncate(mv.Name, 22), mv.Was, mv.Now, mv.FactorReadable))
		}
		d.gap(12)
	}

	for _, w := range m.Warnings {
		d.text("Helvetica-Bold", 9, marginX, "! "+firstSentence(w))
		for _, line := range wrap(rest(w), 104) {
			d.text("Helvetica", 9, marginX+10, line)
		}
		d.gap(8)
	}

	d.text("Helvetica-Bold", 11, marginX, "The file this advice produces")
	d.gap(6)
	for _, line := range strings.Split(strings.TrimRight(m.RobotsTxt, "\n"), "\n") {
		d.text("Courier", 7.5, marginX, truncate(line, 108))
	}
	d.gap(10)
	d.text("Helvetica", 8, marginX, "robotsmith - "+m.Schema+
		" - robots.txt is a request, not a control: whoever disguises itself as a browser ignores it.")

	return d.build()
}

// bars draws the chart with the same geometry decisions as the SVG: horizontal, sorted by volume,
// each value labelled at the end of its own bar.
func (d *pdfDoc) bars(m Model) {
	const (
		rowH   = 13.0
		barH   = 7.0
		labelW = 108.0
		plot   = 250.0
	)
	for _, bar := range m.Bars {
		d.ensure(rowH)
		y := d.y - barH
		d.textAt("Courier", 8, marginX, y+1, truncate(bar.Name, 18))
		w := bar.Fraction * plot
		if w < 1.5 {
			// A zero-width bar disappears; a hairline says "present but tiny", which is the honest
			// reading of a crawler at 0.02%.
			w = 1.5
		}
		c := pdfColours[bar.Policy]
		if c == [3]float64{} {
			c = pdfColours["ignore"]
		}
		fmt.Fprintf(&d.current, "%.3f %.3f %.3f rg %.1f %.1f %.1f %.1f re f\n",
			c[0], c[1], c[2], marginX+labelW, y, w, barH)
		d.textAt("Courier", 8, marginX+labelW+w+6, y+1,
			fmt.Sprintf("%s %s  %.2f%%", bar.Glyph, bar.PolicyLabel, bar.Share))
		d.y -= rowH
	}
}

// ── the writer ──────────────────────────────────────────────────────────────

func (d *pdfDoc) gap(h float64) { d.y -= h }

// ensure starts a new page when the next line would fall off the bottom. Content that falls off is
// content that silently did not exist.
func (d *pdfDoc) ensure(h float64) {
	if d.y-h >= bottom {
		return
	}
	d.pages = append(d.pages, d.current.String())
	d.current.Reset()
	d.y = top
}

func (d *pdfDoc) text(font string, size, x float64, s string) {
	d.ensure(size + 4)
	d.textAt(font, size, x, d.y-size, s)
	d.y -= size + 4
}

func (d *pdfDoc) textAt(font string, size, x, y float64, s string) {
	fmt.Fprintf(&d.current, "0 0 0 rg BT /%s %.1f Tf %.1f %.1f Td (%s) Tj ET\n",
		fontRef(font), size, x, y, pdfString(s))
}

func fontRef(name string) string {
	switch name {
	case "Helvetica-Bold":
		return "F2"
	case "Courier":
		return "F3"
	default:
		return "F1"
	}
}

// pdfString escapes what would break a literal string, and transliterates the few non-Latin-1
// characters this report uses: the base-14 fonts have no glyph for them, and a missing glyph is a
// hole in the sentence.
func pdfString(s string) string {
	s = strings.NewReplacer(
		"✓", "+", "✗", "x", "×", "x", "→", "->", "…", "...", "⚠️", "!", "⚠", "!",
		"⏳", "", "—", "-", "–", "-", "’", "'", "“", `"`, "”", `"`, "·", "-",
		"─", "-", "✅", "OK", "⛔", "!!",
	).Replace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 32:
			b.WriteByte(' ')
		case r < 127:
			b.WriteRune(r)
		case r <= 255:
			// Latin-1 is what WinAnsiEncoding covers well enough for accented prose.
			b.WriteRune(r)
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// build assembles the objects and the cross-reference table. ⚠️ Every xref offset must point at
// the byte where its object starts: a wrong offset is a file that opens in one viewer and not in
// another, which is the classic hand-rolled-PDF bug.
func (d *pdfDoc) build() []byte {
	if d.current.Len() > 0 {
		d.pages = append(d.pages, d.current.String())
	}
	if len(d.pages) == 0 {
		d.pages = []string{""}
	}
	n := len(d.pages)

	// 1 catalog · 2 pages · 3,4,5 fonts · then, per page, the page object and its content stream
	firstPage := 6
	var out bytes.Buffer
	offsets := []int{} // 1-based object offsets

	obj := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}

	out.WriteString("%PDF-1.4\n")
	obj("<< /Type /Catalog /Pages 2 0 R >>")

	kids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", firstPage+i*2))
	}
	obj(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", n, strings.Join(kids, " ")))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>")

	for i, content := range d.pages {
		contentRef := firstPage + i*2 + 1
		obj(fmt.Sprintf("<< /Type /Page\n/Parent 2 0 R\n/MediaBox [0 0 %.0f %.0f]\n"+
			"/Resources << /Font << /F1 3 0 R /F2 4 0 R /F3 5 0 R >> >>\n/Contents %d 0 R >>",
			pageW, pageH, contentRef))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content))
	}

	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets)+1, xref)
	return out.Bytes()
}

func firstSentence(s string) string {
	if i := strings.Index(s, ": "); i > 0 && i < 90 {
		return s[:i]
	}
	if len(s) > 90 {
		return s[:90]
	}
	return s
}

func rest(s string) string {
	if i := strings.Index(s, ": "); i > 0 && i < 90 {
		return strings.TrimSpace(s[i+1:])
	}
	if len(s) > 90 {
		return s[90:]
	}
	return ""
}

// wrap breaks prose at a column, on word boundaries: a PDF has no reflow, so a long line is a line
// that runs off the page.
func wrap(s string, cols int) []string {
	var out []string
	for len(s) > 0 {
		if len(s) <= cols {
			out = append(out, s)
			break
		}
		cut := strings.LastIndex(s[:cols], " ")
		if cut <= 0 {
			cut = cols
		}
		out = append(out, strings.TrimSpace(s[:cut]))
		s = strings.TrimSpace(s[cut:])
	}
	return out
}
