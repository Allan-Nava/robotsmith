// Package check verifica un robots.txt servito in rete: che ci sia, che sia FRESCO e che dica quello
// che deve.
package check

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Allan-Nava/robotsmith/internal/matcher"
)

// Attesi sono i casi che un robots.txt sano deve superare: chi porta visite passa, chi prende senza
// dare è bloccato. Sono la parte "policy" del check, separata dal parser di proposito.
var (
	DevonoPassare = []string{
		"Googlebot", "Google-InspectionTool", "Storebot-Google", "bingbot", "DuckDuckBot", "Applebot",
		"facebookexternalhit", "Twitterbot", "LinkedInBot", "WhatsApp", "TelegramBot", "Slackbot",
		"Discordbot", "Pinterest", "ChatGPT-User", "OAI-SearchBot",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
	}
	DevonoEssereBloccati = []string{
		"YisouSpider", "GPTBot", "CCBot", "ClaudeBot", "anthropic-ai", "PerplexityBot", "Bytespider",
		"Amazonbot", "meta-externalagent", "SemrushBot", "AhrefsBot", "MJ12bot", "DotBot", "BLEXBot",
		"DataForSeoBot",
	}
)

// Result è l'esito della verifica.
type Result struct {
	URL      string
	Body     string
	Headers  http.Header
	Problemi []string
	Casi     int
	Falliti  int
	Deindex  bool // almeno un motore di ricerca è bloccato: è l'unico caso urgente
}

var client = &http.Client{Timeout: 20 * time.Second}

func fetch(u string) (string, http.Header, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "robotsmith/1.0 (+github.com/Allan-Nava/robotsmith)")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != 200 {
		return "", resp.Header, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return string(b), resp.Header, nil
}

// Run scarica e verifica. Se `origin` è indicato, confronta il pubblico con l'origin: è l'unico modo
// affidabile di capire se una CDN sta ancora servendo una versione vecchia.
//
// ⚠️ Il trucco del cache-buster (`?cb=<timestamp>`) NON basta: se la query string non fa parte della
// cache key — configurazione normale per un file statico — entrambe le fetch tornano la stessa copia
// e il confronto dice "aggiornato" mentre la CDN serve il vecchio.
func Run(pubURL, originURL string, path string) (*Result, error) {
	body, hdr, err := fetch(pubURL)
	if err != nil {
		return nil, fmt.Errorf("%s non raggiungibile: %w", pubURL, err)
	}
	r := &Result{URL: pubURL, Body: body, Headers: hdr}

	if originURL != "" {
		ob, _, oerr := fetch(originURL)
		switch {
		case oerr != nil:
			r.Problemi = append(r.Problemi, fmt.Sprintf("origin non raggiungibile: %v", oerr))
		case strings.TrimSpace(ob) != strings.TrimSpace(body):
			r.Problemi = append(r.Problemi, fmt.Sprintf(
				"la CDN serve una versione DIVERSA dall'origin (%d byte contro %d): la modifica c'è "+
					"sull'origin ma i crawler vedono ancora la vecchia. Serve invalidare %s",
				len(body), len(ob), path))
		}
	}

	rt := matcher.Parse(body)
	if path == "" {
		path = "/"
	}
	for _, ua := range DevonoPassare {
		r.Casi++
		if !rt.Allowed(ua, path) {
			r.Falliti++
			msg := fmt.Sprintf("`%s` è BLOCCATO ma deve passare", ua)
			if isMotore(ua) {
				msg += " — ⛔ RISCHIO DEINDICIZZAZIONE"
				r.Deindex = true
			}
			r.Problemi = append(r.Problemi, msg)
		}
	}
	for _, ua := range DevonoEssereBloccati {
		r.Casi++
		if rt.Allowed(ua, path) {
			r.Falliti++
			r.Problemi = append(r.Problemi, fmt.Sprintf("`%s` passa ma dovrebbe essere bloccato", ua))
		}
	}
	return r, nil
}

func isMotore(ua string) bool {
	l := strings.ToLower(ua)
	for _, m := range []string{"googlebot", "bingbot", "google-", "storebot", "duckduck", "applebot"} {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}

// RobotsURL normalizza un input tipo "esempio.it" o "https://esempio.it" in URL del robots.txt.
func RobotsURL(in string) (string, string, error) {
	if !strings.Contains(in, "://") {
		in = "https://" + in
	}
	u, err := url.Parse(in)
	if err != nil {
		return "", "", err
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/robots.txt"
	}
	return u.String(), u.Hostname(), nil
}
