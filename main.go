// robotsmith — verifies and ADVISES a site's robots.txt, starting from its real traffic.
//
//	robotsmith check   example.com [--origin https://internal.origin/robots.txt]
//	robotsmith lint    ./robots.txt          (or a URL)
//	robotsmith advise  --log access.log --current https://example.com/robots.txt
//
// Exits 0 if everything is as it should be, 1 if something is not, 4 if the file is unreachable.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Allan-Nava/robotsmith/internal/advise"
	"github.com/Allan-Nava/robotsmith/internal/check"
	"github.com/Allan-Nava/robotsmith/internal/lint"
)

const version = "0.1.0"

// reorderArgs moves the operands (non-flags) to the end, so `check domain --quiet` behaves like
// `check --quiet domain`. The stdlib `flag` package stops at the first operand, which for a CLI is
// a needless stumble: nobody always writes the flags first.
func reorderArgs(args []string) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// a flag with a separate value (`--origin URL`) carries the next argument with it,
			// unless it was written as `--origin=URL`
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") &&
				takesValue(a) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		rest = append(rest, a)
	}
	return append(flags, rest...)
}

// takesValue lists the flags that take a value: the others are booleans.
func takesValue(f string) bool {
	f = strings.TrimLeft(f, "-")
	switch f {
	case "origin", "path", "log", "ua-counts", "current", "host", "out":
		return true
	}
	return false
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		os.Exit(cmdCheck(os.Args[2:]))
	case "lint":
		os.Exit(cmdLint(os.Args[2:]))
	case "advise":
		os.Exit(cmdAdvise(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("robotsmith", version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, usageText())
}

// usageText is separate from usage() so a test can assert what it claims. The number of verified
// cases comes from the tables in `check`: a literal here would silently lie the first time a
// crawler is added.
func usageText() string {
	return `robotsmith ` + version + ` — verify and advise a robots.txt

  robotsmith check  <domain|url> [--origin <url>] [--path /]
      Verifies that the file is there, is FRESH and says the right thing (` +
		strconv.Itoa(len(check.MustPass)+len(check.MustBeBlocked)) + ` cases).
      --origin compares the public copy with the origin: it is the only reliable way
      to find out whether a CDN is still serving an old version.

  robotsmith lint   <file|url>
      Structural defects: empty file, orphan rules after a blank line,
      "Allow: /" before the Disallow rules, Sitemap on another host, Disallow: / for everyone.

  robotsmith advise --log <access.log> | --ua-counts <file> [--current <file|url>] [--host <host>]
      ADVISES the file starting from the observed traffic, and explains why.
      --ua-counts accepts the output of "... | sort | uniq -c" (count + user-agent).
`
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	origin := fs.String("origin", "", "URL of the robots.txt on the origin")
	path := fs.String("path", "/", "path to test")
	quiet := fs.Bool("quiet", false, "print the verdict only")
	_ = fs.Parse(reorderArgs(args))
	if fs.NArg() < 1 {
		usage()
		return 2
	}
	u, host, err := check.RobotsURL(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid URL:", err)
		return 2
	}
	res, err := check.Run(u, *origin, *path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[4]", err)
		return 4
	}
	if !*quiet {
		fmt.Printf("%s — %d lines, %d groups\n", u, strings.Count(res.Body, "\n")+1,
			strings.Count(strings.ToLower(res.Body), "user-agent:"))
		if v := res.Headers.Get("X-Cache"); v != "" {
			fmt.Println("cache:", v, res.Headers.Get("Age"))
		}
		for _, f := range lint.Check(res.Body, host) {
			fmt.Printf("  %s %s\n", f.Sev, f.Msg)
		}
	}
	if len(res.Problems) == 0 {
		fmt.Printf("✅ OK — %d cases verified\n", res.Cases)
		return 0
	}
	fmt.Println("⛔ NOT as expected:")
	for _, p := range res.Problems {
		fmt.Println("   •", p)
	}
	if res.Deindex {
		fmt.Println("\n⛔ A search engine is blocked: fix this now.")
	}
	return 1
}

func cmdLint(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}
	body, host, err := read(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "[4]", err)
		return 4
	}
	f := lint.Check(body, host)
	if len(f) == 0 {
		fmt.Println("✅ no structural defect")
		return 0
	}
	worst := 0
	for _, x := range f {
		pos := ""
		if x.Line > 0 {
			pos = fmt.Sprintf(" (line %d)", x.Line)
		}
		fmt.Printf("%s%s: %s\n", x.Sev, pos, x.Msg)
		if x.Sev == lint.Error {
			worst = 1
		}
	}
	return worst
}

func cmdAdvise(args []string) int {
	fs := flag.NewFlagSet("advise", flag.ExitOnError)
	logFile := fs.String("log", "", "access log (HAProxy or nginx): the user-agents are extracted from it")
	uaCounts := fs.String("ua-counts", "", "file with `count user-agent` per line (the output of uniq -c)")
	current := fs.String("current", "", "current robots.txt (file or URL): its rules are preserved")
	host := fs.String("host", "", "host of the site, to validate the Sitemap line")
	out := fs.String("out", "", "write the advised file here instead of on screen")
	_ = fs.Parse(reorderArgs(args))

	var obs []advise.Observation
	var err error
	switch {
	case *uaCounts != "":
		obs, err = readCounts(*uaCounts)
	case *logFile != "":
		obs, err = readLog(*logFile)
	default:
		fmt.Fprintln(os.Stderr, "either --log or --ua-counts is required")
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "[4]", err)
		return 4
	}
	var existing string
	if *current != "" {
		existing, _, err = read(*current)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning: current robots.txt not readable:", err)
		}
	}

	a := advise.Analyze(obs)
	fmt.Fprintf(os.Stderr, "Observed %d requests from %d distinct user-agents.\n", a.Total, len(obs))
	fmt.Fprintln(os.Stderr, "\nWhat I advise, and why:")
	for _, d := range a.Decisions {
		label := map[advise.Policy]string{
			advise.Allow: "ALLOW   ", advise.Block: "BLOCK   ", advise.Candidate: "REVIEW  ",
		}[d.Policy]
		fmt.Fprintf(os.Stderr, "  %s %-22s %6.2f%%  %s\n", label, d.Name, d.Share, d.Why)
	}
	if a.Saving > 0 {
		fmt.Fprintf(os.Stderr, "\nThe advised blocks touch %.1f%% of the observed requests.\n", a.Saving)
	}
	for _, w := range a.Warnings {
		fmt.Fprintln(os.Stderr, "\n⚠️ ", w)
	}
	text := advise.Render(a, existing, *host)
	if *out != "" {
		if err := os.WriteFile(*out, []byte(text), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Fprintln(os.Stderr, "\nwritten:", *out)
		return 0
	}
	fmt.Print(text)
	return 0
}

// read accepts a local path or a URL.
func read(src string) (body, host string, err error) {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		u, h, e := check.RobotsURL(src)
		if e != nil {
			return "", "", e
		}
		cl := &http.Client{Timeout: 20 * time.Second}
		resp, e := cl.Get(u)
		if e != nil {
			return "", h, e
		}
		defer resp.Body.Close()
		b, e := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if e != nil {
			return "", h, e
		}
		if resp.StatusCode != 200 {
			return "", h, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return string(b), h, nil
	}
	b, e := os.ReadFile(src)
	return string(b), "", e
}

var reCount = regexp.MustCompile(`^\s*(\d+)[\s\t]+(.+?)\s*$`)

func readCounts(path string) ([]advise.Observation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var obs []advise.Observation
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		m := reCount.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		obs = append(obs, advise.Observation{UA: m[2], Requests: n})
	}
	return obs, sc.Err()
}

// readLog extracts the user-agents from an access log. The heuristic is deliberately simple: take
// the quoted fields and drop the one starting with an HTTP method (the request line). It works with
// both HAProxy and nginx's `combined` format, which put the UA in different positions.
func readLog(path string) ([]advise.Observation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	count := map[string]int64{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), `"`)
		for i := 1; i < len(parts); i += 2 {
			c := strings.TrimSpace(parts[i])
			if c == "" || c == "-" {
				continue
			}
			if isRequestLine(c) {
				continue
			}
			if !strings.Contains(c, "/") && !strings.Contains(c, " ") {
				continue // a bare referer or a host: not a UA
			}
			if strings.HasPrefix(c, "http://") || strings.HasPrefix(c, "https://") {
				continue // referer
			}
			count[c]++
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	var obs []advise.Observation
	for ua, n := range count {
		obs = append(obs, advise.Observation{UA: ua, Requests: n})
	}
	sort.Slice(obs, func(i, j int) bool { return obs[i].Requests > obs[j].Requests })
	return obs, nil
}

func isRequestLine(s string) bool {
	for _, m := range []string{"GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "PATCH "} {
		if strings.HasPrefix(s, m) {
			return true
		}
	}
	return false
}
