package report

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/robotsmith/internal/advise"
	"github.com/Allan-Nava/robotsmith/internal/check"
	"github.com/Allan-Nava/robotsmith/internal/lint"
)

func TestSchemaNamesAreVersioned(t *testing.T) {
	// ⚠️ These strings are a promise: a consumer pins them. Changing a field in a breaking way
	// means bumping the number here, never editing the shape in place.
	for got, want := range map[string]string{
		SchemaCheck:  "robotsmith.check/1",
		SchemaLint:   "robotsmith.lint/1",
		SchemaAdvise: "robotsmith.advise/1",
	} {
		if got != want {
			t.Errorf("schema = %q, expected %q", got, want)
		}
	}
}

func TestLintReportIsWhatACIAnnotationNeeds(t *testing.T) {
	findings := []lint.Finding{
		{Sev: lint.Error, Msg: "orphan rule", Line: 3},
		{Sev: lint.Warn, Msg: "allow first", Line: 2},
	}
	r := FromLint("robots.txt", findings)
	if r.OK || r.Errors != 1 || r.Warnings != 1 {
		t.Errorf("report = %+v", r)
	}
	if r.Findings[0].Severity != "ERROR" || r.Findings[0].Line != 3 {
		t.Errorf("findings must carry severity and line: %+v", r.Findings[0])
	}
	// An empty finding list is "ok", and must serialise as [] rather than null: a consumer
	// iterating the field should not have to special-case a missing one.
	var b bytes.Buffer
	if err := Write(&b, FromLint("robots.txt", nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"findings": []`) {
		t.Errorf("an empty list must serialise as []:\n%s", b.String())
	}
	if !strings.Contains(b.String(), `"ok": true`) {
		t.Error("no finding means ok")
	}
}

func TestAdviseReportNamesPoliciesAndFamiliesInWords(t *testing.T) {
	// Numbers from a Go iota are meaningless outside this binary: the JSON says "block", not 1.
	a := advise.Analyze([]advise.Observation{
		{UA: "Mozilla/5.0 (compatible; GPTBot/1.4)", Requests: 500},
		{UA: "Mozilla/5.0 (compatible; Googlebot/2.1)", Requests: 500},
	})
	r := FromAdvise(a, "the file", 2)
	policies := map[string]string{}
	for _, d := range r.Decisions {
		policies[d.Name] = d.Policy
		if d.Family == "" {
			t.Errorf("decision %s has no family", d.Name)
		}
	}
	if policies["GPTBot"] != "block" || policies["Googlebot"] != "allow" {
		t.Errorf("policies = %v", policies)
	}
	if r.RobotsTxt != "the file" || r.UserAgents != 2 {
		t.Errorf("report = %+v", r)
	}
}

func TestWriteEndsWithANewline(t *testing.T) {
	// A JSON document with no trailing newline confuses line-oriented tools (and `jq` piping).
	var b bytes.Buffer
	if err := Write(&b, FromLint("x", nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(b.String(), "\n") {
		t.Error("the document must end with a newline")
	}
	var any map[string]any
	if err := json.Unmarshal(b.Bytes(), &any); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestCheckReportCarriesTheCacheHeadersWhenTheCDNSaysAnything(t *testing.T) {
	// `X-Cache`/`Age` are the only hint a run gets about a stale copy when no --origin is given:
	// they belong in the document, and must be absent (not empty) when the CDN says nothing.
	res := &check.Result{
		URL: "https://example.com/robots.txt", Cases: 32, Failed: 1, Deindex: true,
		Problems: []string{"`Googlebot` is BLOCKED but must get through"},
		Headers:  http.Header{"X-Cache": []string{"Hit from cloudfront"}, "Age": []string{"120"}},
	}
	d := FromCheck(res, "/", nil)
	if d.OK || d.Cases != 32 || !d.Deindex {
		t.Errorf("document = %+v", d)
	}
	if d.Cache == nil || d.Cache.XCache != "Hit from cloudfront" || d.Cache.Age != "120" {
		t.Errorf("cache = %+v", d.Cache)
	}
	bare := FromCheck(&check.Result{URL: "u", Headers: http.Header{}}, "/", nil)
	if bare.Cache != nil {
		t.Errorf("with no cache header the field must be absent, got %+v", bare.Cache)
	}
	if !bare.OK {
		t.Error("no problem means ok")
	}
	var b bytes.Buffer
	if err := Write(&b, bare); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), `"cache"`) {
		t.Errorf("an absent cache must not serialise at all:\n%s", b.String())
	}
}

func TestCrawlersDocKeepsEvaluationOrder(t *testing.T) {
	// The first matching pattern wins, so the order is the answer: the document must not sort.
	d := FromRules(advise.Rules())
	if d.Schema != SchemaCrawlers || len(d.Rules) < 40 {
		t.Fatalf("document = %+v", d.Schema)
	}
	if d.Rules[0].Token != "Googlebot" {
		t.Errorf("first rule = %+v, expected the allowlist first", d.Rules[0])
	}
	for _, r := range d.Rules {
		if r.Family == "" || r.Policy == "" || r.Pattern == "" {
			t.Errorf("rule with an empty field: %+v", r)
		}
	}
}

func TestAdviseDocCarriesEvidenceOnlyWhenThereIsSome(t *testing.T) {
	// From a log: paths and a span. From a count: nothing, and the field must be absent rather
	// than an empty object a consumer would have to interpret.
	when := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	withLog := advise.Analyze([]advise.Observation{
		{UA: "Mozilla/5.0 (compatible; SomeSpider/1.0)", Requests: 5000, Evidence: &advise.Evidence{
			TopPaths: []advise.PathCount{{Path: "/archive", Requests: 4000}},
			First:    when, Last: when.Add(3 * time.Hour),
		}},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 5000},
	})
	d := FromAdvise(withLog, "file", 2)
	var seen bool
	for _, dec := range d.Decisions {
		if dec.Name != "SomeSpider" {
			continue
		}
		seen = true
		if dec.Evidence == nil || len(dec.Evidence.TopPaths) != 1 {
			t.Fatalf("evidence = %+v", dec.Evidence)
		}
		if dec.Evidence.TopPaths[0].Path != "/archive" || dec.Evidence.FirstSeen == "" {
			t.Errorf("evidence = %+v", dec.Evidence)
		}
		if dec.Evidence.LastSeen != "2026-09-04T13:00:00Z" {
			t.Errorf("last_seen = %q, expected RFC 3339 in UTC", dec.Evidence.LastSeen)
		}
	}
	if !seen {
		t.Fatal("SomeSpider must appear among the decisions")
	}

	bare := FromAdvise(advise.Analyze([]advise.Observation{
		{UA: "Mozilla/5.0 (compatible; GPTBot/1.4)", Requests: 10},
	}), "file", 1)
	var b bytes.Buffer
	if err := Write(&b, bare); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "evidence") {
		t.Errorf("a count input must claim no evidence at all:\n%s", b.String())
	}
}

func TestCheckDocReportsSitemapAnswers(t *testing.T) {
	res := &check.Result{URL: "u", Headers: http.Header{}, Sitemaps: []check.SitemapReport{
		{URL: "https://example.com/sitemap.xml", Status: 404, Problem: "answers HTTP 404"},
	}}
	d := FromCheck(res, "/", nil)
	if len(d.Sitemaps) != 1 || d.Sitemaps[0].Status != 404 || d.Sitemaps[0].Problem == "" {
		t.Errorf("sitemaps = %+v", d.Sitemaps)
	}
	var b bytes.Buffer
	if err := Write(&b, FromCheck(&check.Result{URL: "u", Headers: http.Header{}}, "/", nil)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "sitemaps") {
		t.Error("with --sitemaps off the field must not appear at all")
	}
}
