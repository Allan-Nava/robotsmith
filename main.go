// robotsmith — verifies and ADVISES a site's robots.txt, starting from its real traffic.
//
//	robotsmith check   example.com [--origin https://internal.origin/robots.txt] [--json]
//	robotsmith lint    ./robots.txt [--strict] [--json]
//	robotsmith advise  --log access.log --current https://example.com/robots.txt [--json]
//
// Exits 0 if everything is as it should be, 1 if something is not, 2 on a usage error, 4 if the
// file is unreachable. Those four codes are a contract for the pipelines that call this binary.
package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Allan-Nava/robotsmith/internal/advise"
	"github.com/Allan-Nava/robotsmith/internal/check"
	"github.com/Allan-Nava/robotsmith/internal/lint"
	"github.com/Allan-Nava/robotsmith/internal/policy"
	"github.com/Allan-Nava/robotsmith/internal/report"
)

// exitCodes is the single source of truth for the contract other people's pipelines depend on.
// The usage text is generated from it and a test asserts every document repeats it — that is the
// drift the "31 cases" bug came from.
var exitCodes = []struct {
	code    int
	meaning string
}{
	{0, "everything as it should be"},
	{1, "something is not"},
	{2, "usage error"},
	{4, "file unreachable"},
}

func exitCodeLine() string {
	parts := make([]string, 0, len(exitCodes))
	for _, ec := range exitCodes {
		parts = append(parts, fmt.Sprintf("%d %s", ec.code, ec.meaning))
	}
	return "Exit codes: " + strings.Join(parts, " · ")
}

// version is overwritten at release time with `-ldflags "-X main.version=<tag>"`. A constant would
// make every development build claim to be a release, and a bug report impossible to tie to code.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole CLI: arguments in, exit code out, both streams injected. Keeping `main` down to
// one line is what makes the exit-code contract testable.
func run(args []string, out, errw io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(errw, usageText())
		return 2
	}
	switch args[0] {
	case "check":
		return cmdCheck(args[1:], out, errw)
	case "lint":
		return cmdLint(args[1:], out, errw)
	case "advise":
		return cmdAdvise(args[1:], out, errw)
	case "crawlers":
		return cmdCrawlers(args[1:], out, errw)
	case "version", "--version", "-v":
		fmt.Fprintln(out, "robotsmith", versionString(version, readBuildInfo()))
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(out, usageText())
		return 0
	default:
		fmt.Fprint(errw, usageText())
		return 2
	}
}

// readBuildInfo is a variable so a test can pin what the build stamped in.
var readBuildInfo = func() *debug.BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return info
}

// versionString spells out which binary is running. A release carries its tag; anything else
// carries the commit and whether the tree was dirty, which is what a bug report actually needs.
func versionString(v string, info *debug.BuildInfo) string {
	rev, dirty := "", false
	if info != nil {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		// `go install module@tag` stamps the tag in Main.Version and no vcs.* settings at all, so
		// that is the only case where Main.Version is worth trusting. ⚠️ A plain `go build` inside
		// the repo stamps a pseudo-version there instead (`v0.0.0-<date>-<hash>+dirty`), which would
		// repeat the commit and dress a development build as something installable.
		if v == "dev" && rev == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	switch {
	case rev != "" && dirty:
		return v + " (" + rev + ", dirty)"
	case rev != "":
		return v + " (" + rev + ")"
	default:
		return v
	}
}

func usageText() string {
	return `robotsmith ` + versionString(version, nil) + ` — verify and advise a robots.txt

  robotsmith check  <domain|url> [--origin <url>] [--path /] [--sitemaps] [--quiet]
                    [--format text|json|github]
      Verifies that the file is there, is FRESH and says the right thing (` +
		strconv.Itoa(len(check.MustPass)+len(check.MustBeBlocked)) + ` cases).
      --origin compares the public copy with the origin: it is the only reliable way
      to find out whether a CDN is still serving an old version.
      --sitemaps also asks every Sitemap: URL whether it answers (off by default:
      a verification must not make network calls nobody asked for).
      --expect takes the same policy file as "advise --policy" and verifies THIS
      site's decisions against what is served, quoting the file's own lines. It
      REPLACES the built-in cases: if you stated your policy, yours is the
      contract (otherwise a deliberate exception would be red forever).

  robotsmith lint   <file|url> [--strict] [--format text|json|github]
      Structural defects: empty file, orphan rules after a blank line,
      "Allow: /" before the Disallow rules, Sitemap on another host, Disallow: / for everyone.
      --strict makes warnings fail too, for a team that wants the file correct
      under a first-match parser as well.
      --format github emits GitHub Actions annotations on the offending lines,
      which is what the bundled action uses (--json is --format json).

  robotsmith advise --log <access.log|-> | --ua-counts <file> [--current <file|url>]
                    [--host <host>] [--out <file>] [--diff] [--policy <file.json>]
                    [--compare <previous.json>] [--json]
      ADVISES the file starting from the observed traffic, and explains why.
      --log accepts "-" for stdin and transparently reads gzipped logs, and is the
      only input that can show WHICH paths an unknown crawler asked for.
      --diff reviews what would change against --current instead of printing the
      whole file: reviewing is what decides whether the advice gets applied.
      --policy takes a JSON file with this site's own answers where it disagrees
      with the built-in table; the reasoning then says which rule of yours fired.
      --compare takes a stored --json document from an earlier run and reports
      what moved: a share is a snapshot, and the decision usually hinges on the
      direction. A small crawler growing fast is raised for review.
      --ua-counts accepts the output of "... | sort | uniq -c" (count + user-agent).

  robotsmith crawlers [--json]
      Prints the classification table in evaluation order: family, policy, the token
      written in the robots.txt, and why. It is the opinion this tool applies —
      readable before you run it on your logs.

  robotsmith version | help

` + exitCodeLine() + `
With --json the document goes to stdout and nothing else does; the verdict is unchanged.
`
}

// outputFormats are the shapes an answer can take. ⚠️ The format never changes the verdict: only
// how it is said. `--json` shipped first and stays as an alias, because pipelines pin it.
var outputFormats = []string{"text", "json", "github"}

func resolveFormat(format string, jsonAlias bool, errw io.Writer) (string, bool) {
	if jsonAlias && format == "text" {
		format = "json"
	}
	for _, f := range outputFormats {
		if format == f {
			return format, true
		}
	}
	// Falling back to text silently would hide a typo in a pipeline for as long as nobody reads it.
	fmt.Fprintf(errw, "unknown --format %q: valid values are %s\n", format, strings.Join(outputFormats, ", "))
	return "", false
}

// newFlagSet builds a flag set that reports usage errors instead of killing the process: the
// caller turns that into exit code 2, the documented "usage error".
func newFlagSet(name string, errw io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errw)
	return fs
}

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

// takesValue lists the flags that carry a value: the others are booleans.
//
// ⚠️ Adding a value-flag means adding it here too, or `reorderArgs` hands it the operand as its
// value — `lint robots.txt --format github` once became `--format robots.txt`.
// `TestEveryValueFlagIsKnownToTheArgumentReordering` walks the real flag sets and fails when this
// list falls behind.
func takesValue(f string) bool {
	f = strings.TrimLeft(f, "-")
	switch f {
	case "origin", "path", "log", "ua-counts", "current", "host", "out", "format", "policy", "expect", "compare", "report":
		return true
	}
	return false
}

// checkOpts, lintOpts and adviseOpts exist so flag registration lives in one place per command:
// the documentation gate walks these flag sets, so a new flag is documented or CI goes red.
type checkOpts struct {
	origin, path, format, expect string
	quiet, asJSON, sitemaps      bool
}

func checkFlags(errw io.Writer) (*flag.FlagSet, *checkOpts) {
	fs := newFlagSet("check", errw)
	o := &checkOpts{}
	fs.StringVar(&o.origin, "origin", "", "URL of the robots.txt on the origin")
	fs.StringVar(&o.path, "path", "/", "path to test")
	fs.BoolVar(&o.quiet, "quiet", false, "print the verdict only")
	fs.BoolVar(&o.sitemaps, "sitemaps", false, "also check that every Sitemap: URL answers (makes network calls)")
	fs.StringVar(&o.format, "format", "text", "output shape: text, json or github (workflow annotations)")
	fs.StringVar(&o.expect, "expect", "", "policy file whose decisions must hold in the served file")
	fs.BoolVar(&o.asJSON, "json", false, "shorthand for --format json")
	return fs, o
}

type crawlersOpts struct{ asJSON bool }

func crawlersFlags(errw io.Writer) (*flag.FlagSet, *crawlersOpts) {
	fs := newFlagSet("crawlers", errw)
	o := &crawlersOpts{}
	fs.BoolVar(&o.asJSON, "json", false, "emit a machine-readable document on stdout")
	return fs, o
}

type lintOpts struct {
	format         string
	strict, asJSON bool
}

func lintFlags(errw io.Writer) (*flag.FlagSet, *lintOpts) {
	fs := newFlagSet("lint", errw)
	o := &lintOpts{}
	fs.BoolVar(&o.strict, "strict", false, "make warnings fail too (exit 1)")
	fs.StringVar(&o.format, "format", "text", "output shape: text, json or github (workflow annotations)")
	fs.BoolVar(&o.asJSON, "json", false, "shorthand for --format json")
	return fs, o
}

type adviseOpts struct {
	logFile, uaCounts, current, host, outFile, policyFile, compare string
	asJSON, diff                                                   bool
}

func adviseFlags(errw io.Writer) (*flag.FlagSet, *adviseOpts) {
	fs := newFlagSet("advise", errw)
	o := &adviseOpts{}
	fs.StringVar(&o.logFile, "log", "", "access log (HAProxy or nginx), \"-\" for stdin, .gz supported")
	fs.StringVar(&o.uaCounts, "ua-counts", "", "file with `count user-agent` per line (the output of uniq -c)")
	fs.StringVar(&o.current, "current", "", "current robots.txt (file or URL): its rules are preserved")
	fs.StringVar(&o.host, "host", "", "host of the site, to validate the Sitemap line")
	fs.StringVar(&o.policyFile, "policy", "", "JSON policy file: this site's answer where it disagrees with the defaults")
	fs.StringVar(&o.compare, "compare", "", "a stored --json document from an earlier run: report what moved")
	fs.StringVar(&o.outFile, "out", "", "write the advised file here instead of on stdout")
	fs.BoolVar(&o.diff, "diff", false, "review what would change against --current, instead of printing the file")
	fs.BoolVar(&o.asJSON, "json", false, "emit a machine-readable document on stdout")
	return fs, o
}

func cmdCheck(args []string, out, errw io.Writer) int {
	fs, opt := checkFlags(errw)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprint(errw, usageText())
		return 2
	}
	format, ok := resolveFormat(opt.format, opt.asJSON, errw)
	if !ok {
		return 2
	}
	u, host, err := check.RobotsURL(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(errw, "invalid URL:", err)
		return 2
	}
	var expect []check.Expectation
	if opt.expect != "" {
		if expect, err = expectationsFrom(opt.expect, errw); err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintln(errw, "[4]", err)
				return 4
			}
			fmt.Fprintln(errw, err)
			return 2
		}
	}
	res, err := check.RunWith(check.Options{URL: u, Origin: opt.origin, Path: opt.path,
		Sitemaps: opt.sitemaps, Expect: expect, ExpectOnly: opt.expect != ""})
	if err != nil {
		fmt.Fprintln(errw, "[4]", err)
		return 4
	}
	findings := lint.Check(res.Body, host)

	switch format {
	case "json":
		if err := report.Write(out, report.FromCheck(res, opt.path, findings)); err != nil {
			fmt.Fprintln(errw, err)
			return 1
		}
		return verdict(len(res.Problems) == 0)
	case "github":
		for _, a := range report.Annotations(report.FromCheck(res, opt.path, findings)) {
			fmt.Fprintln(out, a)
		}
		if len(res.Problems) == 0 {
			fmt.Fprintf(out, "✅ OK — %d cases verified\n", res.Cases)
		}
		return verdict(len(res.Problems) == 0)
	}
	if !opt.quiet {
		fmt.Fprintf(out, "%s — %d lines, %d groups\n", u, strings.Count(res.Body, "\n")+1,
			strings.Count(strings.ToLower(res.Body), "user-agent:"))
		if v := res.Headers.Get("X-Cache"); v != "" {
			fmt.Fprintln(out, "cache:", v, res.Headers.Get("Age"))
		}
		for _, f := range findings {
			fmt.Fprintf(out, "  %s %s\n", f.Sev, f.Msg)
		}
		for _, sm := range res.Sitemaps {
			fmt.Fprintf(out, "  sitemap %s — HTTP %d, %s, %s\n", sm.URL, sm.Status, sm.ContentType,
				humanBytes(sm.Bytes))
		}
		for _, e := range res.Expected {
			mark, want := "✅", "allowed"
			if !e.Met {
				mark = "⛔"
			}
			if !e.Want {
				want = "blocked"
			}
			line := fmt.Sprintf("  %s %-24s must be %-7s — ", mark, e.UA, want)
			switch {
			case e.Quote != "":
				line += fmt.Sprintf("`%s` (line %d, group `%s`)", e.Quote, e.Line, e.Agent)
			default:
				line += "no rule in the file mentions it"
			}
			if e.ViaStar {
				line += ", inherited from `*`"
			}
			if e.Why != "" {
				line += "\n" + fmt.Sprintf("%29sbecause: %s", "", e.Why)
			}
			fmt.Fprintln(out, line)
		}
	}
	if len(res.Problems) == 0 {
		fmt.Fprintf(out, "✅ OK — %d cases verified\n", res.Cases)
		return 0
	}
	fmt.Fprintln(out, "⛔ NOT as expected:")
	for _, p := range res.Problems {
		fmt.Fprintln(out, "   •", p)
	}
	if res.Deindex {
		fmt.Fprintln(out, "\n⛔ A search engine is blocked: fix this now.")
	}
	return 1
}

// cmdCrawlers publishes the policy table. ⚠️ The order is the answer, not a presentation detail:
// the first matching pattern wins, so the listing is never sorted.
func cmdCrawlers(args []string, out, errw io.Writer) int {
	fs, opt := crawlersFlags(errw)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return 2
	}
	rules := advise.Rules()
	if opt.asJSON {
		if err := report.Write(out, report.FromRules(rules)); err != nil {
			fmt.Fprintln(errw, err)
			return 1
		}
		return 0
	}
	label := map[advise.Policy]string{
		advise.Allow: "ALLOW ", advise.Block: "BLOCK ", advise.Candidate: "REVIEW", advise.Ignore: "IGNORE",
	}
	fmt.Fprintf(out, "%d rules, in evaluation order — the FIRST match wins.\n\n", len(rules))
	for _, r := range rules {
		token := r.Token
		switch {
		case token != "":
		case r.Family == advise.Unknown:
			// An unknown crawler is addressable: the token is taken from its own UA when observed.
			token = "(derived from the UA)"
		default:
			// No token at all means robots.txt cannot address it (a browser, a monitor): saying so
			// is more useful than leaving the column blank.
			token = "— not addressable"
		}
		fmt.Fprintf(out, "%s %-11s %-22s %s\n", label[r.Policy], r.Family, token, r.Pattern)
		if r.Why != "" {
			fmt.Fprintf(out, "%13s%s\n", "", r.Why)
		}
	}
	return 0
}

func cmdLint(args []string, out, errw io.Writer) int {
	fs, opt := lintFlags(errw)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprint(errw, usageText())
		return 2
	}
	format, ok := resolveFormat(opt.format, opt.asJSON, errw)
	if !ok {
		return 2
	}
	src := fs.Arg(0)
	body, host, err := read(src)
	if err != nil {
		fmt.Fprintln(errw, "[4]", err)
		return 4
	}
	findings := lint.Check(body, host)

	switch format {
	case "json":
		if err := report.Write(out, report.FromLint(src, findings)); err != nil {
			fmt.Fprintln(errw, err)
			return 1
		}
		return lintVerdict(findings, opt.strict)
	case "github":
		for _, a := range report.Annotations(report.FromLint(src, findings)) {
			fmt.Fprintln(out, a)
		}
		if len(findings) == 0 {
			// Exit 0 with an empty stdout reads like a silent failure in a log.
			fmt.Fprintln(out, "✅ no structural defect")
		}
		return lintVerdict(findings, opt.strict)
	}
	if len(findings) == 0 {
		fmt.Fprintln(out, "✅ no structural defect")
		return 0
	}
	for _, x := range findings {
		pos := ""
		if x.Line > 0 {
			pos = fmt.Sprintf(" (line %d)", x.Line)
		}
		fmt.Fprintf(out, "%s%s: %s\n", x.Sev, pos, x.Msg)
	}
	return lintVerdict(findings, opt.strict)
}

// lintVerdict decides the exit code. By default only an ERROR fails: `Allow: /` in the wrong place
// is legal and works with Google, so failing on it by default would cry wolf. `--strict` is for a
// team that has decided the file must also be correct under a first-match parser.
func lintVerdict(findings []lint.Finding, strict bool) int {
	for _, f := range findings {
		if f.Sev == lint.Error || strict {
			return 1
		}
	}
	return 0
}

func verdict(ok bool) int {
	if ok {
		return 0
	}
	return 1
}

func cmdAdvise(args []string, out, errw io.Writer) int {
	fs, opt := adviseFlags(errw)
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return 2
	}

	// ⚠️ Read the policy BEFORE doing any work: falling back to the defaults on a broken file
	// would apply an opinion the site explicitly rejected, quietly.
	var override advise.Override
	if opt.policyFile != "" {
		b, err := os.ReadFile(opt.policyFile)
		if err != nil {
			fmt.Fprintln(errw, "[4]", err)
			return 4
		}
		p, err := policy.Parse(b, opt.policyFile)
		if err != nil {
			fmt.Fprintln(errw, err)
			return 2
		}
		override = p
	}

	var previous *advise.Snapshot
	if opt.compare != "" {
		b, err := os.ReadFile(opt.compare)
		if err != nil {
			fmt.Fprintln(errw, "[4]", err)
			return 4
		}
		if previous, err = report.SnapshotFrom(b, opt.compare); err != nil {
			fmt.Fprintln(errw, err)
			return 2
		}
	}

	var obs []advise.Observation
	var err error
	switch {
	case opt.uaCounts != "":
		obs, err = readCounts(opt.uaCounts)
	case opt.logFile != "":
		obs, err = readLog(opt.logFile, os.Stdin)
	default:
		fmt.Fprintln(errw, "either --log or --ua-counts is required")
		return 2
	}
	if err != nil {
		fmt.Fprintln(errw, "[4]", err)
		return 4
	}
	var existing string
	if opt.current != "" {
		existing, _, err = read(opt.current)
		if err != nil {
			fmt.Fprintln(errw, "warning: current robots.txt not readable:", err)
		}
	}

	a := advise.AnalyzeWith(obs, advise.Options{Override: override, Previous: previous})
	text := advise.Render(a, existing, opt.host)

	if opt.diff {
		return printDiff(out, errw, a, existing, opt.host)
	}
	if opt.asJSON {
		if err := report.Write(out, report.FromAdvise(a, text, len(obs))); err != nil {
			fmt.Fprintln(errw, err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(errw, "Observed %d requests from %d distinct user-agents.\n", a.Total, len(obs))
	fmt.Fprintln(errw, "\nWhat I advise, and why:")
	for _, d := range a.Decisions {
		label := map[advise.Policy]string{
			advise.Allow: "ALLOW   ", advise.Block: "BLOCK   ", advise.Candidate: "REVIEW  ",
		}[d.Policy]
		provenance := ""
		if d.FromPolicy {
			// Whose answer this is, and which line of theirs: a decision with invisible provenance
			// cannot be argued with.
			provenance = fmt.Sprintf(" [%s: %s]", opt.policyFile, d.Rule)
		}
		fmt.Fprintf(errw, "  %s %-22s %6.2f%%  %s%s\n", label, d.Name, d.Share, d.Why, provenance)
	}
	for _, d := range a.Decisions {
		// ⚠️ Only for the decisions a person has to take. For an ALLOW or a BLOCK the policy is the
		// answer; for a REVIEW the shape of the traffic IS the argument.
		if d.Policy != advise.Candidate || d.Evidence == nil {
			continue
		}
		fmt.Fprintf(errw, "\n  %s — what it asked for:\n", d.Name)
		for _, p := range d.Evidence.TopPaths {
			fmt.Fprintf(errw, "      %8d  %s\n", p.Requests, p.Path)
		}
		if span := d.Evidence.Last.Sub(d.Evidence.First); span > 0 {
			fmt.Fprintf(errw, "      spread over %s (%s → %s): a crawl, not a burst\n",
				span.Round(time.Minute), d.Evidence.First.Format("2006-01-02 15:04"),
				d.Evidence.Last.Format("15:04"))
		} else if !d.Evidence.First.IsZero() {
			fmt.Fprintf(errw, "      all within one timestamp (%s): a burst\n",
				d.Evidence.First.Format("2006-01-02 15:04"))
		}
	}
	if c := a.Comparison; c != nil {
		fmt.Fprintf(errw, "\nAgainst the previous run (%s requests):\n", thousandsSep(c.PreviousTotal))
		for _, m := range c.Movements {
			if m.Direction == advise.Steady {
				continue // a review shows what moved
			}
			line := fmt.Sprintf("  %-9s %-22s %5.2f%% → %5.2f%%", m.Direction, m.Name, m.Was, m.Now)
			if m.Factor > 0 {
				line += fmt.Sprintf("  (×%.3g)", m.Factor)
			}
			if m.Direction == advise.Vanished {
				line += "  — a rule still blocking it does nothing"
			}
			fmt.Fprintln(errw, line)
		}
	}
	if a.Saving > 0 {
		fmt.Fprintf(errw, "\nThe advised blocks touch %.1f%% of the observed requests.\n", a.Saving)
	}
	for _, w := range a.Warnings {
		fmt.Fprintln(errw, "\n⚠️ ", w)
	}
	if opt.outFile != "" {
		if err := os.WriteFile(opt.outFile, []byte(text), 0o644); err != nil {
			fmt.Fprintln(errw, err)
			return 1
		}
		fmt.Fprintln(errw, "\nwritten:", opt.outFile)
		return 0
	}
	fmt.Fprint(out, text)
	return 0
}

// printDiff answers the reviewer's question — what would applying this change? — instead of
// handing over a second sixty-line file to compare by eye. The asymmetric risk is the reason: a
// review that is hard does not happen, and the mistake it would have caught costs weeks of traffic.
func printDiff(out, errw io.Writer, a *advise.Advice, existing, host string) int {
	changes := advise.Diff(a, existing, host)
	if existing == "" {
		fmt.Fprintln(errw, "no --current given: everything below would be new.")
	}
	if advise.AllKept(changes) {
		fmt.Fprintln(out, "✅ Nothing to change: the current file already says what the traffic advises.")
		return 0
	}
	mark := map[advise.ChangeKind]string{
		advise.Added: "+", advise.Flipped: "!", advise.Kept: "=", advise.Carried: "~",
	}
	for _, c := range changes {
		if c.Kind == advise.Kept {
			continue // a review shows what moves, not what stands still
		}
		line := fmt.Sprintf("%s %-22s %-14s", mark[c.Kind], c.Agent, c.Directive)
		if c.Share > 0 {
			line += fmt.Sprintf("%6.2f%%  ", c.Share)
		} else {
			line += "         "
		}
		fmt.Fprintln(out, line+c.Why)
		if c.Kind == advise.Flipped {
			fmt.Fprintf(out, "%25s⚠️  the file currently says: %s\n", "", c.Was)
		}
	}
	fmt.Fprintln(out, "\n+ added   ! the file says the opposite   ~ kept verbatim (not advised here)")
	return 0
}

// expectationsFrom turns a policy file into the decisions to verify. `review` and `ignore` are not
// assertions — there is nothing to check — so only allow/block become expectations.
//
// ⚠️ The pattern doubles as the user-agent to test, which works for a literal token and cannot work
// for a real regex (`youbot|diffbot` is a fine matching rule and a nonsense UA). Those are skipped
// out loud: a confident verdict on a synthesised UA would be worse than no verdict.
func expectationsFrom(path string, errw io.Writer) ([]check.Expectation, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, err := policy.Parse(b, path)
	if err != nil {
		return nil, err
	}
	var out []check.Expectation
	for _, r := range p.Rules {
		switch r.Policy {
		case advise.Allow, advise.Block:
		default:
			continue
		}
		if strings.ContainsAny(r.Pattern, `|()[]{}*+?^$\`) {
			fmt.Fprintf(errw, "skipping %q: a regex cannot be used as a user-agent to test — "+
				"give the literal token you want verified\n", r.Pattern)
			continue
		}
		out = append(out, check.Expectation{UA: r.Pattern, Allow: r.Policy == advise.Allow, Why: r.Why})
	}
	return out, nil
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
	sc := scanner(f)
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

// readLog extracts the user-agents from an access log. `path` may be "-" for stdin, and a gzipped
// stream is decompressed transparently — rotated logs arrive gzipped and usually down a pipe, and
// decompressing gigabytes to disk first just to count user-agents is not a real option.
func readLog(path string, stdin io.Reader) ([]advise.Observation, error) {
	var src io.Reader
	if path == "-" {
		src = stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		src = f
	}
	src, err := maybeGunzip(src)
	if err != nil {
		return nil, err
	}

	count := map[string]int64{}
	paths := map[string]map[string]int64{}
	first := map[string]time.Time{}
	last := map[string]time.Time{}
	sc := scanner(src)
	for sc.Scan() {
		line := sc.Text()
		uas := userAgentsIn(line)
		if len(uas) == 0 {
			continue
		}
		path := pathIn(line)
		when := timeIn(line)
		for _, ua := range uas {
			count[ua]++
			if path != "" {
				if paths[ua] == nil {
					paths[ua] = map[string]int64{}
				}
				paths[ua][path]++
			}
			if when.IsZero() {
				continue
			}
			if f, ok := first[ua]; !ok || when.Before(f) {
				first[ua] = when
			}
			if when.After(last[ua]) {
				last[ua] = when
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	var obs []advise.Observation
	for ua, n := range count {
		o := advise.Observation{UA: ua, Requests: n}
		if len(paths[ua]) > 0 || !first[ua].IsZero() {
			o.Evidence = &advise.Evidence{
				TopPaths: advise.TopPaths(paths[ua], 5),
				First:    first[ua],
				Last:     last[ua],
			}
		}
		obs = append(obs, o)
	}
	sort.Slice(obs, func(i, j int) bool { return obs[i].Requests > obs[j].Requests })
	return obs, nil
}

// maybeGunzip sniffs the gzip magic bytes instead of trusting the file name: rotated logs are
// called `access.log.3.gz`, `access.log-20260904`, or nothing in particular, and a pipe has no name
// at all.
func maybeGunzip(r io.Reader) (io.Reader, error) {
	br := bufio.NewReaderSize(r, 1<<16)
	magic, err := br.Peek(2)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		return zr, nil
	}
	return br, nil
}

// userAgentsIn pulls the user-agent out of one log line. The heuristic is deliberately simple:
// take the quoted fields and drop the request line and the referer. It works with both HAProxy and
// nginx's `combined` format, which put the UA in different positions.
func userAgentsIn(line string) []string {
	var out []string
	parts := strings.Split(line, `"`)
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
		out = append(out, c)
	}
	return out
}

// pathIn pulls the requested path out of the quoted request line (`GET /news/a HTTP/1.1`). The
// query string is dropped: for judging a crawler, `/search?p=2` and `/search?p=3` are one place.
func pathIn(line string) string {
	parts := strings.Split(line, `"`)
	for i := 1; i < len(parts); i += 2 {
		if !isRequestLine(parts[i]) {
			continue
		}
		f := strings.Fields(parts[i])
		if len(f) < 2 {
			return ""
		}
		return strings.SplitN(f[1], "?", 2)[0]
	}
	return ""
}

// logTimeLayouts covers what the two log formats put between brackets. ⚠️ A format that carries no
// date at all is a reason to report less, never to fail: the user-agent counts must always work.
var logTimeLayouts = []string{
	"02/Jan/2006:15:04:05 -0700", // nginx combined
	"02/Jan/2006:15:04:05.000",   // HAProxy httplog
	"02/Jan/2006:15:04:05",
}

var reBracketed = regexp.MustCompile(`\[([^\]]+)\]`)

func timeIn(line string) time.Time {
	for _, m := range reBracketed.FindAllStringSubmatch(line, -1) {
		for _, layout := range logTimeLayouts {
			if t, err := time.Parse(layout, m[1]); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func isRequestLine(s string) bool {
	for _, m := range []string{"GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "PATCH "} {
		if strings.HasPrefix(s, m) {
			return true
		}
	}
	return false
}

// thousandsSep groups digits so a request count is readable at a glance.
func thousandsSep(n int64) string {
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

// humanBytes keeps a size readable at a glance: a sitemap is either a few KB or tens of MB, and
// the difference matters more than the exact number.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// scanner reads long lines without choking: a user-agent string can be absurdly long, and the
// default 64 KiB token limit would silently truncate the line it appears on.
func scanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	return sc
}
