package advise

import (
	"strings"
	"testing"
)

func advice(t *testing.T) *Advice {
	t.Helper()
	return Analyze([]Observation{
		{UA: "Mozilla/5.0 (compatible; Googlebot/2.1)", Requests: 2000},
		{UA: "Mozilla/5.0 (compatible; GPTBot/1.4)", Requests: 1000},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 7000},
	})
}

func TestDiffOnAFileThatAlreadySaysItIsEmpty(t *testing.T) {
	// The review is what decides whether the advice gets applied, so "nothing to do" has to be
	// visible at a glance instead of hidden in sixty identical lines.
	existing := "User-agent: Googlebot\nAllow: /\n\nUser-agent: GPTBot\nDisallow: /\n\n" +
		"User-agent: *\nDisallow: /admin/\n"
	changes := Diff(advice(t), existing, "example.com")
	for _, c := range changes {
		if c.Kind != Kept {
			t.Errorf("nothing should change, got %v on %s", c.Kind, c.Agent)
		}
	}
	if !AllKept(changes) {
		t.Error("AllKept must report an empty diff, so the CLI can say so in one line")
	}
}

func TestDiffNamesWhatWouldChangeAndWhy(t *testing.T) {
	existing := "User-agent: Googlebot\nAllow: /\n\nUser-agent: *\nDisallow: /admin/\n"
	changes := Diff(advice(t), existing, "example.com")
	byAgent := map[string]Change{}
	for _, c := range changes {
		byAgent[c.Agent] = c
	}
	if got := byAgent["GPTBot"]; got.Kind != Added || got.Directive != "Disallow: /" {
		t.Errorf("GPTBot = %+v, expected an added Disallow", got)
	}
	if byAgent["GPTBot"].Why == "" || byAgent["GPTBot"].Share == 0 {
		t.Errorf("an added block must carry its reason and its volume: %+v", byAgent["GPTBot"])
	}
	if got := byAgent["Googlebot"]; got.Kind != Kept {
		t.Errorf("Googlebot = %+v, expected Kept: it is already allowed", got)
	}
	if AllKept(changes) {
		t.Error("this diff is not empty")
	}
}

func TestDiffReportsADirectiveThatWouldFlip(t *testing.T) {
	// The dangerous case: the site currently blocks something this tool would let through (or the
	// other way round). It must be shown as a flip, not as an addition.
	existing := "User-agent: Googlebot\nDisallow: /\n\nUser-agent: *\nDisallow:\n"
	for _, c := range Diff(advice(t), existing, "example.com") {
		if c.Agent != "Googlebot" {
			continue
		}
		if c.Kind != Flipped {
			t.Errorf("Googlebot = %+v, expected Flipped", c)
		}
		if !strings.Contains(c.Was, "Disallow") || !strings.Contains(c.Directive, "Allow") {
			t.Errorf("a flip must show both sides: was %q, becomes %q", c.Was, c.Directive)
		}
	}
}

func TestHandWrittenGroupsSurviveAndAreReported(t *testing.T) {
	// ⚠️ The worst damage this tool can do is lose a rule a person wrote. A group for a crawler
	// the algorithm never saw in the logs (here YandexBot) must be carried over VERBATIM, and the
	// diff must say so — before this it was silently dropped.
	existing := "User-agent: YandexBot\nDisallow: /search\nDisallow: /cart\n\n" +
		"User-agent: *\nDisallow: /admin/\n"
	a := advice(t)

	out := Render(a, existing, "example.com")
	if !strings.Contains(out, "User-agent: YandexBot") || !strings.Contains(out, "Disallow: /search") {
		t.Errorf("a hand-written group must be preserved:\n%s", out)
	}
	if strings.Count(out, "User-agent: YandexBot") != 1 {
		t.Error("it must be carried over once, not duplicated")
	}

	var found bool
	for _, c := range Diff(a, existing, "example.com") {
		if c.Agent == "YandexBot" {
			found = true
			if c.Kind != Carried {
				t.Errorf("YandexBot = %+v, expected Carried (kept verbatim, not advised)", c)
			}
		}
	}
	if !found {
		t.Error("the diff must mention the group it carries over: silence is what loses rules")
	}
}

func TestRenderStillPutsTheStarGroupLast(t *testing.T) {
	// Carrying extra groups must not disturb the layout the linter checks: the `*` group last, no
	// blank line inside a group.
	out := Render(advice(t), "User-agent: YandexBot\nDisallow: /search\n\nUser-agent: *\nDisallow: /admin/\n", "example.com")
	if strings.LastIndex(out, "User-agent: *") < strings.Index(out, "User-agent: YandexBot") {
		t.Error("the `*` group must come after the specific ones")
	}
}
