// Package check verifies a robots.txt as it is actually served: that it exists, that it is FRESH,
// and that it says what it should.
package check

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Allan-Nava/robotsmith/internal/matcher"
)

// The expected cases a healthy robots.txt must pass: whoever brings visits gets through, whoever
// takes without giving is blocked. This is the "policy" half of the check, deliberately kept
// separate from the parser.
var (
	MustPass = []string{
		"Googlebot", "Google-InspectionTool", "Storebot-Google", "bingbot", "DuckDuckBot", "Applebot",
		"facebookexternalhit", "Twitterbot", "LinkedInBot", "WhatsApp", "TelegramBot", "Slackbot",
		"Discordbot", "Pinterest", "ChatGPT-User", "OAI-SearchBot",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
	}
	MustBeBlocked = []string{
		"YisouSpider", "GPTBot", "CCBot", "ClaudeBot", "anthropic-ai", "PerplexityBot", "Bytespider",
		"Amazonbot", "meta-externalagent", "SemrushBot", "AhrefsBot", "MJ12bot", "DotBot", "BLEXBot",
		"DataForSeoBot",
	}
)

// Result is the outcome of the verification.
type Result struct {
	URL      string
	Body     string
	Headers  http.Header
	Problems []string
	Cases    int
	Failed   int
	Deindex  bool // at least one search engine is blocked: the only urgent case
	// Sitemaps is filled only when Options.Sitemaps asked for it: see SitemapReport.
	Sitemaps []SitemapReport
}

// SitemapReport is what one `Sitemap:` line actually answers. `lint` checks the host, which catches
// a copy-paste from another site; this catches the more common one — a sitemap that 404s or
// redirects after a migration, invisible until Search Console complains weeks later.
type SitemapReport struct {
	URL         string
	Status      int
	ContentType string
	Bytes       int64
	Problem     string
}

// Options is the full form of the verification. ⚠️ Sitemaps defaults to off on purpose: a
// verification tool must not make network calls nobody asked for, and the sitemap of a large site
// is a big file.
type Options struct {
	URL      string
	Origin   string
	Path     string
	Sitemaps bool
}

var client = &http.Client{Timeout: 20 * time.Second}

// userAgent identifies this tool to whoever reads their logs: a crawler-policy tool showing up
// disguised would be a poor joke.
const userAgent = "robotsmith/1.0 (+github.com/Allan-Nava/robotsmith)"

func fetch(u string) (string, http.Header, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != 200 {
		return "", resp.Header, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return string(b), resp.Header, nil
}

// Run downloads and verifies. If `origin` is given, it compares the public copy against the
// origin: that is the only reliable way to tell whether a CDN is still serving an old version.
//
// ⚠️ The cache-buster trick (`?cb=<timestamp>`) is NOT enough: if the query string is not part of
// the cache key — the normal setup for a static file — both fetches return the same copy and the
// comparison reports "up to date" while the CDN serves the old one.
func Run(pubURL, originURL string, path string) (*Result, error) {
	return RunWith(Options{URL: pubURL, Origin: originURL, Path: path})
}

// RunWith is Run with everything that is off by default.
func RunWith(o Options) (*Result, error) {
	pubURL, originURL, path := o.URL, o.Origin, o.Path
	body, hdr, err := fetch(pubURL)
	if err != nil {
		return nil, fmt.Errorf("%s not reachable: %w", pubURL, err)
	}
	r := &Result{URL: pubURL, Body: body, Headers: hdr}

	if originURL != "" {
		ob, _, oerr := fetch(originURL)
		switch {
		case oerr != nil:
			r.Problems = append(r.Problems, fmt.Sprintf("origin not reachable: %v", oerr))
		case strings.TrimSpace(ob) != strings.TrimSpace(body):
			r.Problems = append(r.Problems, fmt.Sprintf(
				"the CDN serves a version DIFFERENT from the origin (%d bytes against %d): the change "+
					"is on the origin but crawlers still see the old one. %s needs invalidating",
				len(body), len(ob), path))
		}
	}

	rt := matcher.Parse(body)
	if path == "" {
		path = "/"
	}
	for _, ua := range MustPass {
		r.Cases++
		if !rt.Allowed(ua, path) {
			r.Failed++
			msg := fmt.Sprintf("`%s` is BLOCKED but must get through", ua)
			if isSearchEngine(ua) {
				msg += " — ⛔ DEINDEXING RISK"
				r.Deindex = true
			}
			r.Problems = append(r.Problems, msg)
		}
	}
	for _, ua := range MustBeBlocked {
		r.Cases++
		if rt.Allowed(ua, path) {
			r.Failed++
			r.Problems = append(r.Problems, fmt.Sprintf("`%s` gets through but should be blocked", ua))
		}
	}
	if o.Sitemaps {
		for _, sm := range rt.Sitemaps {
			rep := fetchSitemap(sm)
			r.Sitemaps = append(r.Sitemaps, rep)
			if rep.Problem != "" {
				r.Problems = append(r.Problems, fmt.Sprintf("the sitemap %s %s", rep.URL, rep.Problem))
			}
		}
	}
	return r, nil
}

// fetchSitemap asks the sitemap whether it is there. It reads the body only to size it: nothing
// here parses XML, because "does it answer" is the question that goes unnoticed, not "is it valid".
func fetchSitemap(u string) SitemapReport {
	rep := SitemapReport{URL: u}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		rep.Problem = "is not a valid URL: " + err.Error()
		return rep
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		rep.Problem = "is not reachable: " + err.Error()
		return rep
	}
	defer resp.Body.Close()
	rep.Status = resp.StatusCode
	rep.ContentType = strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<20))
	rep.Bytes = n
	switch {
	case resp.StatusCode != 200:
		rep.Problem = fmt.Sprintf("answers HTTP %d: search engines cannot read it", resp.StatusCode)
	case n == 0:
		rep.Problem = "answers 200 with an empty body"
	}
	return rep
}

func isSearchEngine(ua string) bool {
	l := strings.ToLower(ua)
	for _, m := range []string{"googlebot", "bingbot", "google-", "storebot", "duckduck", "applebot"} {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}

// RobotsURL normalises an input like "example.com" or "https://example.com" into the URL of its
// robots.txt.
func RobotsURL(in string) (string, string, error) {
	if !strings.Contains(in, "://") {
		in = "https://" + in
	}
	u, err := url.Parse(in)
	if err != nil {
		return "", "", err
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/robots.txt"
	}
	return u.String(), u.Hostname(), nil
}
