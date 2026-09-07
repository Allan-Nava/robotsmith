package visual

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/Allan-Nava/robotsmith/internal/advise"
	"github.com/Allan-Nava/robotsmith/internal/report"
)

func doc(t *testing.T) report.AdviseDoc {
	t.Helper()
	a := advise.Analyze([]advise.Observation{
		{UA: "Mozilla/5.0 (compatible; YisouSpider/5.0)", Requests: 11460},
		{UA: "Mozilla/5.0 (compatible; ChatGPT-User/1.0; +openai.com/bot)", Requests: 8200},
		{UA: "Mozilla/5.0 (compatible; Googlebot/2.1)", Requests: 7050},
		{UA: "Mozilla/5.0 (compatible; Bytespider)", Requests: 5750},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 29200},
	})
	return report.FromAdvise(a, advise.Render(a, "", "example.com"), 5)
}

func TestTheModelIsBuiltFromTheSameDocumentAsTheJSON(t *testing.T) {
	// ⚠️ One layout model, two backends — and the model comes from the SAME document `--json`
	// emits. Two renderings computed independently drift, and then the numbers in the deck
	// disagree with the numbers in the tool, which is the credibility this report exists to have.
	m := From(doc(t), "example.com")
	if m.TotalRequests != 61660 || m.UserAgents != 5 {
		t.Errorf("model = %+v", m)
	}
	if len(m.Bars) == 0 {
		t.Fatal("the decisions must become bars")
	}
	// Heaviest first: the argument is "this one is worth more than those ten".
	for i := 1; i < len(m.Bars); i++ {
		if m.Bars[i].Requests > m.Bars[i-1].Requests {
			t.Errorf("bars are not sorted by volume: %+v", m.Bars)
			break
		}
	}
	// Every bar carries its policy in words, not only in colour.
	for _, b := range m.Bars {
		if b.PolicyLabel == "" || b.Glyph == "" {
			t.Errorf("bar %+v: policy must be readable without seeing colour", b)
		}
		if b.Fraction <= 0 || b.Fraction > 1 {
			t.Errorf("bar %+v: fraction must be relative to the largest bar", b)
		}
	}
	if m.Unjudged <= 0 {
		t.Error("the browser share is not a footnote: it is how much of the traffic cannot be judged")
	}
}

func TestHTMLIsSelfContained(t *testing.T) {
	// The artifact gets emailed and opened on a laptop with no network. A report that needs a CDN
	// is a broken report — which also rules out a chart library.
	h := string(HTML(From(doc(t), "example.com")))
	for _, forbidden := range []string{"http://", "https://cdn", "<script", "<iframe", "@import", "url("} {
		if strings.Contains(h, forbidden) {
			t.Errorf("the report reaches outside itself: found %q", forbidden)
		}
	}
	if !strings.Contains(h, "<svg") {
		t.Error("the chart must be inline SVG")
	}
	if !strings.Contains(h, "@media print") {
		t.Error("it has to print tidily: somebody will attach the PDF of it to a ticket")
	}
	if !strings.Contains(h, "prefers-color-scheme: dark") {
		t.Error("dark mode must be chosen, not left to the browser inverting a white page")
	}
	// The hover layer without JavaScript: an SVG <title> is a native tooltip.
	if !strings.Contains(h, "<title>") {
		t.Error("each bar needs its own <title>, which is the tooltip a static page can have")
	}
}

func TestHTMLCarriesEveryNumberFromTheDocument(t *testing.T) {
	d := doc(t)
	h := string(HTML(From(d, "example.com")))
	for _, dec := range d.Decisions {
		if !strings.Contains(h, dec.Name) {
			t.Errorf("%s is missing from the report", dec.Name)
		}
		if !strings.Contains(h, dec.Why) {
			t.Errorf("%s: the reason must travel with the decision", dec.Name)
		}
	}
	if !strings.Contains(h, "61,660") {
		t.Error("the total must be readable, grouped")
	}
	if !strings.Contains(h, d.RobotsTxt[:40]) {
		t.Error("the generated file belongs in the report: it is what the reader has to approve")
	}
}

func TestPDFIsAValidDocumentWithSelectableText(t *testing.T) {
	b := PDF(From(doc(t), "example.com"))
	if !bytes.HasPrefix(b, []byte("%PDF-1.4")) {
		t.Fatalf("not a PDF: %q", b[:min(16, len(b))])
	}
	for _, want := range []string{"/Type /Catalog", "/Type /Pages", "/Type /Page", "/Font", "xref", "trailer", "%%EOF"} {
		if !bytes.Contains(b, []byte(want)) {
			t.Errorf("the PDF has no %s", want)
		}
	}
	// Text stays text: a picture of a report cannot be searched, quoted or read by a screen reader.
	if !bytes.Contains(b, []byte("YisouSpider")) {
		t.Error("the text must be text, not a rasterised drawing")
	}
	// The xref offsets have to point where they say: a wrong offset is a file that opens in one
	// viewer and not in another.
	if err := checkXref(b); err != nil {
		t.Error(err)
	}
}

func TestPDFIsByteIdenticalForTheSameInput(t *testing.T) {
	// So a report can be diffed in CI instead of eyeballed.
	m := From(doc(t), "example.com")
	if !bytes.Equal(PDF(m), PDF(m)) {
		t.Error("two runs on the same input produced different bytes")
	}
}

func TestPDFAndHTMLAgreeOnEveryNumber(t *testing.T) {
	// The trap this design avoids: two renderers drifting, and the deck disagreeing with the tool.
	m := From(doc(t), "example.com")
	h, p := string(HTML(m)), string(PDF(m))
	numbers := regexp.MustCompile(`\d+\.\d{2}%`).FindAllString(h, -1)
	if len(numbers) < 3 {
		t.Fatalf("expected the shares in the HTML, found %v", numbers)
	}
	for _, n := range numbers {
		if !strings.Contains(p, n) {
			t.Errorf("%s appears in the HTML report and not in the PDF", n)
		}
	}
	for _, b := range m.Bars {
		if !strings.Contains(p, b.Name) {
			t.Errorf("%s is missing from the PDF", b.Name)
		}
	}
}

func TestPDFPaginatesInsteadOfClipping(t *testing.T) {
	// A long list must run onto a second page: content that falls off the bottom is content that
	// silently did not exist.
	var obs []advise.Observation
	for _, n := range []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf",
		"Hotel", "India", "Juliett", "Kilo", "Lima", "Mike", "November", "Oscar", "Papa",
		"Quebec", "Romeo", "Sierra", "Tango", "Uniform", "Victor", "Whiskey", "Xray"} {
		obs = append(obs, advise.Observation{UA: "Mozilla/5.0 (compatible; " + n + "Spider/1.0)", Requests: 1000})
	}
	a := advise.Analyze(obs)
	m := From(report.FromAdvise(a, advise.Render(a, "", "x.com"), len(obs)), "x.com")
	b := PDF(m)
	if pages := bytes.Count(b, []byte("/Type /Page\n")); pages < 2 {
		t.Errorf("24 crawlers must not be squeezed onto one page: %d page(s)", pages)
	}
	if !bytes.Contains(b, []byte("XraySpider")) {
		t.Error("the last crawler fell off the document")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestPDFLinesStayInsideTheMargins(t *testing.T) {
	// ⚠️ A PDF has no reflow: a line that runs off the page is content the reader silently does not
	// get. Courier advances exactly 0.6 em, so this is arithmetic, not taste.
	a := advise.Analyze([]advise.Observation{
		{UA: "Mozilla/5.0 (compatible; ChatGPT-User/1.0; +openai.com/bot)", Requests: 8200},
		{UA: "Mozilla/5.0 (compatible; SomeVeryLongCrawlerNameIndeed/1.0)", Requests: 5000},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 30000},
	})
	m := From(report.FromAdvise(a, advise.Render(a, "", "example.com"), 3), "example.com")
	for _, problem := range checkLineWidths(PDF(m)) {
		t.Error(problem)
	}
}
