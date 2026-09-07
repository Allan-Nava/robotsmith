package check

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestSitemapsAreOnlyFetchedWhenAsked(t *testing.T) {
	// ⚠️ A verification tool must not make network calls nobody asked for: the sitemap of a large
	// site is a big file, and fetching it silently turns a lint into traffic.
	var hits int32
	sm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<urlset></urlset>`))
	}))
	t.Cleanup(sm.Close)
	body := "User-agent: *\nDisallow:\nSitemap: " + sm.URL + "/sitemap.xml\n"

	res, err := Run(serve(t, body, 200), "", "/")
	if err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Errorf("the sitemap was fetched %d times without being asked", n)
	}
	if len(res.Sitemaps) != 0 {
		t.Errorf("no sitemap report expected by default, got %+v", res.Sitemaps)
	}

	res, err = RunWith(Options{URL: serve(t, body, 200), Path: "/", Sitemaps: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("expected exactly one fetch, got %d", n)
	}
	if len(res.Sitemaps) != 1 {
		t.Fatalf("expected one sitemap report, got %+v", res.Sitemaps)
	}
	got := res.Sitemaps[0]
	if got.Status != 200 || got.ContentType != "application/xml" || got.Bytes == 0 {
		t.Errorf("sitemap report = %+v", got)
	}
	if got.Problem != "" {
		t.Errorf("a healthy sitemap has no problem, got %q", got.Problem)
	}
}

func TestASitemapThatDoesNotAnswerIsAProblem(t *testing.T) {
	// The common failure after a migration: the file is right, the sitemap 404s, and nobody knows
	// until Search Console complains weeks later.
	gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(gone.Close)
	body := "User-agent: *\nDisallow:\nSitemap: " + gone.URL + "/sitemap.xml\n"

	res, err := RunWith(Options{URL: serve(t, body, 200), Path: "/", Sitemaps: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sitemaps) != 1 || res.Sitemaps[0].Status != 404 {
		t.Fatalf("sitemaps = %+v", res.Sitemaps)
	}
	if res.Sitemaps[0].Problem == "" {
		t.Error("a 404 sitemap must be reported as a problem")
	}
	if !strings.Contains(strings.Join(res.Problems, " "), "sitemap") {
		t.Errorf("it must reach the verdict, not just the detail: %v", res.Problems)
	}
	if res.Deindex {
		t.Error("an unreachable sitemap does not deindex: that flag is for a blocked search engine")
	}
}

func TestRunIsStillTheOneArgumentForm(t *testing.T) {
	// Run stays: it is the call every existing caller makes, and Options is the way to ask for more.
	res, err := Run(serve(t, healthy, 200), "", "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) != 0 {
		t.Errorf("problems = %v", res.Problems)
	}
}

func TestExpectVerifiesTheDecisionsSomebodyActuallyMade(t *testing.T) {
	// `check` verifies a fixed set of cases. This verifies THIS site's decisions — which is what
	// somebody silently reverts by regenerating the file from a copied list.
	body := "User-agent: Bytespider\nAllow: /\n\nUser-agent: GPTBot\nDisallow: /\n\nUser-agent: *\nDisallow: /admin/\n"
	res, err := RunWith(Options{URL: serve(t, body, 200), Path: "/", Expect: []Expectation{
		{UA: "Bytespider", Allow: true, Why: "we license our content to them"},
		{UA: "GPTBot", Allow: false, Why: "takes without giving"},
		{UA: "CCBot", Allow: false, Why: "same"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Expected) != 3 {
		t.Fatalf("expected one result per expectation, got %+v", res.Expected)
	}
	byUA := map[string]ExpectResult{}
	for _, e := range res.Expected {
		byUA[e.UA] = e
	}
	if !byUA["Bytespider"].Met || !byUA["GPTBot"].Met {
		t.Errorf("both stated decisions are honoured by the file: %+v", res.Expected)
	}
	// The quoted line is the point: a verdict a reader cannot check is one they must trust.
	if byUA["GPTBot"].Line == 0 || !strings.Contains(byUA["GPTBot"].Quote, "Disallow: /") {
		t.Errorf("GPTBot = %+v, expected the deployed file's own line quoted", byUA["GPTBot"])
	}
	// CCBot was never mentioned in the file: it falls through to `*`, which allows it. The
	// expectation said it must be blocked, so this is a divergence — and it must be visible as
	// "inherited from *", not as a rule somebody wrote.
	cc := byUA["CCBot"]
	if cc.Met {
		t.Error("CCBot is allowed by the file but was expected blocked")
	}
	if !cc.ViaStar {
		t.Errorf("CCBot = %+v: the report must say the answer came from the * group", cc)
	}
	if !strings.Contains(strings.Join(res.Problems, " "), "CCBot") {
		t.Errorf("a divergence must reach the verdict: %v", res.Problems)
	}
}

func TestExpectIsOffUnlessAsked(t *testing.T) {
	res, err := Run(serve(t, healthy, 200), "", "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Expected) != 0 {
		t.Errorf("no expectations were given: %+v", res.Expected)
	}
}
