package visual

import (
	"fmt"
	"html"
	"strings"
)

// Palette, validated with the dataviz validator against both surfaces (see INTENT.md):
//
//	allow  #2a78d6 / #3987e5   block #d03b3b   review #eda100 / #c98500
//
// ⚠️ Colour is the SECOND channel here, never the first: every bar carries its policy word and a
// glyph, because red and green are one colour to a deuteranope and a printed page has none. The
// light review amber sits under 3:1 on the light surface, whose documented relief is exactly what
// this report already has — visible labels and a full table.
const css = `:root{--bg:#fcfcfb;--ink:#0b0b0b;--muted:#52514e;--line:#e4e0d9;--panel:#fff;
--allow:#2a78d6;--block:#d03b3b;--review:#eda100;--code:#f2efe9}
@media (prefers-color-scheme: dark){:root{--bg:#1a1a19;--ink:#fff;--muted:#c3c2b7;--line:#31312e;
--panel:#232320;--allow:#3987e5;--block:#d03b3b;--review:#c98500;--code:#111110}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);
font:15px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
.wrap{max-width:900px;margin:0 auto;padding:32px 24px 64px}
h1{font-size:24px;margin:0 0 4px}h2{font-size:17px;margin:36px 0 8px}
.sub{color:var(--muted);font-size:14px;margin:0}
.tiles{display:flex;flex-wrap:wrap;gap:12px;margin:20px 0 8px}
.tile{background:var(--panel);border:1px solid var(--line);border-radius:10px;padding:12px 16px;min-width:150px}
.tile .n{font:600 22px/1.2 ui-monospace,SFMono-Regular,Menlo,monospace}
.tile .l{color:var(--muted);font-size:12px;text-transform:uppercase;letter-spacing:.06em}
table{width:100%;border-collapse:collapse;font-size:14px;margin:8px 0}
th,td{text-align:left;padding:7px 10px;border-bottom:1px solid var(--line);vertical-align:top}
th{font-size:11px;letter-spacing:.06em;text-transform:uppercase;color:var(--muted);font-weight:600}
td.n{text-align:right;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;white-space:nowrap}
.tag{font:600 11px/1 ui-monospace,SFMono-Regular,Menlo,monospace;padding:3px 6px;border-radius:4px;
border:1px solid currentColor;white-space:nowrap}
.allow{color:var(--allow)}.block{color:var(--block)}.review{color:var(--review)}
.legend{display:flex;gap:16px;flex-wrap:wrap;color:var(--muted);font-size:13px;margin:4px 0 12px}
.legend span{display:inline-flex;align-items:center;gap:6px}
.sw{width:10px;height:10px;border-radius:2px;display:inline-block}
pre{background:var(--code);border:1px solid var(--line);border-radius:8px;padding:12px 14px;
overflow-x:auto;font:12px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace}
.note{border-left:3px solid var(--review);padding:8px 12px;background:var(--panel);
border-radius:0 8px 8px 0;font-size:14px;margin:12px 0}
footer{margin-top:40px;padding-top:16px;border-top:1px solid var(--line);color:var(--muted);font-size:12px}
@media print{
 body{background:#fff;color:#000}
 .wrap{max-width:none;padding:0}
 .tile,.note,pre{break-inside:avoid}
 h2{break-after:avoid}
 table{font-size:11px}
 footer{position:static}
}`

// HTML renders the report as one self-contained file: inline CSS, inline SVG, no JavaScript and
// nothing fetched. The artifact gets emailed and opened on a laptop with no network — a report that
// needs a CDN is a broken report, which also rules out a chart library.
func HTML(m Model) []byte {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>robots.txt advice for %s</title>\n<style>%s</style>\n</head>\n<body>\n",
		esc(m.Host), css)
	b.WriteString("<div class=\"wrap\">\n")

	fmt.Fprintf(&b, "<h1>robots.txt advice — %s</h1>\n", esc(m.Host))
	fmt.Fprintf(&b, "<p class=\"sub\">Derived from %s observed requests across %d distinct "+
		"user-agents. Every line below says why.</p>\n", m.TotalReadable, m.UserAgents)

	// Tiles: the three numbers a reader needs before any chart.
	b.WriteString("<div class=\"tiles\">\n")
	tile(&b, m.TotalReadable, "requests observed")
	tile(&b, fmt.Sprintf("%.1f%%", m.SavingShare), "touched by the advised blocks")
	tile(&b, fmt.Sprintf("%.1f%%", m.Unjudged), "this method cannot judge")
	b.WriteString("</div>\n")
	fmt.Fprintf(&b, "<p class=\"sub\">The last tile is not a footnote: %.1f%% of the traffic "+
		"declares a browser user-agent, and telling a person from a disguised scraper needs the "+
		"per-IP rate, which this method does not look at.</p>\n", m.Unjudged)

	if len(m.Bars) > 0 {
		b.WriteString("<h2>What the traffic costs, by crawler</h2>\n")
		b.WriteString("<div class=\"legend\">")
		for _, l := range []struct{ cls, txt string }{
			{"allow", "✓ ALLOW — brings visits"},
			{"block", "✗ BLOCK — takes without giving"},
			{"review", "? REVIEW — a person decides"},
		} {
			fmt.Fprintf(&b, "<span><i class=\"sw\" style=\"background:var(--%s)\"></i>%s</span>", l.cls, l.txt)
		}
		b.WriteString("</div>\n")
		bars(&b, m)
	}

	b.WriteString("<h2>Every decision, and why</h2>\n<table>\n<thead><tr><th>Crawler</th>" +
		"<th>Family</th><th>Policy</th><th class=\"n\">Requests</th><th class=\"n\">Share</th>" +
		"<th>Reason</th></tr></thead>\n<tbody>\n")
	for _, r := range m.Rows {
		why := esc(r.Why)
		if r.FromPolicy {
			why += fmt.Sprintf(" <em>(your policy: %s)</em>", esc(r.Rule))
		}
		fmt.Fprintf(&b, "<tr><td><b>%s</b></td><td>%s</td><td><span class=\"tag %s\">%s %s</span></td>"+
			"<td class=\"n\">%s</td><td class=\"n\">%.2f%%</td><td>%s</td></tr>\n",
			esc(r.Name), esc(r.Family), r.Policy, r.Glyph, r.PolicyLabel, grouped(r.Requests), r.Share, why)
	}
	b.WriteString("</tbody>\n</table>\n")

	if len(m.Movements) > 0 {
		b.WriteString("<h2>What moved since the previous run</h2>\n<table>\n<thead><tr>" +
			"<th>Crawler</th><th>Direction</th><th class=\"n\">Was</th><th class=\"n\">Now</th>" +
			"<th class=\"n\">Factor</th></tr></thead>\n<tbody>\n")
		for _, mv := range m.Movements {
			fmt.Fprintf(&b, "<tr><td><b>%s</b></td><td>%s</td><td class=\"n\">%.2f%%</td>"+
				"<td class=\"n\">%.2f%%</td><td class=\"n\">%s</td></tr>\n",
				esc(mv.Name), esc(mv.Direction), mv.Was, mv.Now, esc(mv.FactorReadable))
		}
		b.WriteString("</tbody>\n</table>\n")
	}

	for _, w := range m.Warnings {
		fmt.Fprintf(&b, "<div class=\"note\">⚠️ %s</div>\n", esc(w))
	}

	b.WriteString("<h2>The file this advice produces</h2>\n")
	b.WriteString("<p class=\"sub\">This is what you would deploy. Nothing here was inferred " +
		"after the fact: it is the same file the command writes to stdout.</p>\n")
	fmt.Fprintf(&b, "<pre>%s</pre>\n", esc(m.RobotsTxt))

	fmt.Fprintf(&b, "<footer>robotsmith · %s · robots.txt is a request, not a control: "+
		"whoever disguises itself as a browser ignores it, and for those you need a request cap "+
		"or a WAF.</footer>\n", esc(m.Schema))
	b.WriteString("</div>\n</body>\n</html>\n")
	return []byte(b.String())
}

func tile(b *strings.Builder, n, l string) {
	fmt.Fprintf(b, "<div class=\"tile\"><div class=\"n\">%s</div><div class=\"l\">%s</div></div>\n",
		esc(n), esc(l))
}

// bars draws the chart. Horizontal bars sorted by volume, one row per crawler, the value labelled
// at the end of its own bar — the argument being made is "this one is worth more than those ten",
// which is exactly what a pie chart destroys at the sizes that matter.
func bars(b *strings.Builder, m Model) {
	const (
		rowH   = 26.0
		barH   = 12.0
		labelW = 150.0
		valueW = 116.0
		width  = 860.0
	)
	plot := width - labelW - valueW
	h := rowH*float64(len(m.Bars)) + 8

	fmt.Fprintf(b, "<svg viewBox=\"0 0 %.0f %.0f\" width=\"100%%\" role=\"img\" "+
		"aria-label=\"Requests per crawler, by policy\" style=\"max-width:%.0fpx\">\n", width, h, width)
	for i, bar := range m.Bars {
		y := float64(i)*rowH + 4
		// The whole row is one accessible unit: the tooltip a page with no JavaScript can have.
		fmt.Fprintf(b, "<g><title>%s — %s requests, %.2f%% of the observed traffic (%s)</title>\n",
			esc(bar.Name), grouped(bar.Requests), bar.Share, esc(bar.PolicyLabel))
		fmt.Fprintf(b, "<text x=\"0\" y=\"%.1f\" font-size=\"12\" font-family=\"ui-monospace,"+
			"SFMono-Regular,Menlo,monospace\" fill=\"currentColor\">%s</text>\n",
			y+barH-1, esc(truncate(bar.Name, 20)))
		// ⚠️ A zero-width rect disappears; a hairline says "present but tiny", which is the honest
		// reading of a crawler at 0.02%.
		w := bar.Fraction * plot
		if w < 2 {
			w = 2
		}
		fmt.Fprintf(b, "<rect x=\"%.0f\" y=\"%.1f\" width=\"%.1f\" height=\"%.0f\" rx=\"4\" "+
			"fill=\"var(--%s)\"/>\n", labelW, y, w, barH, bar.Policy)
		fmt.Fprintf(b, "<text x=\"%.1f\" y=\"%.1f\" font-size=\"11\" font-family=\"ui-monospace,"+
			"SFMono-Regular,Menlo,monospace\" fill=\"currentColor\">%s %s  %.2f%%</text>\n",
			labelW+w+8, y+barH-1, bar.Glyph, esc(bar.PolicyLabel), bar.Share)
		b.WriteString("</g>\n")
	}
	b.WriteString("</svg>\n")
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func esc(s string) string { return html.EscapeString(s) }
