package lint

import "testing"

func has(f []Finding, sub string) bool {
	for _, x := range f {
		if len(sub) > 0 && contains(x.Msg, sub) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestFileVuoto(t *testing.T) {
	if !has(Check("", "x.it"), "VUOTO") {
		t.Error("un file vuoto va segnalato come errore")
	}
}

func TestRegolaOrfana(t *testing.T) {
	f := Check("User-agent: *\n\nDisallow: /login\n", "x.it")
	if !has(f, "riga VUOTA") {
		t.Error("la regola dopo una riga vuota va segnalata")
	}
}

func TestAllowPrimaDeiDisallow(t *testing.T) {
	f := Check("User-agent: *\nAllow: /\nDisallow: /login\n", "x.it")
	if !has(f, "prima-corrispondenza") {
		t.Error("`Allow: /` in testa va segnalato")
	}
	// se sta in fondo, nessun avviso
	if len(Check("User-agent: *\nDisallow: /login\nAllow: /\n", "x.it")) != 0 {
		t.Error("`Allow: /` in fondo è corretto: nessun avviso")
	}
}

func TestSitemapAltroHost(t *testing.T) {
	if !has(Check("User-agent: *\nDisallow:\nSitemap: https://altro.es/s.xml\n", "mio.it"), "altro host") {
		t.Error("sitemap cross-domain va segnalata")
	}
}

func TestBloccaTutto(t *testing.T) {
	if !has(Check("User-agent: *\nDisallow: /\n", "x.it"), "esce dagli indici") {
		t.Error("Disallow: / per tutti è il difetto più costoso: va urlato")
	}
}
