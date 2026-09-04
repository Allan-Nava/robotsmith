package lint

import (
	"strings"
	"testing"
)

func has(f []Finding, sub string) bool {
	for _, x := range f {
		if strings.Contains(x.Msg, sub) {
			return true
		}
	}
	return false
}

func TestEmptyFile(t *testing.T) {
	if !has(Check("", "x.com"), "EMPTY") {
		t.Error("an empty file must be reported as an error")
	}
}

func TestOrphanRule(t *testing.T) {
	f := Check("User-agent: *\n\nDisallow: /login\n", "x.com")
	if !has(f, "BLANK line") {
		t.Error("a rule after a blank line must be reported")
	}
}

func TestAllowBeforeDisallow(t *testing.T) {
	f := Check("User-agent: *\nAllow: /\nDisallow: /login\n", "x.com")
	if !has(f, "first-match") {
		t.Error("`Allow: /` placed first must be reported")
	}
	// at the end of the group it is correct: no warning
	if len(Check("User-agent: *\nDisallow: /login\nAllow: /\n", "x.com")) != 0 {
		t.Error("`Allow: /` last is correct: no warning expected")
	}
}

func TestSitemapOnAnotherHost(t *testing.T) {
	if !has(Check("User-agent: *\nDisallow:\nSitemap: https://other.example/s.xml\n", "mine.com"), "another host") {
		t.Error("a cross-domain sitemap must be reported")
	}
}

func TestDisallowEverything(t *testing.T) {
	if !has(Check("User-agent: *\nDisallow: /\n", "x.com"), "drops out of the search indexes") {
		t.Error("`Disallow: /` for everyone is the costliest defect: it must be shouted")
	}
}

func TestSeverityNames(t *testing.T) {
	// The severity is printed verbatim by the CLI: it is part of the output contract.
	if Error.String() != "ERROR" || Warn.String() != "WARNING" {
		t.Errorf("severity names = %q / %q", Error.String(), Warn.String())
	}
}
