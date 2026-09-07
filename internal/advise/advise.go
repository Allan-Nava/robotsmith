// Package advise holds the algorithm that ADVISES a robots.txt starting from observed traffic.
//
// The core idea: a robots.txt is not written on instinct, it is derived from data. For every
// crawler that actually reaches the site there is a single question — «does this traffic BRING me
// anything?» — and the answer determines the policy:
//
//	brings visits (search engines, link previews)    → ALLOW, always. Blocking them costs real traffic.
//	takes without giving (AI scrapers, SEO tools)    → BLOCK. This is where the file earns its keep.
//	triggered by a person (ChatGPT-User)             → ALLOW: blocking it costs visibility, not load.
//	not addressable (browsers, apps, monitors)       → IGNORE: robots.txt does not concern them.
//	unknown but high-volume                          → CANDIDATE, to be decided: "unknown" does not
//	                                                   mean "useless", and a wrong block is invisible.
//
// The order of the blocks is given by VOLUME: blocking one crawler worth 5% of the requests is
// worth more than blocking ten worth 0.1% each.
package advise

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Family is the classification of a user-agent.
type Family int

const (
	Unknown Family = iota
	Search         // search engines: they bring visits
	Social         // previews of shared links: they bring visits
	AIUser         // fetches triggered by a person
	AITrain        // scrapers collecting training data
	SEO            // third-party analysis tools
	Tool           // monitors, health checks, HTTP libraries, our own services
	App            // mobile/TV apps
	Browser        // browsers
)

func (f Family) String() string {
	return [...]string{"unknown", "search", "social", "ai-user", "ai-training", "seo", "tool", "app", "browser"}[f]
}

// Policy is the decision taken about a crawler.
type Policy int

const (
	Allow     Policy = iota // to be let through explicitly
	Block                   // to be blocked
	Candidate               // high volume but uncertain nature: a person decides
	Ignore                  // robots.txt does not concern it
)

// classification rule: the first matching pattern wins, so the order is significant.
type rule struct {
	re  *regexp.Regexp
	fam Family
	// name is the token to use in the robots.txt (`User-agent: <name>`), where one is needed.
	name string
}

var rules = []rule{
	// ⚠️ Allowlist first: "Googlebot" contains "bot", so a generic /bot/ rule placed at the top
	// would capture every one of them.
	{regexp.MustCompile(`googlebot`), Search, "Googlebot"},
	{regexp.MustCompile(`google-inspectiontool`), Search, "Google-InspectionTool"},
	{regexp.MustCompile(`storebot-google`), Search, "Storebot-Google"},
	{regexp.MustCompile(`bingbot|adidxbot`), Search, "bingbot"},
	{regexp.MustCompile(`duckduckbot`), Search, "DuckDuckBot"},
	{regexp.MustCompile(`yandex`), Search, "YandexBot"},
	{regexp.MustCompile(`baiduspider`), Search, "Baiduspider"},
	{regexp.MustCompile(`seznambot`), Search, "SeznamBot"},
	{regexp.MustCompile(`petalbot`), Search, "PetalBot"},
	{regexp.MustCompile(`applebot-extended`), AITrain, "Applebot-Extended"},
	{regexp.MustCompile(`applebot`), Search, "Applebot"},
	{regexp.MustCompile(`facebookexternalhit|facebookcatalog`), Social, "facebookexternalhit"},
	{regexp.MustCompile(`twitterbot`), Social, "Twitterbot"},
	{regexp.MustCompile(`linkedinbot`), Social, "LinkedInBot"},
	{regexp.MustCompile(`whatsapp`), Social, "WhatsApp"},
	{regexp.MustCompile(`telegrambot`), Social, "TelegramBot"},
	{regexp.MustCompile(`slackbot`), Social, "Slackbot"},
	{regexp.MustCompile(`discordbot`), Social, "Discordbot"},
	{regexp.MustCompile(`pinterest`), Social, "Pinterest"},
	{regexp.MustCompile(`redditbot`), Social, "redditbot"},
	{regexp.MustCompile(`embedly|skypeuripreview`), Social, "Embedly"},
	// User-triggered AI: BEFORE the training one, because their UA contains "openai".
	{regexp.MustCompile(`chatgpt-user`), AIUser, "ChatGPT-User"},
	{regexp.MustCompile(`oai-searchbot`), AIUser, "OAI-SearchBot"},
	{regexp.MustCompile(`gptbot`), AITrain, "GPTBot"},
	{regexp.MustCompile(`ccbot`), AITrain, "CCBot"},
	{regexp.MustCompile(`claudebot|claude-web|anthropic-ai`), AITrain, "ClaudeBot"},
	{regexp.MustCompile(`perplexity`), AITrain, "PerplexityBot"},
	{regexp.MustCompile(`bytespider`), AITrain, "Bytespider"},
	{regexp.MustCompile(`amazonbot`), AITrain, "Amazonbot"},
	{regexp.MustCompile(`meta-externalagent`), AITrain, "meta-externalagent"},
	{regexp.MustCompile(`google-extended`), AITrain, "Google-Extended"},
	{regexp.MustCompile(`youbot|diffbot|timpibot|omgili|imagesift`), AITrain, "YouBot"},
	{regexp.MustCompile(`semrush`), SEO, "SemrushBot"},
	{regexp.MustCompile(`ahrefs`), SEO, "AhrefsBot"},
	{regexp.MustCompile(`mj12bot`), SEO, "MJ12bot"},
	{regexp.MustCompile(`dotbot`), SEO, "DotBot"},
	{regexp.MustCompile(`blexbot`), SEO, "BLEXBot"},
	{regexp.MustCompile(`dataforseo`), SEO, "DataForSeoBot"},
	{regexp.MustCompile(`majestic|screaming|sistrix|serpstat|barkrowler`), SEO, "MajesticSEO"},
	// Not addressable through robots.txt.
	{regexp.MustCompile(`^node$|listmonk|kener|compress-bot|go-http|python|curl|wget|okhttp|` +
		`java/|health-?check|route53|prometheus|blackbox|zabbix|uptime-kuma|axios|deno/|supabase|` +
		`nagios|checkly|pingdom|uptimerobot`), Tool, ""},
	{regexp.MustCompile(`cfnetwork|darwin/|dalvik|exoplayer|networkingextension|roku|tizen|webos|` +
		`smart-?tv|appletv|tvos|crkey|firetv`), App, ""},
	// Last resort: whoever declares itself a crawler without being one of the known ones.
	{regexp.MustCompile(`bot|spider|crawl|slurp|scan|fetch`), Unknown, ""},
	{regexp.MustCompile(`mozilla`), Browser, ""},
}

// RuleInfo is one classification rule, in the shape `robotsmith crawlers` prints it. Exposing the
// table is what lets a person decide whether to trust the advice before running it on their logs —
// and what lets a test prove no rule has been shadowed by an earlier pattern.
type RuleInfo struct {
	Pattern string
	Family  Family
	Token   string
	Policy  Policy
	Why     string
}

// Rules returns the table in evaluation order: the first pattern that matches wins, so the order
// is part of the answer, not a detail.
func Rules() []RuleInfo {
	out := make([]RuleInfo, 0, len(rules))
	for _, r := range rules {
		pol, why := policyFor(r.fam, MinCandidateShare)
		if r.fam == Unknown {
			// ⚠️ The catch-all has no single policy: below the threshold it is ignored, above it a
			// person decides. Printing "ignore" would be a lie and "block" would be worse.
			pol = Candidate
			why = fmt.Sprintf("unrecognised: reviewed when it is worth %.1f%% or more of the requests, "+
				"ignored below that (a wrong block is invisible)", MinCandidateShare)
		}
		out = append(out, RuleInfo{Pattern: r.re.String(), Family: r.fam, Token: r.name,
			Policy: pol, Why: why})
	}
	return out
}

// Classify assigns a family and, where needed, the token to write in the file.
func Classify(ua string) (Family, string) {
	l := strings.ToLower(ua)
	for _, r := range rules {
		if r.re.MatchString(l) {
			name := r.name
			if name == "" && r.fam == Unknown {
				name = firstToken(ua)
			}
			return r.fam, name
		}
	}
	return Browser, ""
}

// firstToken extracts the crawler name out of an unknown UA (`YisouSpider` from
// "Mozilla/5.0 (compatible; YisouSpider/5.0; ...)"): it is what makes it writable in the file.
func firstToken(ua string) string {
	f := strings.FieldsFunc(ua, func(r rune) bool {
		return r == ' ' || r == ';' || r == '(' || r == ')' || r == '/' || r == ','
	})
	for _, t := range f {
		lt := strings.ToLower(t)
		if !strings.Contains(lt, "bot") && !strings.Contains(lt, "spider") && !strings.Contains(lt, "crawl") {
			continue
		}
		// ⚠️ Discard the GENERIC words: from "... crawler ..." we would derive the token `crawler`,
		// which matches no real crawler in a robots.txt and would produce a useless line.
		if generic[lt] || strings.HasPrefix(lt, "mozilla") {
			continue
		}
		return t
	}
	return ""
}

var generic = map[string]bool{
	"bot": true, "bots": true, "spider": true, "crawler": true, "crawl": true, "compatible": true,
	"robot": true, "webcrawler": true, "crawler.php": true,
}

// Observation is one crawler seen in the logs, with how much it weighs.
type Observation struct {
	UA       string
	Requests int64
	// Evidence is what the raw log said beyond the count. It is nil for a `uniq -c` input, which
	// cannot know paths or times — and claiming otherwise would be an invention.
	Evidence *Evidence
}

// Evidence is the shape of the traffic behind one user-agent: which paths, over how long. For an
// unknown crawler this is the difference between "0.4% and flat for a year" and "0.4% and
// tripling", which is the whole decision.
type Evidence struct {
	TopPaths    []PathCount
	First, Last time.Time
}

// PathCount is one path and how often it was asked for.
type PathCount struct {
	Path     string
	Requests int64
}

// merge folds another observation's evidence in, so two UA strings mapping to the same token do not
// lose half their traffic.
func (e *Evidence) merge(o *Evidence) {
	if o == nil {
		return
	}
	agg := map[string]int64{}
	for _, p := range append(append([]PathCount{}, e.TopPaths...), o.TopPaths...) {
		agg[p.Path] += p.Requests
	}
	e.TopPaths = TopPaths(agg, 5)
	if e.First.IsZero() || (!o.First.IsZero() && o.First.Before(e.First)) {
		e.First = o.First
	}
	if o.Last.After(e.Last) {
		e.Last = o.Last
	}
}

// TopPaths turns a path histogram into the few lines worth printing, heaviest first.
func TopPaths(counts map[string]int64, n int) []PathCount {
	out := make([]PathCount, 0, len(counts))
	for p, c := range counts {
		out = append(out, PathCount{Path: p, Requests: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		return out[i].Path < out[j].Path // stable output: a report has to be diffable
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Decision is the recommendation for a single crawler.
type Decision struct {
	Name     string
	UA       string
	Family   Family
	Policy   Policy
	Requests int64
	Share    float64 // share of the observed total, as a percentage
	Why      string
	// Evidence is present only when the input was a real log. The CLI shows it for the decisions a
	// person has to take (REVIEW), where the shape of the traffic is the argument.
	Evidence *Evidence
	// Rule is the pattern that decided this, and FromPolicy says whether it came from the site's
	// policy file or from the built-in table. A decision whose provenance is invisible cannot be
	// argued with.
	Rule       string
	FromPolicy bool
}

// Advice is the full result of the algorithm.
type Advice struct {
	Decisions []Decision
	// Saving is the share of requests the advised blocks would touch.
	Saving float64
	// Warnings are the cases robots.txt CANNOT solve.
	Warnings []string
	Total    int64
}

// MinCandidateShare is the share below which an unknown crawler is not worth a line in the file:
// blocking something worth 0.1% adds maintenance without removing load.
const MinCandidateShare = 0.5

// Override is a source of per-site answers — the parsed policy file. It is an interface so this
// package keeps knowing nothing about JSON or files (see internal/policy).
//
// It returns the family (Unknown meaning "not stated, keep the classification"), the policy, the
// reason written in the file, the pattern that matched, and whether anything matched at all.
type Override interface {
	Override(ua string) (Family, Policy, string, string, bool)
}

// Options are the analysis knobs that default to off.
type Options struct {
	// Override, when set, wins over the built-in table: that is the whole point of the file.
	Override Override
}

// Analyze applies the algorithm with no overrides — the call everything already makes.
func Analyze(obs []Observation) *Advice {
	return AnalyzeWith(obs, Options{})
}

// AnalyzeWith applies the algorithm, consulting the site's own policy first where it has one.
func AnalyzeWith(obs []Observation, opt Options) *Advice {
	a := &Advice{}
	agg := map[string]*Decision{}
	for _, o := range obs {
		a.Total += o.Requests
	}
	if a.Total == 0 {
		return a
	}
	// total share of traffic declaring a browser UA: used only to say how much of the traffic this
	// command CANNOT judge (see the closing warning)
	var browserShare float64

	for _, o := range obs {
		fam, name := Classify(o.UA)
		share := float64(o.Requests) / float64(a.Total) * 100
		if fam == Browser {
			browserShare += share
		}
		pol, why := policyFor(fam, share)
		rule, fromPolicy := "", false
		if opt.Override != nil {
			if ofam, opol, owhy, orule, ok := opt.Override.Override(o.UA); ok {
				// ⚠️ A stated family reclassifies; an unstated one (Unknown) leaves the built-in
				// classification alone and overrides only the policy.
				if ofam != Unknown {
					fam = ofam
				}
				pol, rule, fromPolicy = opol, orule, true
				why = owhy
				if why == "" {
					why = "set by the policy file (" + orule + ")"
				}
				// An overridden crawler is worth a line even when the table would have ignored it:
				// somebody wrote it down on purpose.
				if name == "" {
					name = firstToken(o.UA)
					if name == "" {
						name = orule
					}
				}
			}
		}
		if pol == Ignore {
			continue
		}
		if name == "" {
			continue // we would not know what to write in the file
		}
		key := strings.ToLower(name)
		if d, ok := agg[key]; ok {
			d.Requests += o.Requests
			if d.Evidence == nil {
				d.Evidence = o.Evidence
			} else {
				d.Evidence.merge(o.Evidence)
			}
			continue
		}
		agg[key] = &Decision{Name: name, UA: o.UA, Family: fam, Policy: pol, Requests: o.Requests,
			Why: why, Evidence: o.Evidence, Rule: rule, FromPolicy: fromPolicy}
	}
	for _, d := range agg {
		d.Share = float64(d.Requests) / float64(a.Total) * 100
		if d.Policy == Block || d.Policy == Candidate {
			a.Saving += d.Share
		}
		a.Decisions = append(a.Decisions, *d)
	}
	// order: heaviest decisions first, so the order of convenience is readable
	sort.Slice(a.Decisions, func(i, j int) bool {
		if a.Decisions[i].Policy != a.Decisions[j].Policy {
			return a.Decisions[i].Policy < a.Decisions[j].Policy
		}
		return a.Decisions[i].Requests > a.Decisions[j].Requests
	})
	// ⚠️ An honest warning, not an accusation: a browser UA with a high share is NORMAL (behind one
	// string there are thousands of people). The point is that disguised scrapers hide exactly
	// there, and telling them apart needs the PER-IP RATE — which this command does not look at.
	if browserShare > 0 {
		a.Warnings = append(a.Warnings, fmt.Sprintf(
			"%.1f%% of the traffic declares a browser User-Agent: this command cannot tell whether "+
				"there are people or disguised scrapers behind it, because it does not look at the "+
				"per-IP rate. Scrapers that lie about their UA do not show up in this list by "+
				"definition, and robots.txt does not stop them: that needs a request cap or a WAF",
			browserShare))
	}
	return a
}

func policyFor(fam Family, share float64) (Policy, string) {
	switch fam {
	case Search:
		return Allow, "brings visits: blocking it costs real traffic"
	case Social:
		return Allow, "previews of shared links: it brings visits"
	case AIUser:
		return Allow, "fetch triggered by a person: blocking it costs visibility, not load"
	case AITrain:
		return Block, "takes content for training without bringing visits"
	case SEO:
		return Block, "analyses the site on behalf of third parties"
	case Unknown:
		if share >= MinCandidateShare {
			return Candidate, fmt.Sprintf("unrecognised but heavy crawler (%.1f%%): to be reviewed", share)
		}
		return Ignore, ""
	default:
		return Ignore, ""
	}
}
