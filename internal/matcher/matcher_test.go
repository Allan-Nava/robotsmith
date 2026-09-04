package matcher

import "testing"

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
