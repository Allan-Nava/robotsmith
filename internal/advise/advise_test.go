package advise

import "strings"
import "testing"

func TestOrdineDiClassificazione(t *testing.T) {
	// "Googlebot" contiene "bot": se la regola generica vincesse, lo bloccheremmo.
	casi := map[string]Family{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)": Search,
		"facebookexternalhit/1.1": Social,
		"Mozilla/5.0 AppleWebKit (compatible; ChatGPT-User/1.0; +openai.com/bot)": AIUser,
		"Mozilla/5.0 (compatible; GPTBot/1.4; +openai.com/gptbot)":                AITrain,
		"Mozilla/5.0 (compatible; SemrushBot/7~bl)":                               SEO,
		"Mozilla/5.0 (compatible; YisouSpider/5.0; +http://www.yisou.com)":        Unknown,
		"okhttp/5.4.0": Tool,
		"SevillaFC/1 CFNetwork/3860 Darwin/25.6.0":             App,
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/148": Browser,
		"Amazon-Route53-Health-Check-Service":                  Tool,
	}
	for ua, atteso := range casi {
		if fam, _ := Classify(ua); fam != atteso {
			t.Errorf("%.44s → %v, atteso %v", ua, fam, atteso)
		}
	}
}

func TestNomeEstrattoDaUAIgnoto(t *testing.T) {
	_, name := Classify("Mozilla/5.0 (compatible; YisouSpider/5.0; +http://www.yisou.com/help)")
	if name != "YisouSpider" {
		t.Errorf("atteso YisouSpider come token da scrivere nel file, ottenuto %q", name)
	}
}

func TestCrawlerIgnotoVoluminosoDiventaCandidato(t *testing.T) {
	a := Analyze([]Observation{
		{UA: "Mozilla/5.0 (compatible; YisouSpider/5.0)", Requests: 5000},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 95000},
	})
	var d *Decision
	for i := range a.Decisions {
		if a.Decisions[i].Name == "YisouSpider" {
			d = &a.Decisions[i]
		}
	}
	if d == nil {
		t.Fatal("YisouSpider deve comparire fra le decisioni")
	}
	if d.Policy != Candidate {
		t.Errorf("policy = %v, atteso Candidate (ignoto ma al 5%%)", d.Policy)
	}
	// al 5% supera AutoBlockShare, quindi nel file deve uscire ATTIVO (non commentato)
	out := Render(a, "", "esempio.it")
	if !strings.Contains(out, "\nUser-agent: YisouSpider\nDisallow: /\n") {
		t.Error("un candidato oltre il 3% va scritto attivo, con il numero accanto")
	}
}

func TestCandidatoPiccoloEsceCommentato(t *testing.T) {
	a := Analyze([]Observation{
		{UA: "Mozilla/5.0 (compatible; PincoSpider/1.0)", Requests: 700},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 99300},
	})
	out := Render(a, "", "esempio.it")
	if !strings.Contains(out, "# User-agent: PincoSpider") {
		t.Error("sotto il 3% il candidato va proposto COMMENTATO, non applicato d'ufficio")
	}
}

func TestRegoleEsistentiPreservateEOrfaneRecuperate(t *testing.T) {
	esistente := "User-agent: *\nDisallow: /admin/\n\nDisallow: /product/\n"
	a := Analyze([]Observation{{UA: "GPTBot/1.4", Requests: 100}})
	out := Render(a, esistente, "esempio.it")
	if !strings.Contains(out, "Disallow: /admin/") {
		t.Error("le regole esistenti devono essere riportate")
	}
	if !strings.Contains(out, "Disallow: /product/") {
		t.Error("le regole ORFANE del file precedente devono essere recuperate, non perse")
	}
	if !strings.Contains(out, "ORFANE") {
		t.Error("il recupero va segnalato a chi legge")
	}
}

func TestSitemapDiAltroHostScartata(t *testing.T) {
	esistente := "User-agent: *\nDisallow:\nSitemap: https://altrosito.es/sitemap.xml\n"
	out := Render(Analyze(nil), esistente, "esempio.it")
	if strings.Contains(out, "\nSitemap: https://altrosito.es") {
		t.Error("una sitemap cross-domain non va riportata")
	}
	if !strings.Contains(out, "Sitemap scartata") {
		t.Error("lo scarto va spiegato")
	}
}

func TestAvvisoDichiaraIlLimiteDelMetodo(t *testing.T) {
	// L'avviso non deve ACCUSARE gli UA da browser (una stringa comune vale per migliaia di
	// persone): deve dire che su quella fetta il metodo non può pronunciarsi.
	a := Analyze([]Observation{
		{UA: "Mozilla/5.0 (X11; Linux x86_64) Firefox/147.0", Requests: 30000},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 70000},
	})
	if len(a.Warnings) != 1 {
		t.Fatalf("atteso 1 avviso complessivo, trovati %d", len(a.Warnings))
	}
	if !strings.Contains(a.Warnings[0], "tasso per IP") {
		t.Error("l'avviso deve spiegare COSA manca per giudicare, non accusare")
	}
}

func TestTokenGenericoScartato(t *testing.T) {
	// Da "... crawler ..." non si deve ricavare il token `crawler`: nel robots.txt non blocca nulla.
	_, name := Classify("Mozilla/5.0 (compatible; some crawler; +http://example.com)")
	if name == "crawler" {
		t.Error("`crawler` è una parola generica, non il nome di un crawler")
	}
}
