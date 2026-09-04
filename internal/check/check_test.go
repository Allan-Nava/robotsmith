package check

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRobotsURL(t *testing.T) {
	cases := []struct {
		in, url, host string
	}{
		{"esempio.it", "https://esempio.it/robots.txt", "esempio.it"},
		{"https://esempio.it", "https://esempio.it/robots.txt", "esempio.it"},
		{"https://esempio.it/", "https://esempio.it/robots.txt", "esempio.it"},
		// an explicit path must NOT be overwritten: origins sometimes serve the file from a
		// different path
		{"https://origin.interno/static/robots.txt", "https://origin.interno/static/robots.txt", "origin.interno"},
		{"http://esempio.it", "http://esempio.it/robots.txt", "esempio.it"},
	}
	for _, c := range cases {
		u, h, err := RobotsURL(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if u != c.url || h != c.host {
			t.Errorf("%s → (%s, %s), expected (%s, %s)", c.in, u, h, c.url, c.host)
		}
	}
}

// serve exposes a robots.txt on a test server.
func serve(t *testing.T, body string, status int) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s.URL + "/robots.txt"
}

// healthy is a file that must pass every case: whoever brings visits gets through, whoever takes
// without giving does not.
const healthy = `User-agent: Googlebot
User-agent: Google-InspectionTool
User-agent: Storebot-Google
User-agent: bingbot
User-agent: DuckDuckBot
User-agent: Applebot
User-agent: facebookexternalhit
User-agent: Twitterbot
User-agent: LinkedInBot
User-agent: WhatsApp
User-agent: TelegramBot
User-agent: Slackbot
User-agent: Discordbot
User-agent: Pinterest
User-agent: ChatGPT-User
User-agent: OAI-SearchBot
Allow: /

User-agent: YisouSpider
User-agent: GPTBot
User-agent: CCBot
User-agent: ClaudeBot
User-agent: anthropic-ai
User-agent: PerplexityBot
User-agent: Bytespider
User-agent: Amazonbot
User-agent: meta-externalagent
User-agent: SemrushBot
User-agent: AhrefsBot
User-agent: MJ12bot
User-agent: DotBot
User-agent: BLEXBot
User-agent: DataForSeoBot
Disallow: /

User-agent: *
Disallow: /admin/
`

func TestRunOnHealthyFileFindsNoProblem(t *testing.T) {
	res, err := Run(serve(t, healthy, 200), "", "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) != 0 {
		t.Errorf("expected no problem, got: %v", res.Problems)
	}
	if res.Cases != len(MustPass)+len(MustBeBlocked) {
		t.Errorf("cases verified = %d, expected %d", res.Cases, len(MustPass)+len(MustBeBlocked))
	}
}

func TestRunOnEmptyFileBlocksNothing(t *testing.T) {
	// ⚠️ 200 with 0 bytes is NOT "everything forbidden": every scraper gets through.
	res, err := Run(serve(t, "", 200), "", "/")
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != len(MustBeBlocked) {
		t.Errorf("failed = %d, expected %d (every block missing)", res.Failed, len(MustBeBlocked))
	}
	if res.Deindex {
		t.Error("an empty file does not deindex: it must not raise the red flag")
	}
}

func TestRunReportsDeindexingRisk(t *testing.T) {
	res, err := Run(serve(t, "User-agent: *\nDisallow: /\n", 200), "", "/")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Deindex {
		t.Fatal("a blocked Googlebot must raise the deindexing flag")
	}
	if !strings.Contains(strings.Join(res.Problems, " "), "DEINDEXING") {
		t.Error("the problem must be stated in plain words")
	}
}

func TestRunAgainstOriginDetectsStaleCache(t *testing.T) {
	// The CDN serves the old copy, the origin the new one: this is the only comparison that finds
	// it, because a query-string cache-buster returns the same copy when it is not in the cache key.
	pub := serve(t, "User-agent: *\nDisallow: /admin/\n", 200)
	org := serve(t, healthy, 200)
	res, err := Run(pub, org, "/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(res.Problems, " "), "DIFFERENT from the origin") {
		t.Errorf("the CDN/origin divergence must be reported, problems: %v", res.Problems)
	}
}

func TestRunAgainstIdenticalOriginSaysNothing(t *testing.T) {
	// Trailing whitespace aside, public and origin match: nothing to say about the cache.
	pub := serve(t, healthy, 200)
	org := serve(t, healthy+"\n\n", 200)
	res, err := Run(pub, org, "/")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Problems {
		if strings.Contains(p, "origin") {
			t.Errorf("expected no cache problem, got: %s", p)
		}
	}
}

func TestRunOnUnreachableFileIsAnError(t *testing.T) {
	// A 404 is not "no rules": it is a missing file, and it must be told apart (exit 4).
	if _, err := Run(serve(t, "not found", 404), "", "/"); err == nil {
		t.Error("a 404 must return an error, not an empty result")
	}
}
