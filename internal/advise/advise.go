// Package advise contiene l'algoritmo che CONSIGLIA un robots.txt partendo dal traffico osservato.
//
// L'idea di fondo: un robots.txt non si scrive a intuito, si ricava dai dati. Per ogni crawler che
// arriva davvero sul sito ci si chiede una cosa sola — «questo traffico mi PORTA qualcosa?» — e la
// risposta determina la policy:
//
//	porta visite (motori, preview dei link)      → ALLOW, sempre. Bloccarli costa traffico reale.
//	prende senza dare (scraper AI, tool SEO)     → BLOCK. È il caso in cui il file serve.
//	è innescato da una persona (ChatGPT-User)    → ALLOW: bloccarlo toglie visibilità, non carico.
//	non è indirizzabile (browser, app, monitor)  → IGNORA: il robots.txt non li riguarda.
//	ignoto ma voluminoso                         → CANDIDATO, da decidere: "ignoto" non vuol dire
//	                                               "inutile", e un blocco sbagliato non si vede.
//
// L'ordine di convenienza fra i blocchi è dato dal VOLUME: bloccare un crawler che fa il 5% delle
// richieste vale più che bloccarne dieci che fanno lo 0,1% ciascuno.
package advise

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Family è la classificazione di uno user-agent.
type Family int

const (
	Unknown Family = iota
	Search         // motori di ricerca: portano visite
	Social         // preview dei link condivisi: portano visite
	AIUser         // fetch innescate da una persona
	AITrain        // scraper per addestramento
	SEO            // tool di analisi per terzi
	Tool           // monitoraggi, health-check, librerie HTTP, nostri servizi
	App            // app mobile/TV
	Browser        // navigatori
)

func (f Family) String() string {
	return [...]string{"ignoto", "motore", "social", "ai-utente", "ai-training", "seo", "tool", "app", "browser"}[f]
}

// Policy è la decisione sul crawler.
type Policy int

const (
	Allow     Policy = iota // da lasciare passare esplicitamente
	Block                   // da bloccare
	Candidate               // volume alto ma natura incerta: decide una persona
	Ignore                  // il robots.txt non lo riguarda
)

// regola di classificazione: il primo pattern che combacia vince, quindi l'ordine è significativo.
type rule struct {
	re  *regexp.Regexp
	fam Family
	// name è il token da usare nel robots.txt (`User-agent: <name>`), quando serve.
	name string
}

var rules = []rule{
	// ⚠️ Prima gli allowlist: "Googlebot" contiene "bot", quindi una regola generica su /bot/
	// messa in testa li catturerebbe tutti.
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
	// AI innescata da un utente: PRIMA di quella di training, perché il loro UA contiene "openai".
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
	// Non indirizzabili dal robots.txt.
	{regexp.MustCompile(`^node$|listmonk|kener|compress-bot|go-http|python|curl|wget|okhttp|` +
		`java/|health-?check|route53|prometheus|blackbox|zabbix|uptime-kuma|axios|deno/|supabase|` +
		`nagios|checkly|pingdom|uptimerobot`), Tool, ""},
	{regexp.MustCompile(`cfnetwork|darwin/|dalvik|exoplayer|networkingextension|roku|tizen|webos|` +
		`smart-?tv|appletv|tvos|crkey|firetv`), App, ""},
	// Ultima risorsa: chi si dichiara crawler senza essere fra i noti.
	{regexp.MustCompile(`bot|spider|crawl|slurp|scan|fetch`), Unknown, ""},
	{regexp.MustCompile(`mozilla`), Browser, ""},
}

// Classify assegna una famiglia e, dove serve, il token da scrivere nel file.
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

// firstToken estrae il nome del crawler da uno UA ignoto (`YisouSpider` da
// "Mozilla/5.0 (compatible; YisouSpider/5.0; ...)"): serve per poterlo scrivere nel robots.txt.
func firstToken(ua string) string {
	f := strings.FieldsFunc(ua, func(r rune) bool {
		return r == ' ' || r == ';' || r == '(' || r == ')' || r == '/' || r == ','
	})
	for _, t := range f {
		lt := strings.ToLower(t)
		if !strings.Contains(lt, "bot") && !strings.Contains(lt, "spider") && !strings.Contains(lt, "crawl") {
			continue
		}
		// ⚠️ Scartare le parole GENERICHE: da "... crawler ..." si ricaverebbe il token `crawler`,
		// che nel robots.txt non corrisponde a nessun crawler reale e darebbe una riga inutile.
		if generici[lt] || strings.HasPrefix(lt, "mozilla") {
			continue
		}
		return t
	}
	return ""
}

var generici = map[string]bool{
	"bot": true, "bots": true, "spider": true, "crawler": true, "crawl": true, "compatible": true,
	"robot": true, "webcrawler": true, "crawler.php": true,
}

// Observation è un crawler visto nei log, con quanto pesa.
type Observation struct {
	UA       string
	Requests int64
}

// Decision è la raccomandazione per un singolo crawler.
type Decision struct {
	Name     string
	UA       string
	Family   Family
	Policy   Policy
	Requests int64
	Share    float64 // quota sul totale osservato, in percentuale
	Why      string
}

// Advice è il risultato completo dell'algoritmo.
type Advice struct {
	Decisions []Decision
	// Saving è la quota di richieste che i blocchi consigliati toccherebbero.
	Saving float64
	// Warnings sono i casi che il robots.txt NON può risolvere.
	Warnings []string
	Total    int64
}

// MinCandidateShare è la quota sotto la quale un crawler ignoto non vale una riga nel file:
// bloccare qualcosa che fa lo 0,1% aggiunge manutenzione senza togliere carico.
const MinCandidateShare = 0.5

// Analyze applica l'algoritmo alle osservazioni.
func Analyze(obs []Observation) *Advice {
	a := &Advice{}
	agg := map[string]*Decision{}
	for _, o := range obs {
		a.Total += o.Requests
	}
	if a.Total == 0 {
		return a
	}
	// quota totale di traffico che dichiara uno UA da browser: serve solo a dire quanta parte del
	// traffico questo comando NON può giudicare (vedi il warning finale)
	var quotaBrowser float64

	for _, o := range obs {
		fam, name := Classify(o.UA)
		share := float64(o.Requests) / float64(a.Total) * 100
		if fam == Browser {
			quotaBrowser += share
		}
		pol, why := policyFor(fam, share)
		if pol == Ignore {
			continue
		}
		if name == "" {
			continue // non sapremmo cosa scrivere nel file
		}
		key := strings.ToLower(name)
		if d, ok := agg[key]; ok {
			d.Requests += o.Requests
			continue
		}
		agg[key] = &Decision{Name: name, UA: o.UA, Family: fam, Policy: pol, Requests: o.Requests, Why: why}
	}
	for _, d := range agg {
		d.Share = float64(d.Requests) / float64(a.Total) * 100
		if d.Policy == Block || d.Policy == Candidate {
			a.Saving += d.Share
		}
		a.Decisions = append(a.Decisions, *d)
	}
	// ordine: prima le decisioni che pesano più, così l'ordine di convenienza è leggibile
	sort.Slice(a.Decisions, func(i, j int) bool {
		if a.Decisions[i].Policy != a.Decisions[j].Policy {
			return a.Decisions[i].Policy < a.Decisions[j].Policy
		}
		return a.Decisions[i].Requests > a.Decisions[j].Requests
	})
	// ⚠️ Un avviso onesto, non un'accusa: uno UA da browser con quota alta è NORMALE (dietro una
	// stringa ci sono migliaia di persone). Il punto è che gli scraper travestiti si nascondono
	// proprio lì, e per distinguerli serve il TASSO PER IP — che questo comando non guarda.
	if quotaBrowser > 0 {
		a.Warnings = append(a.Warnings, fmt.Sprintf(
			"il %.1f%% del traffico dichiara uno User-Agent da browser: questo comando non può dire "+
				"se dietro ci siano persone o scraper travestiti, perché non guarda il tasso per IP. "+
				"Gli scraper che mentono sullo UA non compaiono in questa lista per definizione, e il "+
				"robots.txt non li ferma: servono un tetto di richieste o un WAF", quotaBrowser))
	}
	return a
}

func policyFor(fam Family, share float64) (Policy, string) {
	switch fam {
	case Search:
		return Allow, "porta visite: bloccarlo costa traffico reale"
	case Social:
		return Allow, "preview dei link condivisi: porta visite"
	case AIUser:
		return Allow, "fetch innescata da una persona: bloccarla toglie visibilità, non carico"
	case AITrain:
		return Block, "prende contenuti per addestramento senza portare visite"
	case SEO:
		return Block, "analizza il sito per conto di terzi"
	case Unknown:
		if share >= MinCandidateShare {
			return Candidate, fmt.Sprintf("crawler non riconosciuto ma pesante (%.1f%%): da valutare", share)
		}
		return Ignore, ""
	default:
		return Ignore, ""
	}
}

func short(s string) string {
	if len(s) > 42 {
		return s[:42] + "…"
	}
	return s
}
