package main

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"

	"github.com/Allan-Nava/robotsmith/internal/check"
	"github.com/Allan-Nava/robotsmith/internal/lint"
	"github.com/Allan-Nava/robotsmith/internal/matcher"
)

// ⚠️ These tests are the dogfooding: the file this project publishes is held to the tool's own
// standard, offline and on every run. A robots.txt advisor shipping a robots.txt its own linter
// rejects would be the whole credibility of the thing.

func TestOurOwnRobotsTxtHasNoStructuralDefect(t *testing.T) {
	body := readDoc(t, "docs/robots.txt")
	if findings := lint.Check(body, "allan-nava.github.io"); len(findings) != 0 {
		for _, f := range findings {
			t.Errorf("docs/robots.txt: %s (line %d) %s", f.Sev, f.Line, f.Msg)
		}
	}
}

func TestOurOwnRobotsTxtPassesOurOwnExpectedCases(t *testing.T) {
	// The same 32 cases `check` runs against a stranger's site, applied to ours.
	rt := matcher.Parse(readDoc(t, "docs/robots.txt"))
	for _, ua := range check.MustPass {
		if !rt.Allowed(ua, "/") {
			t.Errorf("%q is blocked by our own file but must get through", ua)
		}
	}
	for _, ua := range check.MustBeBlocked {
		if rt.Allowed(ua, "/") {
			t.Errorf("%q gets through our own file but should be blocked", ua)
		}
	}
}

func TestOurOwnRobotsTxtSaysWhereItHasToLiveToWork(t *testing.T) {
	// ⚠️ This is a PROJECT page: the file is served at /robotsmith/robots.txt, and crawlers read
	// robots.txt only from the host root — so this copy governs nothing on its own. A tool whose
	// README is about files that look right and do not work must not ship one silently.
	body := readDoc(t, "docs/robots.txt")
	for _, want := range []string{"host root", "allan-nava.github.io/robots.txt"} {
		if !strings.Contains(body, want) {
			t.Errorf("our robots.txt must explain that only the host-root copy is read (missing %q)", want)
		}
	}
	if !strings.Contains(body, "Sitemap: https://allan-nava.github.io/robotsmith/sitemap.xml") {
		t.Error("it must point at this site's own sitemap: that is what the host-root file syncs from")
	}
}

func TestWeShipASitemapSoTheHostRootCanListUs(t *testing.T) {
	// The host-root file lists only sitemaps that answer 200. No sitemap here means this site
	// never appears there — which is the actual, checkable gap.
	b, err := os.ReadFile("docs/sitemap.xml")
	if err != nil {
		t.Fatalf("docs/sitemap.xml: %v", err)
	}
	var set struct {
		XMLName xml.Name `xml:"urlset"`
		URLs    []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(b, &set); err != nil {
		t.Fatalf("the sitemap must be valid XML: %v", err)
	}
	if len(set.URLs) == 0 {
		t.Fatal("an empty sitemap is worse than none: it wastes crawl budget")
	}
	for _, u := range set.URLs {
		if !strings.HasPrefix(u.Loc, "https://allan-nava.github.io/robotsmith/") {
			t.Errorf("loc %q is not on this site: a cross-site entry is ignored", u.Loc)
		}
	}
}

func TestTheDogfoodJobIsScheduled(t *testing.T) {
	// Running it once proves nothing: the published file is changed by other people, in another
	// repo, on their own schedule.
	w := readDoc(t, ".github/workflows/dogfood.yml")
	for _, want := range []string{"schedule:", "cron:", "workflow_dispatch", "allan-nava.github.io"} {
		if !strings.Contains(w, want) {
			t.Errorf("the dogfood workflow is missing %q", want)
		}
	}
}
