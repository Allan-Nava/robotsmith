package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const nginxLine = `1.2.3.4 - - [01/Sep/2026:10:00:00 +0000] "GET /a HTTP/1.1" 200 12 "-" "Mozilla/5.0 (compatible; GPTBot/1.4)"`

func gzipped(t *testing.T, content string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	if _, err := zw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestReadLogFromStdin(t *testing.T) {
	// Rotated logs arrive down a pipe: `zcat access.log.*.gz | robotsmith advise --log -`.
	obs, _, err := readLog("-", strings.NewReader(nginxLine+"\n"+nginxLine+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].Requests != 2 {
		t.Fatalf("expected one user-agent seen twice, got %+v", obs)
	}
}

func TestReadLogDecompressesGzipFromStdin(t *testing.T) {
	obs, _, err := readLog("-", bytes.NewReader(gzipped(t, nginxLine+"\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].UA != "Mozilla/5.0 (compatible; GPTBot/1.4)" {
		t.Fatalf("a gzipped stream must be decompressed transparently, got %+v", obs)
	}
}

func TestReadLogDetectsGzipByContentNotByName(t *testing.T) {
	// ⚠️ Rotated names lie: `access.log.3`, `access.log-20260904`, `access.log` fed by a pipe. The
	// magic bytes are the only thing that tells the truth.
	dir := t.TempDir()
	misnamed := filepath.Join(dir, "access.log") // gzip content, no .gz extension
	if err := os.WriteFile(misnamed, gzipped(t, nginxLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	obs, _, err := readLog(misnamed, nil)
	if err != nil {
		t.Fatalf("a gzipped file without the .gz extension must still be read: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected 1 user-agent, got %+v", obs)
	}

	plainNamedGz := filepath.Join(dir, "access.log.gz") // plain content, .gz extension
	if err := os.WriteFile(plainNamedGz, []byte(nginxLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if obs, _, err = readLog(plainNamedGz, nil); err != nil {
		t.Fatalf("a plain file named .gz must not be treated as compressed: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected 1 user-agent, got %+v", obs)
	}
}

func TestReadLogOnAnEmptyStreamIsNotAnError(t *testing.T) {
	// An empty rotated log is normal; it must produce no advice, not a failure.
	obs, _, err := readLog("-", strings.NewReader(""))
	if err != nil {
		t.Fatalf("an empty stream is not an error: %v", err)
	}
	if len(obs) != 0 {
		t.Errorf("expected no observation, got %+v", obs)
	}
}

func TestAdviseReadsAGzippedLogEndToEnd(t *testing.T) {
	p := filepath.Join(t.TempDir(), "access.log.gz")
	if err := os.WriteFile(p, gzipped(t, strings.Repeat(nginxLine+"\n", 40)), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI("advise", "--log", p)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "User-agent: GPTBot") {
		t.Errorf("the advice must reach the generated file:\n%s", stdout)
	}
}

func TestReadLogCollectsPathsAndTimeSpan(t *testing.T) {
	// For an unknown crawler the decision a person has to make — channel or leech? — needs the
	// shape of the traffic, and `--log` already reads the lines that carry it.
	log := `1.1.1.1 - - [01/Sep/2026:10:00:00 +0000] "GET /news/a HTTP/1.1" 200 1 "-" "Mozilla/5.0 (compatible; SomeSpider/1.0)"
1.1.1.1 - - [01/Sep/2026:11:30:00 +0000] "GET /news/a HTTP/1.1" 200 1 "-" "Mozilla/5.0 (compatible; SomeSpider/1.0)"
1.1.1.1 - - [01/Sep/2026:14:00:00 +0000] "GET /archive/b HTTP/1.1" 200 1 "-" "Mozilla/5.0 (compatible; SomeSpider/1.0)"
`
	obs, _, err := readLog(writeTemp(t, "access.log", log), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected one user-agent, got %+v", obs)
	}
	ev := obs[0].Evidence
	if ev == nil {
		t.Fatal("a log carries paths and timestamps: they must not be thrown away")
	}
	if len(ev.TopPaths) == 0 || ev.TopPaths[0].Path != "/news/a" || ev.TopPaths[0].Requests != 2 {
		t.Errorf("top paths = %+v, expected /news/a twice first", ev.TopPaths)
	}
	if ev.Last.Sub(ev.First) != 4*time.Hour {
		t.Errorf("span = %v, expected 4h: a crawl spread over hours is not a burst", ev.Last.Sub(ev.First))
	}
}

func TestReadLogSurvivesALineWithNoTimestamp(t *testing.T) {
	// A custom log-format may not carry a date at all. That is a reason to report less, never to
	// fail: the user-agent counts are the part that must always work.
	obs, _, err := readLog("-", strings.NewReader(`- - - "GET /x HTTP/1.1" 200 1 "-" "Mozilla/5.0 (compatible; SomeSpider/1.0)"`+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].Requests != 1 {
		t.Fatalf("observations = %+v", obs)
	}
	if ev := obs[0].Evidence; ev == nil || len(ev.TopPaths) != 1 || !ev.First.IsZero() {
		t.Errorf("evidence = %+v: the path is known, the time is not", obs[0].Evidence)
	}
}

func TestUACountsCarryNoEvidence(t *testing.T) {
	// A `uniq -c` count cannot know paths or times: claiming otherwise would be an invention.
	obs, err := readCounts(writeTemp(t, "ua.txt", "10 Mozilla/5.0 (compatible; SomeSpider/1.0)\n"))
	if err != nil {
		t.Fatal(err)
	}
	if obs[0].Evidence != nil {
		t.Error("a count file has no evidence to carry")
	}
}

// ---------------------------------------------------------------------------
// Log-format coverage. ⚠️ The failure mode this guards against is SILENT: a
// format the parser does not understand produces no crash and no empty output,
// just an advice built on a fraction of the traffic — or on none of it.
// ---------------------------------------------------------------------------

func TestUserAgentIsFoundInEveryRealLogFormat(t *testing.T) {
	const ua = "Mozilla/5.0 (compatible; GPTBot/1.4; +https://openai.com/gptbot)"
	for _, tc := range []struct {
		format string
		line   string
	}{
		{
			// nginx `combined`, the one this parser was born on.
			"nginx combined",
			`1.2.3.4 - - [01/Sep/2026:10:00:00 +0000] "GET /a HTTP/1.1" 200 12 "-" "` + ua + `"`,
		},
		{
			// HAProxy `option httplog` with `capture request header User-Agent len 200`: the
			// captured headers land in {braces}, never in quotes, and the quoted field is the
			// request line. This is the format that used to go silently missing.
			"haproxy httplog with captured headers",
			`Sep  1 10:00:00 lb haproxy[1234]: 1.2.3.4:53512 [01/Sep/2026:10:00:00.123] fe be/srv1 ` +
				`0/0/1/23/24 200 1234 - - ---- 10/10/1/1/0 0/0 {` + ua + `} "GET /a HTTP/1.1"`,
		},
		{
			// Two captures — `capture request header Host` then `User-Agent` — are pipe-separated
			// inside the same brace group.
			"haproxy httplog with two captured headers",
			`Sep  1 10:00:00 lb haproxy[1234]: 1.2.3.4:53512 [01/Sep/2026:10:00:00.123] fe be/srv1 ` +
				`0/0/1/23/24 200 1234 - - ---- 10/10/1/1/0 0/0 {example.com|` + ua + `} "GET /a HTTP/1.1"`,
		},
		{
			// A custom `log-format` that quotes the UA with %{+Q} and separates fields with pipes.
			"haproxy custom log-format",
			`1.2.3.4|01/Sep/2026:10:00:00.123|GET /a HTTP/1.1|200|"` + ua + `"`,
		},
	} {
		t.Run(tc.format, func(t *testing.T) {
			obs, _, err := readLog("-", strings.NewReader(tc.line+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			if len(obs) != 1 || obs[0].UA != ua {
				t.Fatalf("got %+v, expected exactly the user-agent %q", obs, ua)
			}
		})
	}
}

func TestResponseCaptureIsNotMistakenForAUserAgent(t *testing.T) {
	// HAProxy puts the RESPONSE captures in a second brace group: `{text/html}` contains a slash
	// and would otherwise be counted as a crawler, inventing traffic that does not exist.
	const ua = "Mozilla/5.0 (compatible; GPTBot/1.4)"
	line := `Sep  1 10:00:00 lb haproxy[1234]: 1.2.3.4:53512 [01/Sep/2026:10:00:00.123] fe be/srv1 ` +
		`0/0/1/23/24 200 1234 - - ---- 10/10/1/1/0 0/0 {` + ua + `} {text/html; charset=utf-8} "GET /a HTTP/1.1"`
	obs, _, err := readLog("-", strings.NewReader(line+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].UA != ua {
		t.Fatalf("got %+v, expected only the user-agent %q", obs, ua)
	}
}

func TestAFormatWithNoUserAgentWarnsInsteadOfAdvisingOnNothing(t *testing.T) {
	// HAProxy's DEFAULT httplog carries no User-Agent at all: without a `capture request header`
	// there is nothing to read. Answering with a short list is the worst outcome — the advice
	// looks complete and is built on nothing.
	line := `Sep  1 10:00:00 lb haproxy[1234]: 1.2.3.4:53512 [01/Sep/2026:10:00:00.123] fe be/srv1 ` +
		`0/0/1/23/24 200 1234 - - ---- 10/10/1/1/0 0/0 "GET /a HTTP/1.1"`
	obs, warns, err := readLog("-", strings.NewReader(strings.Repeat(line+"\n", 10)))
	if err != nil {
		t.Fatalf("an unreadable format is a reason to say so, not to fail: %v", err)
	}
	if len(obs) != 0 {
		t.Fatalf("observations = %+v, expected none", obs)
	}
	if len(warns) == 0 {
		t.Fatal("a log whose format carries no user-agent must produce a warning, not silence")
	}
	if !strings.Contains(warns[0], "10 of 10") {
		t.Errorf("the warning must say how much was skipped, got %q", warns[0])
	}
}

func TestAWellReadLogProducesNoFormatWarning(t *testing.T) {
	// The warning has to stay rare, or it becomes noise people filter out. A `-` user-agent is
	// normal in any real log and must not, on its own, accuse the format.
	log := nginxLine + "\n" +
		`1.2.3.4 - - [01/Sep/2026:10:00:01 +0000] "GET /b HTTP/1.1" 200 12 "-" "-"` + "\n"
	_, warns, err := readLog("-", strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Errorf("a log the parser reads must warn about nothing, got %q", warns)
	}
}

func TestAdviseSaysSoWhenItCouldNotReadTheFormat(t *testing.T) {
	line := `Sep  1 10:00:00 lb haproxy[1234]: 1.2.3.4:53512 [01/Sep/2026:10:00:00.123] fe be/srv1 ` +
		`0/0/1/23/24 200 1234 - - ---- 10/10/1/1/0 0/0 "GET /a HTTP/1.1"`
	p := writeTemp(t, "access.log", strings.Repeat(line+"\n", 10))
	_, _, stderr := runCLI("advise", "--log", p)
	if !strings.Contains(stderr, "user-agent") {
		t.Errorf("the warning must reach the operator on stderr:\n%s", stderr)
	}
}

// ---------------------------------------------------------------------------
// JSON access logs. ⚠️ The defect these guard against is not a missing format:
// splitting a JSON line on `"` FABRICATES user-agents — the request path and a
// raw JSON fragment both look like candidates — which inflates the denominator
// every share is computed against, and `AutoBlockShare` keys off those shares.
// ---------------------------------------------------------------------------

const jsonUA = "Mozilla/5.0 (compatible; GPTBot/1.4; +https://openai.com/gptbot)"

func TestJSONLogsYieldExactlyOneUserAgentPerLine(t *testing.T) {
	for _, tc := range []struct{ product, line string }{
		{
			// Caddy's default access log: the UA is an ARRAY under request.headers.
			"caddy",
			`{"level":"info","ts":1790000000.5,"logger":"http.log.access","msg":"handled request",` +
				`"request":{"remote_ip":"1.2.3.4","method":"GET","uri":"/news/a",` +
				`"headers":{"User-Agent":["` + jsonUA + `"],"Accept-Encoding":["gzip"]}},"status":200}`,
		},
		{
			"cloudflare logpush",
			`{"ClientIP":"1.2.3.4","ClientRequestPath":"/news/a","ClientRequestUserAgent":"` + jsonUA + `",` +
				`"EdgeStartTimestamp":"2026-09-28T10:00:00Z","EdgeResponseStatus":200}`,
		},
		{
			"ingress-nginx json",
			`{"time_iso8601":"2026-09-28T10:00:00+00:00","request_uri":"/news/a","status":200,` +
				`"http_user_agent":"` + jsonUA + `","http_referer":"-"}`,
		},
	} {
		t.Run(tc.product, func(t *testing.T) {
			obs, warns, err := readLog("-", strings.NewReader(tc.line+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			if len(obs) != 1 {
				got := make([]string, 0, len(obs))
				for _, o := range obs {
					got = append(got, o.UA)
				}
				t.Fatalf("got %d user-agents %q, expected exactly one: a JSON line read by "+
					"splitting on quotes invents them", len(obs), got)
			}
			if obs[0].UA != jsonUA {
				t.Errorf("UA = %q, expected %q", obs[0].UA, jsonUA)
			}
			if obs[0].Requests != 1 {
				t.Errorf("requests = %d, expected 1: the denominator is what every share is "+
					"computed against", obs[0].Requests)
			}
			if len(warns) != 0 {
				t.Errorf("a line we can read must not warn, got %q", warns)
			}
		})
	}
}

func TestJSONLogKeepsTheDenominatorHonest(t *testing.T) {
	// The whole point. Three lines from three products, one crawler: 3 requests at 100%.
	// Before this, the same three lines read as 8 requests from 7 user-agents and put the
	// crawler at 62.5% — a number that decides whether it crosses AutoBlockShare.
	log := `{"ts":1790000000.5,"request":{"uri":"/a","headers":{"User-Agent":["` + jsonUA + `"]}}}
{"ClientRequestPath":"/b","ClientRequestUserAgent":"` + jsonUA + `"}
{"request_uri":"/c","http_user_agent":"` + jsonUA + `"}
`
	obs, _, err := readLog("-", strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].Requests != 3 {
		t.Fatalf("got %+v, expected one user-agent with 3 requests", obs)
	}
}

func TestAJSONLineWithNoUserAgentIsUnreadNotGuessed(t *testing.T) {
	// ⚠️ No falling back to the quote heuristic for a line we know is JSON: that is exactly how a
	// path became a crawler. Unread is honest and lets the 0.5.0 warning see it.
	log := strings.Repeat(`{"ClientIP":"1.2.3.4","ClientRequestPath":"/news/a","EdgeResponseStatus":200}`+"\n", 10)
	obs, warns, err := readLog("-", strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 0 {
		t.Fatalf("got %+v, expected nothing: no user-agent key means we did not read one", obs)
	}
	if len(warns) == 0 {
		t.Error("a log we could not read must say so")
	}
}

func TestJSONLogCarriesPathAndTime(t *testing.T) {
	// The evidence behind a REVIEW has to keep working on these formats too.
	log := `{"ts":1790000000,"request":{"uri":"/news/a?p=2","headers":{"User-Agent":["` + jsonUA + `"]}}}
{"ts":1790003600,"request":{"uri":"/news/a","headers":{"User-Agent":["` + jsonUA + `"]}}}
`
	obs, _, err := readLog("-", strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	ev := obs[0].Evidence
	if ev == nil || len(ev.TopPaths) != 1 || ev.TopPaths[0].Path != "/news/a" {
		t.Fatalf("top paths = %+v, expected /news/a twice (the query string is dropped)", ev)
	}
	if ev.Last.Sub(ev.First) != time.Hour {
		t.Errorf("span = %v, expected 1h", ev.Last.Sub(ev.First))
	}
}

func TestANonJSONLineStartingWithABraceIsStillReadAsText(t *testing.T) {
	// ⚠️ HAProxy's captured headers are wrapped in {braces}: a line can begin with `{` and not be
	// JSON at all. Deciding by the first character alone would lose every HAProxy log.
	line := `{` + jsonUA + `} "GET /a HTTP/1.1" 200`
	obs, _, err := readLog("-", strings.NewReader(line+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].UA != jsonUA {
		t.Fatalf("got %+v, expected the HAProxy capture to still be read", obs)
	}
}

// ---------------------------------------------------------------------------
// ⚠️ Reading JSON as JSON fixed the formats we know about. It did not fix the
// SHAPE of the mistake: the text heuristic still accepts a candidate merely for
// containing a slash or a space, so any format quoting something else keeps
// inflating the totals — silently, and always in the direction of more traffic.
// ---------------------------------------------------------------------------

func TestAFabricatedCandidateIsNotCountedAsACrawler(t *testing.T) {
	// ⚠️ Whole lines, not a synthetic wrapper: the JSON fragment reaches the candidate list through
	// the HAProxy {brace} capture, so wrapping it in quotes would test a path the code never takes
	// and pass for the wrong reason.
	for _, tc := range []struct{ why, line string }{
		{
			"a request path is not a crawler",
			`1.2.3.4 - - [01/Sep/2026:10:00:00 +0000] "/news/a" 200 12`,
		},
		{
			// A JSON document that does not parse (truncated by the log rotator, say) falls through
			// to the text heuristic, where the braces hand the whole body over as one candidate.
			"a fragment of a quoted structure is not a crawler",
			`lb haproxy[1]: {"ClientIP":"1.2.3.4","ClientRequestPath":"/news/b"} 200`,
		},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got := userAgentsIn(tc.line); len(got) != 0 {
				t.Errorf("accepted %q as a user-agent — it inflates the denominator every share is "+
					"computed against, and AutoBlockShare decides against those shares", got)
			}
		})
	}
}

func TestEveryRealUserAgentIsStillAccepted(t *testing.T) {
	// ⚠️ The guard above must stay GENEROUS. User-agents are notoriously irregular, and dropping a
	// genuine crawler is the worse failure: it disappears from the advice entirely, so nobody ever
	// decides about it. This list is the real strings used elsewhere in this suite.
	for _, ua := range []string{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; GPTBot/1.4; +https://openai.com/gptbot)",
		"facebookexternalhit/1.1",
		"Mozilla/5.0 AppleWebKit (compatible; ChatGPT-User/1.0; +openai.com/bot)",
		"Mozilla/5.0 (compatible; SemrushBot/7~bl)",
		"Mozilla/5.0 (compatible; YisouSpider/5.0; +http://www.yisou.com)",
		"okhttp/5.4.0",
		"curl/8.4.0",
		"SomeApp/1 CFNetwork/3860 Darwin/25.6.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/148",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko)",
	} {
		if got := userAgentsIn(`1.2.3.4 - - "GET /a HTTP/1.1" 200 12 "-" "` + ua + `"`); len(got) != 1 || got[0] != ua {
			t.Errorf("real user-agent %q was dropped (got %q): losing a crawler is worse than "+
				"counting a stray field", ua, got)
		}
	}
}

func TestALineOfOnlyFabricatedCandidatesCountsAsUnread(t *testing.T) {
	// Rejecting them is half the job: the line must then be UNREAD, so the 0.5.0 warning sees it
	// rather than the run quietly advising on nothing.
	line := `- - - "/news/a" 200 12`
	obs, warns, err := readLog("-", strings.NewReader(strings.Repeat(line+"\n", 10)))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 0 {
		t.Fatalf("got %+v, expected nothing", obs)
	}
	if len(warns) == 0 {
		t.Error("a log whose lines yielded nothing readable must say so")
	}
}

// ---------------------------------------------------------------------------
// ⚠️ Both defects fixed in this milestone were invisible from the output.
// "Observed 8 requests from 7 distinct user-agents" reads exactly like a
// correct answer, and an operator cannot check a denominator they are never
// shown against a log they could count themselves.
// ---------------------------------------------------------------------------

func TestReadLogReportsWhatItParsed(t *testing.T) {
	// Four non-empty lines: two JSON we can read, one JSON with no user-agent key, one nginx.
	log := `{"request":{"uri":"/a","headers":{"User-Agent":["` + jsonUA + `"]}}}
{"ClientRequestPath":"/b","ClientRequestUserAgent":"` + jsonUA + `"}
{"ClientIP":"1.2.3.4","ClientRequestPath":"/c"}

` + nginxLine + `
`
	_, sum, _, err := readLogWithSummary("-", strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Lines != 4 {
		t.Errorf("lines = %d, expected 4 (the blank line is not a line)", sum.Lines)
	}
	if sum.Read != 3 {
		t.Errorf("read = %d, expected 3", sum.Read)
	}
	if sum.Skipped() != 1 {
		t.Errorf("skipped = %d, expected 1: the JSON line with no user-agent key", sum.Skipped())
	}
	if sum.JSON != 3 || sum.Text != 1 {
		t.Errorf("json/text = %d/%d, expected 3/1", sum.JSON, sum.Text)
	}
	if got := sum.Format(); got != "mixed" {
		t.Errorf("format = %q, expected %q", got, "mixed")
	}
}

func TestTheFormatIsNamedWhenTheLogIsAllOneShape(t *testing.T) {
	for _, tc := range []struct{ want, log string }{
		{"json", `{"ClientRequestUserAgent":"` + jsonUA + `"}` + "\n"},
		{"text", nginxLine + "\n"},
	} {
		_, sum, _, err := readLogWithSummary("-", strings.NewReader(tc.log))
		if err != nil {
			t.Fatal(err)
		}
		if got := sum.Format(); got != tc.want {
			t.Errorf("format = %q, expected %q", got, tc.want)
		}
	}
}

func TestAdvisePrintsWhatItParsedOnStderr(t *testing.T) {
	// ⚠️ stderr, never stdout: `advise … > robots.txt` must stay a clean file.
	p := writeTemp(t, "access.log", strings.Repeat(nginxLine+"\n", 5))
	code, stdout, stderr := runCLI("advise", "--log", p)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "5 of 5") {
		t.Errorf("stderr must say how many lines were read, got:\n%s", stderr)
	}
	if strings.Contains(stdout, "of 5") {
		t.Errorf("the parse summary must not reach stdout:\n%s", stdout)
	}
}

func TestTheJSONDocumentCarriesTheSameFigures(t *testing.T) {
	// A consumer trending these numbers is exactly who notices the denominator drifting.
	p := writeTemp(t, "access.log", strings.Repeat(nginxLine+"\n", 5))
	code, stdout, stderr := runCLI("advise", "--json", "--log", p)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, stderr)
	}
	var doc struct {
		Input *struct {
			Format  string `json:"format"`
			Lines   int64  `json:"lines"`
			Read    int64  `json:"read"`
			Skipped int64  `json:"skipped"`
		} `json:"input"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if doc.Input == nil {
		t.Fatal("the document must carry what was parsed")
	}
	if doc.Input.Format != "text" || doc.Input.Lines != 5 || doc.Input.Read != 5 || doc.Input.Skipped != 0 {
		t.Errorf("input = %+v, expected text 5/5/0", *doc.Input)
	}
}

func TestUACountsCarryNoParseSummary(t *testing.T) {
	// ⚠️ Same rule as Evidence: a `uniq -c` count has no lines to report on, and inventing a
	// summary for it would be the kind of confident-looking number this milestone is about.
	counts := writeTemp(t, "ua.txt", "10 "+jsonUA+"\n")
	code, stdout, stderr := runCLI("advise", "--json", "--ua-counts", counts)
	if code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, stderr)
	}
	if strings.Contains(stdout, `"input"`) {
		t.Errorf("a count file has no parse summary to carry:\n%s", stdout)
	}
}

func TestTheWarningNamesARemedyForTheFormatActuallyRead(t *testing.T) {
	// ⚠️ Telling somebody running Caddy to edit their HAProxy config is advice they cannot act on,
	// and a warning nobody can act on becomes noise — the same argument as a red build nobody can
	// fix.
	jsonNoUA := strings.Repeat(`{"ClientIP":"1.2.3.4","ClientRequestPath":"/a"}`+"\n", 10)
	_, _, warns, err := readLogWithSummary("-", strings.NewReader(jsonNoUA))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %q, expected one", warns)
	}
	if strings.Contains(warns[0], "HAProxy") {
		t.Errorf("a JSON log was told to fix its HAProxy config:\n%s", warns[0])
	}
	if !strings.Contains(warns[0], "ClientRequestUserAgent") {
		t.Errorf("the remedy must name the keys we do look under:\n%s", warns[0])
	}

	haproxy := strings.Repeat(`Sep  1 10:00:00 lb haproxy[1]: 1.2.3.4:1 [01/Sep/2026:10:00:00.123] fe be/s1 0/0/1/2/3 200 12 - - ---- 1/1/1/1/0 0/0 "GET /a HTTP/1.1"`+"\n", 10)
	if _, _, warns, err = readLogWithSummary("-", strings.NewReader(haproxy)); err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "capture request header") {
		t.Errorf("a text log must still get the capture remedy, got %q", warns)
	}
}
