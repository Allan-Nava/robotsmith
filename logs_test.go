package main

import (
	"bytes"
	"compress/gzip"
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
