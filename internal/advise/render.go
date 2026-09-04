package advise

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Allan-Nava/robotsmith/internal/matcher"
)

// AutoBlockShare è la quota oltre la quale un crawler ignoto viene scritto ATTIVO invece che
// commentato. Sotto questa soglia l'onere della prova sta su chi blocca (il costo è piccolo, il
// rischio di togliere un canale sconosciuto no); sopra, il costo è tale che l'onere si ribalta e
// la riga va scritta — con il numero accanto, così la decisione è verificabile.
const AutoBlockShare = 3.0

// Render produce il contenuto del robots.txt consigliato.
//
// `existing` è il file attuale (può essere vuoto): le sue regole per `User-agent: *` vengono
// riportate INVARIATE, perché sono state scritte per quel sito e questo algoritmo non sa perché.
// `host` serve a decidere se conservare la riga `Sitemap:`: puntare alla sitemap di un altro
// dominio non serve a nulla e in Search Console è un segnale sbagliato.
func Render(a *Advice, existing, host string) string {
	var b strings.Builder
	cur := matcher.Parse(existing)

	b.WriteString("# robots.txt generato da robotsmith (github.com/Allan-Nava/robotsmith)\n")
	if a.Total > 0 {
		b.WriteString(fmt.Sprintf("# Basato su %s richieste osservate. I blocchi consigliati ne toccano il %.1f%%.\n",
			thousands(a.Total), a.Saving))
	}
	b.WriteString("#\n# ⚠️ robots.txt è una RICHIESTA, non un controllo: chi si traveste da browser lo ignora.\n")
	b.WriteString("#    Per quelli serve un tetto di richieste o un WAF.\n")

	// 1) Allowlist esplicita: non è tecnicamente necessaria (ciò che non è vietato è permesso), ma
	//    rende evidente a chi legge il file che quei crawler sono voluti — e previene il
	//    "blocchiamo tutto tranne due" che porta alla deindicizzazione.
	if hasPolicy(a, Allow) {
		b.WriteString("\n# ── Passano di proposito: portano visite ─────────────────────────────\n")
		for _, d := range a.Decisions {
			if d.Policy != Allow {
				continue
			}
			b.WriteString(fmt.Sprintf("# %s — %s\n", d.Family, d.Why))
			b.WriteString("User-agent: " + d.Name + "\n")
			b.WriteString("Allow: /\n")
		}
	}

	// 2) Blocchi, in ordine di convenienza (volume decrescente).
	if hasPolicy(a, Block) || hasPolicy(a, Candidate) {
		b.WriteString("\n# ── Bloccati: prendono contenuti senza portare visite ────────────────\n")
		for _, d := range a.Decisions {
			if d.Policy != Block && d.Policy != Candidate {
				continue
			}
			attivo := d.Policy == Block || d.Share >= AutoBlockShare
			pref := ""
			if !attivo {
				pref = "# "
				b.WriteString(fmt.Sprintf("# ⏳ DA CONFERMARE (%.1f%% delle richieste): togli il commento se non porta visite.\n", d.Share))
			} else if d.Policy == Candidate {
				b.WriteString(fmt.Sprintf("# ⚠️ Crawler non riconosciuto ma pesante: %.1f%% delle richieste (%s).\n",
					d.Share, thousands(d.Requests)))
			}
			b.WriteString(pref + "User-agent: " + d.Name + "\n")
			b.WriteString(pref + "Disallow: /\n")
		}
	}

	// 3) Il gruppo `*`: regole esistenti riportate come sono.
	b.WriteString("\n# ── Regole generali ─────────────────────────────────────────────────\n")
	star := starGroup(cur)
	if star == nil {
		b.WriteString("User-agent: *\n")
		b.WriteString("# Nessuna regola preesistente trovata: aggiungi qui i percorsi privati\n")
		b.WriteString("# (area amministrativa, API, flussi di login) — questo tool non li conosce.\n")
		b.WriteString("Disallow:\n")
	} else {
		b.WriteString("User-agent: *\n")
		for _, r := range star.Rules {
			verb := "Disallow: "
			if r.Allow {
				verb = "Allow: "
			}
			b.WriteString(verb + r.Pattern + "\n")
		}
		if len(cur.Orphans) > 0 {
			b.WriteString("# ⚠️ Le righe qui sotto erano ORFANE nel file precedente (dopo una riga vuota\n")
			b.WriteString("#    dentro il gruppo): un parser stretto le ignorava. Recuperate qui.\n")
			for _, r := range cur.Orphans {
				verb := "Disallow: "
				if r.Allow {
					verb = "Allow: "
				}
				b.WriteString(verb + r.Pattern + "\n")
			}
		}
	}

	// 4) Sitemap: solo se è dello stesso host.
	for _, sm := range cur.Sitemaps {
		if sameHost(sm, host) {
			b.WriteString("\nSitemap: " + sm + "\n")
		} else {
			b.WriteString("\n# ⚠️ Sitemap scartata: punta a un altro host (" + sm + ").\n")
			b.WriteString("#    Una sitemap cross-domain non viene considerata.\n")
		}
	}
	return b.String()
}

func hasPolicy(a *Advice, p Policy) bool {
	for _, d := range a.Decisions {
		if d.Policy == p {
			return true
		}
	}
	return false
}

func starGroup(r *matcher.RobotsTxt) *matcher.Group {
	for i := range r.Groups {
		for _, ag := range r.Groups[i].Agents {
			if ag == "*" {
				return &r.Groups[i]
			}
		}
	}
	return nil
}

func sameHost(sitemap, host string) bool {
	if host == "" {
		return true
	}
	u, err := url.Parse(sitemap)
	if err != nil {
		return false
	}
	a := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	b := strings.TrimPrefix(strings.ToLower(host), "www.")
	return a == b
}

func thousands(n int64) string {
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	return string(out)
}
