package matcher

import (
	"strings"
	"testing"
)

func TestLongestMatchWins(t *testing.T) {
	// The case that separates the RFC from a first-match parser: with `Allow: /` first, a
	// simplified parser lets /login through. The RFC does not, because /login is more specific.
	r := Parse("User-agent: *\nAllow: /\nDisallow: /login\n")
	if r.Allowed("Googlebot", "/login") {
		t.Error("/login must be disallowed: `Disallow: /login` is longer than `Allow: /`")
	}
	if !r.Allowed("Googlebot", "/") {
		t.Error("/ must be allowed")
	}
}

func TestMostSpecificGroupApplies(t *testing.T) {
	body := "User-agent: *\nDisallow: /\n\nUser-agent: Googlebot\nAllow: /\n"
	r := Parse(body)
	if !r.Allowed("Googlebot/2.1", "/anything") {
		t.Error("Googlebot has its own group: it must pass, not inherit the * Disallow")
	}
	if r.Allowed("SomeBot", "/anything") {
		t.Error("a UA without its own group falls back to * and must be disallowed")
	}
}

func TestOrphanRulesAfterBlankLine(t *testing.T) {
	// The blank line closes the group and the directives after it are left without a user-agent.
	r := Parse("User-agent: *\n\nDisallow: /login\n")
	if len(r.Orphans) != 1 {
		t.Fatalf("expected 1 orphan rule, found %d", len(r.Orphans))
	}
	if !r.Allowed("Googlebot", "/login") {
		t.Error("an orphan rule does not apply: the tool reports it, it does not apply it")
	}
}

func TestCommentsDoNotCloseTheGroup(t *testing.T) {
	r := Parse("User-agent: *\n# a comment\nDisallow: /login\n")
	if len(r.Orphans) != 0 {
		t.Error("a comment must not close the group")
	}
	if r.Allowed("Googlebot", "/login") {
		t.Error("/login must stay disallowed")
	}
}

func TestWildcards(t *testing.T) {
	r := Parse("User-agent: *\nDisallow: /*.pdf$\nDisallow: /priv*/tmp\n")
	cases := map[string]bool{
		"/doc/a.pdf":     false, // matches /*.pdf$
		"/doc/a.pdf?x=1": true,  // `$` anchors the end: the query string does not match
		"/private/tmp":   false, // matches /priv*/tmp
		"/public/tmp":    true,
	}
	for path, want := range cases {
		if got := r.Allowed("Googlebot", path); got != want {
			t.Errorf("%s: expected allowed=%v, got %v", path, want, got)
		}
	}
}

func TestEmptyDisallowForbidsNothing(t *testing.T) {
	r := Parse("User-agent: *\nDisallow:\n")
	if !r.Allowed("Googlebot", "/x") {
		t.Error("`Disallow:` with no value means no prohibition")
	}
}

func TestEmptyFileAllowsEverything(t *testing.T) {
	r := Parse("")
	if !r.Allowed("YisouSpider", "/") {
		t.Error("an empty file (200 with 0 bytes) means everything is allowed")
	}
}

func TestParseKeepsTheOriginalSpellingOfAnAgent(t *testing.T) {
	// Matching is case-insensitive, so Agents is lowercased. But anything that writes a group back
	// out needs the spelling a person chose: rewriting "YandexBot" as "yandexbot" is equivalent to
	// a crawler and reads, to a human, like the tool mangled their file.
	r := Parse("User-agent: YandexBot\nUser-agent: Applebot-Extended\nDisallow: /search\n")
	if len(r.Groups) != 1 {
		t.Fatalf("expected one group, got %d", len(r.Groups))
	}
	g := r.Groups[0]
	if strings.Join(g.Agents, ",") != "yandexbot,applebot-extended" {
		t.Errorf("Agents = %v: matching needs them lowercased", g.Agents)
	}
	if strings.Join(g.AgentsRaw, ",") != "YandexBot,Applebot-Extended" {
		t.Errorf("AgentsRaw = %v: rendering needs the original spelling", g.AgentsRaw)
	}
}

func TestDecideNamesTheRuleThatDecided(t *testing.T) {
	// A verdict a reader cannot check is a verdict they have to trust. `check --expect` quotes the
	// deployed file's own line, and this is where that line comes from.
	body := "User-agent: *\nDisallow: /\n\nUser-agent: Googlebot\nAllow: /\nDisallow: /private\n"
	r := Parse(body)

	d := r.Decide("Mozilla/5.0 (compatible; Googlebot/2.1)", "/private")
	if d.Allowed {
		t.Error("/private is disallowed for Googlebot: the longer match wins")
	}
	if d.Agent != "Googlebot" {
		t.Errorf("agent = %q, expected the token as written in the file", d.Agent)
	}
	if d.Rule == nil || d.Rule.Pattern != "/private" || d.Rule.Line != 6 {
		t.Errorf("rule = %+v, expected `Disallow: /private` on line 6", d.Rule)
	}

	// Falling back to `*` must be visible: "your rule matched" and "you inherited the catch-all"
	// are different things to a person reading a report.
	d = r.Decide("SomeBot/1.0", "/anything")
	if d.Allowed || d.Agent != "*" || !d.ViaStar {
		t.Errorf("decision = %+v, expected the * group, marked as inherited", d)
	}

	// No applicable rule at all: allowed, and honest about there being nothing to quote.
	d = Parse("Sitemap: https://example.com/s.xml\n").Decide("Googlebot", "/")
	if !d.Allowed || d.Rule != nil || d.Agent != "" {
		t.Errorf("decision = %+v, expected allowed with nothing to quote", d)
	}
}

func TestDecideAgreesWithAllowed(t *testing.T) {
	// Two code paths answering the same question would drift: Decide is the one with the detail,
	// Allowed must be a thin wrapper over it.
	body := "User-agent: *\nAllow: /\nDisallow: /login\nDisallow: /*.pdf$\n"
	r := Parse(body)
	for _, path := range []string{"/", "/login", "/a.pdf", "/a.pdf?x=1", "/deep/login/x"} {
		if got, want := r.Decide("Googlebot", path).Allowed, r.Allowed("Googlebot", path); got != want {
			t.Errorf("%s: Decide says %v, Allowed says %v", path, got, want)
		}
	}
}
