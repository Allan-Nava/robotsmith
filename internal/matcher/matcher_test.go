package matcher

import "testing"

func TestCorrispondenzaPiuLunga(t *testing.T) {
	// Il caso che distingue la RFC da un parser a prima-corrispondenza: con `Allow: /` in testa,
	// un parser semplificato lascia passare /login. La RFC no, perché /login è più specifico.
	r := Parse("User-agent: *\nAllow: /\nDisallow: /login\n")
	if r.Allowed("Googlebot", "/login") {
		t.Error("/login deve essere bloccato: `Disallow: /login` è più lungo di `Allow: /`")
	}
	if !r.Allowed("Googlebot", "/") {
		t.Error("/ deve passare")
	}
}

func TestGruppoPiuSpecifico(t *testing.T) {
	body := "User-agent: *\nDisallow: /\n\nUser-agent: Googlebot\nAllow: /\n"
	r := Parse(body)
	if !r.Allowed("Googlebot/2.1", "/qualsiasi") {
		t.Error("Googlebot ha un gruppo suo: deve passare, non ereditare il Disallow di *")
	}
	if r.Allowed("PincoBot", "/qualsiasi") {
		t.Error("un UA senza gruppo proprio ricade su * e deve essere bloccato")
	}
}

func TestRegoleOrfaneDopoRigaVuota(t *testing.T) {
	// Il difetto trovato su www.streamwayplus.com: la riga vuota chiude il gruppo e le direttive
	// successive restano senza user-agent.
	r := Parse("User-agent: *\n\nDisallow: /login\n")
	if len(r.Orphans) != 1 {
		t.Fatalf("attesa 1 regola orfana, trovate %d", len(r.Orphans))
	}
	if !r.Allowed("Googlebot", "/login") {
		t.Error("una regola orfana non si applica: il tool la segnala, non la applica")
	}
}

func TestCommentiNonChiudonoIlGruppo(t *testing.T) {
	r := Parse("User-agent: *\n# commento\nDisallow: /login\n")
	if len(r.Orphans) != 0 {
		t.Error("un commento non deve chiudere il gruppo")
	}
	if r.Allowed("Googlebot", "/login") {
		t.Error("/login deve restare bloccato")
	}
}

func TestJolly(t *testing.T) {
	r := Parse("User-agent: *\nDisallow: /*.pdf$\nDisallow: /priv*/tmp\n")
	casi := map[string]bool{
		"/doc/a.pdf":     false, // combacia /*.pdf$
		"/doc/a.pdf?x=1": true,  // `$` ancora alla fine: la query string non combacia
		"/private/tmp":   false, // combacia /priv*/tmp
		"/public/tmp":    true,
	}
	for path, atteso := range casi {
		if got := r.Allowed("Googlebot", path); got != atteso {
			t.Errorf("%s: atteso allowed=%v, ottenuto %v", path, atteso, got)
		}
	}
}

func TestDisallowVuotoNonBlocca(t *testing.T) {
	r := Parse("User-agent: *\nDisallow:\n")
	if !r.Allowed("Googlebot", "/x") {
		t.Error("`Disallow:` senza valore significa nessun divieto")
	}
}

func TestFileVuotoPermetteTutto(t *testing.T) {
	r := Parse("")
	if !r.Allowed("YisouSpider", "/") {
		t.Error("un file vuoto (200 con 0 byte) equivale a tutto permesso")
	}
}
