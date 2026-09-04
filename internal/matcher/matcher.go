// Package matcher implementa la valutazione di un robots.txt secondo la RFC 9309.
//
// Perché non riusare una libreria: le implementazioni "storiche" (compresa quella della stdlib di
// Python) applicano la PRIMA regola che combacia. La RFC 9309 — e Google — usano invece la
// corrispondenza PIÙ LUNGA, con l'Allow che vince a pari lunghezza. La differenza non è teorica:
//
//	User-agent: *
//	Allow: /
//	Disallow: /login
//
// con la prima-corrispondenza `/login` risulta PERMESSO (vince `Allow: /`), con la RFC risulta
// BLOCCATO (`/login` è più lungo di `/`). Un tool che consiglia cosa scrivere deve modellare il
// comportamento reale dei crawler, non quello di un parser semplificato.
package matcher

import (
	"strings"
)

// Rule è una direttiva Allow/Disallow con il suo path pattern.
type Rule struct {
	Allow   bool
	Pattern string
	Line    int
}

// Group è un record del file: uno o più user-agent con le loro regole.
type Group struct {
	Agents []string
	Rules  []Rule
	// StartLine è la riga del primo `User-agent:` del gruppo (per i messaggi diagnostici).
	StartLine int
}

// RobotsTxt è un file parsato.
type RobotsTxt struct {
	Groups   []Group
	Sitemaps []string
	// Orphans sono le regole trovate FUORI da ogni gruppo: succede quando una riga vuota chiude il
	// record e le direttive successive restano senza `User-agent:`. Un parser stretto le ignora,
	// Google le tollera: il tool le segnala invece di scegliere per conto proprio.
	Orphans []Rule
}

// Parse legge un robots.txt. Volutamente tollerante come i crawler reali: ignora righe non
// riconosciute e non si ferma al primo errore.
func Parse(body string) *RobotsTxt {
	r := &RobotsTxt{}
	var cur *Group
	// inGroup dice se stiamo raccogliendo regole per un gruppo aperto. Una riga vuota lo chiude:
	// è il punto in cui nascono le regole orfane.
	inGroup := false

	for i, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		// ⚠️ L'ordine conta: una riga di solo COMMENTO non chiude il gruppo, una riga VUOTA sì.
		// Togliendo prima il commento le due diventerebbero indistinguibili (bug preso dal test
		// TestCommentiNonChiudonoIlGruppo).
		if line == "" {
			inGroup = false
			continue
		}
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue // era solo un commento: il gruppo resta aperto
		}
		key, val, ok := split(line)
		if !ok {
			continue
		}
		switch key {
		case "user-agent":
			if !inGroup || cur == nil {
				r.Groups = append(r.Groups, Group{StartLine: i + 1})
				cur = &r.Groups[len(r.Groups)-1]
				inGroup = true
			}
			cur.Agents = append(cur.Agents, strings.ToLower(val))
		case "allow", "disallow":
			rule := Rule{Allow: key == "allow", Pattern: val, Line: i + 1}
			if inGroup && cur != nil && len(cur.Rules) >= 0 && len(cur.Agents) > 0 {
				cur.Rules = append(cur.Rules, rule)
			} else {
				r.Orphans = append(r.Orphans, rule)
			}
		case "sitemap":
			r.Sitemaps = append(r.Sitemaps, val)
		}
	}
	return r
}

func split(line string) (key, val string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(line[:i])), strings.TrimSpace(line[i+1:]), true
}

// groupFor sceglie il gruppo applicabile a uno user-agent: quello con il token più SPECIFICO
// (match più lungo, case-insensitive, su sottostringa come fanno i crawler), con `*` come ultima
// risorsa. È la regola della RFC: un solo gruppo si applica, non l'unione di tutti.
func (r *RobotsTxt) groupFor(ua string) *Group {
	ua = strings.ToLower(ua)
	var best *Group
	bestLen := -1
	var star *Group
	for i := range r.Groups {
		for _, a := range r.Groups[i].Agents {
			if a == "*" {
				if star == nil {
					star = &r.Groups[i]
				}
				continue
			}
			if strings.Contains(ua, a) && len(a) > bestLen {
				best, bestLen = &r.Groups[i], len(a)
			}
		}
	}
	if best != nil {
		return best
	}
	return star
}

// Allowed dice se `path` è consentito a `ua`. Corrispondenza più lunga; a pari lunghezza vince
// Allow (RFC 9309 § 2.2.2). Nessuna regola applicabile ⇒ permesso.
func (r *RobotsTxt) Allowed(ua, path string) bool {
	g := r.groupFor(ua)
	if g == nil {
		return true
	}
	bestLen, allow := -1, true
	for _, rule := range g.Rules {
		if rule.Pattern == "" {
			continue // `Disallow:` vuoto significa "nessun divieto"
		}
		if !match(rule.Pattern, path) {
			continue
		}
		l := effLen(rule.Pattern)
		if l > bestLen || (l == bestLen && rule.Allow) {
			bestLen, allow = l, rule.Allow
		}
	}
	return allow
}

// effLen è la lunghezza "utile" del pattern per il confronto di specificità: i jolly non contano.
func effLen(p string) int {
	return len(strings.NewReplacer("*", "", "$", "").Replace(p))
}

// match applica il pattern con i jolly della RFC: `*` = qualsiasi sequenza, `$` = fine del path.
func match(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	if anchored {
		pattern = strings.TrimSuffix(pattern, "$")
	}
	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			if !strings.HasPrefix(path[pos:], part) {
				return false
			}
			pos += len(part)
			continue
		}
		k := strings.Index(path[pos:], part)
		if k < 0 {
			return false
		}
		pos += k + len(part)
	}
	if anchored {
		// con `$` l'ultimo pezzo deve arrivare in fondo
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			return pos == len(path)
		}
		return true
	}
	return true
}
