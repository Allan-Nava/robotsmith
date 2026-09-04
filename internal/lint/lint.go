// Package lint trova i difetti STRUTTURALI di un robots.txt: quelli che rendono il file diverso da
// come chi l'ha scritto pensa che sia. Sono la causa più comune di "l'ho messo e non funziona".
package lint

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Allan-Nava/robotsmith/internal/matcher"
)

// Severity distingue ciò che è rotto da ciò che è fragile.
type Severity int

const (
	Warn Severity = iota
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "ERRORE"
	}
	return "AVVISO"
}

// Finding è un difetto trovato.
type Finding struct {
	Sev  Severity
	Msg  string
	Line int
}

// Check analizza il contenuto. `host` è l'host da cui il file è stato scaricato (per la Sitemap).
func Check(body, host string) []Finding {
	var out []Finding
	r := matcher.Parse(body)

	if strings.TrimSpace(body) == "" {
		return append(out, Finding{Error, "file VUOTO: per un crawler equivale a «tutto permesso», " +
			"non a «tutto vietato»", 0})
	}
	if len(r.Groups) == 0 {
		out = append(out, Finding{Error, "nessun gruppo `User-agent:`: tutte le direttive sono ignorate", 0})
	}
	// 1) regole orfane: la causa numero uno di regole che "non si applicano"
	for _, o := range r.Orphans {
		verb := "Disallow"
		if o.Allow {
			verb = "Allow"
		}
		out = append(out, Finding{Error, fmt.Sprintf(
			"`%s: %s` è dopo una riga VUOTA dentro un gruppo: la riga vuota chiude il record, quindi "+
				"un parser stretto ignora questa regola (Google la rispetta comunque). Togli la riga vuota",
			verb, o.Pattern), o.Line})
	}
	// 2) `Allow: /` prima dei Disallow: innocuo con Google (vince il match più lungo), fatale con i
	//    parser a prima-corrispondenza
	for _, g := range r.Groups {
		for i, rule := range g.Rules {
			if !rule.Allow || rule.Pattern != "/" {
				continue
			}
			for _, dopo := range g.Rules[i+1:] {
				if !dopo.Allow {
					out = append(out, Finding{Warn, fmt.Sprintf(
						"`Allow: /` è PRIMA di `Disallow: %s`: con Google non cambia nulla (vince la "+
							"corrispondenza più lunga) ma un parser a prima-corrispondenza annulla tutti i "+
							"divieti. Spostalo in fondo al gruppo, o togli la riga: ciò che non è vietato "+
							"è già permesso", dopo.Pattern), rule.Line})
					break
				}
			}
			break
		}
	}
	// 3) Sitemap di un altro host
	for _, sm := range r.Sitemaps {
		if host == "" {
			continue
		}
		if u, err := url.Parse(sm); err == nil {
			a := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
			b := strings.TrimPrefix(strings.ToLower(host), "www.")
			if a != "" && a != b {
				out = append(out, Finding{Warn, fmt.Sprintf(
					"la `Sitemap:` punta a un altro host (%s): una sitemap cross-domain non viene "+
						"considerata", a), 0})
			}
		}
	}
	// 4) un gruppo che blocca tutto per tutti
	for _, g := range r.Groups {
		for _, ag := range g.Agents {
			if ag != "*" {
				continue
			}
			for _, rule := range g.Rules {
				if !rule.Allow && rule.Pattern == "/" {
					out = append(out, Finding{Error, "`User-agent: *` con `Disallow: /`: il sito " +
						"esce dagli indici. Se è voluto (ambiente di staging) va bene, altrimenti è " +
						"il difetto più costoso possibile", rule.Line})
				}
			}
		}
	}
	return out
}
