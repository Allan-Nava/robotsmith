// robotsmith — verifica e CONSIGLIA il robots.txt di un sito, partendo dal traffico reale.
//
//	robotsmith check   esempio.it [--origin https://origin.interno/robots.txt]
//	robotsmith lint    ./robots.txt          (oppure un URL)
//	robotsmith advise  --log access.log --current https://esempio.it/robots.txt
//
// Esce 0 se tutto è come dovrebbe, 1 se qualcosa non lo è, 4 se il file non è raggiungibile.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Allan-Nava/robotsmith/internal/advise"
	"github.com/Allan-Nava/robotsmith/internal/check"
	"github.com/Allan-Nava/robotsmith/internal/lint"
)

const version = "0.1.0"

// riordina sposta gli operandi (non-flag) in fondo, così `check dominio --quiet` funziona come
// `check --quiet dominio`. Il pacchetto `flag` della stdlib si ferma al primo operando, e per un CLI
// e' un inciampo gratuito: nessuno scrive i flag sempre prima.
func riordina(args []string) []string {
	var flags, resto []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// un flag con valore separato (`--origin URL`) si porta dietro l'argomento successivo,
			// a meno che sia scritto come `--origin=URL`
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") &&
				vuoleValore(a) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		resto = append(resto, a)
	}
	return append(flags, resto...)
}

// vuoleValore elenca i flag che prendono un valore: gli altri sono booleani.
func vuoleValore(f string) bool {
	f = strings.TrimLeft(f, "-")
	switch f {
	case "origin", "path", "log", "ua-counts", "current", "host", "out":
		return true
	}
	return false
}

func main() {
	if len(os.Args) < 2 {
		uso()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		os.Exit(cmdCheck(os.Args[2:]))
	case "lint":
		os.Exit(cmdLint(os.Args[2:]))
	case "advise":
		os.Exit(cmdAdvise(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("robotsmith", version)
	default:
		uso()
		os.Exit(2)
	}
}

func uso() {
	fmt.Fprint(os.Stderr, `robotsmith `+version+` — verifica e consiglia il robots.txt

  robotsmith check  <dominio|url> [--origin <url>] [--path /]
      Verifica che il file ci sia, sia FRESCO e dica il giusto (31 casi).
      --origin confronta il pubblico con l'origin: e' l'unico modo affidabile di
      scoprire se una CDN sta ancora servendo una versione vecchia.

  robotsmith lint   <file|url>
      Difetti strutturali: file vuoto, regole orfane dopo una riga vuota,
      "Allow: /" prima dei Disallow, Sitemap di un altro host, Disallow: / per tutti.

  robotsmith advise --log <access.log> | --ua-counts <file> [--current <file|url>] [--host <host>]
      CONSIGLIA il file partendo dal traffico osservato, e spiega perche'.
      --ua-counts accetta l'uscita di "... | sort | uniq -c" (conteggio + user-agent).
`)
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	origin := fs.String("origin", "", "URL del robots.txt sull'origin")
	path := fs.String("path", "/", "percorso da testare")
	quiet := fs.Bool("quiet", false, "stampa solo il verdetto")
	_ = fs.Parse(riordina(args))
	if fs.NArg() < 1 {
		uso()
		return 2
	}
	u, host, err := check.RobotsURL(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "URL non valido:", err)
		return 2
	}
	res, err := check.Run(u, *origin, *path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[4]", err)
		return 4
	}
	if !*quiet {
		fmt.Printf("%s — %d righe, %d gruppi\n", u, strings.Count(res.Body, "\n")+1,
			strings.Count(strings.ToLower(res.Body), "user-agent:"))
		if v := res.Headers.Get("X-Cache"); v != "" {
			fmt.Println("cache:", v, res.Headers.Get("Age"))
		}
		for _, f := range lint.Check(res.Body, host) {
			fmt.Printf("  %s %s\n", f.Sev, f.Msg)
		}
	}
	if len(res.Problemi) == 0 {
		fmt.Printf("✅ OK — %d casi verificati\n", res.Casi)
		return 0
	}
	fmt.Println("⛔ NON come previsto:")
	for _, p := range res.Problemi {
		fmt.Println("   •", p)
	}
	if res.Deindex {
		fmt.Println("\n⛔ Un motore di ricerca è bloccato: da correggere subito.")
	}
	return 1
}

func cmdLint(args []string) int {
	if len(args) < 1 {
		uso()
		return 2
	}
	body, host, err := leggi(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "[4]", err)
		return 4
	}
	f := lint.Check(body, host)
	if len(f) == 0 {
		fmt.Println("✅ nessun difetto strutturale")
		return 0
	}
	peggio := 0
	for _, x := range f {
		pos := ""
		if x.Line > 0 {
			pos = fmt.Sprintf(" (riga %d)", x.Line)
		}
		fmt.Printf("%s%s: %s\n", x.Sev, pos, x.Msg)
		if x.Sev == lint.Error {
			peggio = 1
		}
	}
	return peggio
}

func cmdAdvise(args []string) int {
	fs := flag.NewFlagSet("advise", flag.ExitOnError)
	logFile := fs.String("log", "", "access log (HAProxy o nginx): gli user-agent vengono estratti")
	uaCounts := fs.String("ua-counts", "", "file con `conteggio user-agent` per riga (uscita di uniq -c)")
	current := fs.String("current", "", "robots.txt attuale (file o URL): le sue regole vengono preservate")
	host := fs.String("host", "", "host del sito, per validare la riga Sitemap")
	out := fs.String("out", "", "scrive il file consigliato qui invece che a schermo")
	_ = fs.Parse(riordina(args))

	var obs []advise.Observation
	var err error
	switch {
	case *uaCounts != "":
		obs, err = leggiConteggi(*uaCounts)
	case *logFile != "":
		obs, err = leggiLog(*logFile)
	default:
		fmt.Fprintln(os.Stderr, "serve --log oppure --ua-counts")
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "[4]", err)
		return 4
	}
	var attuale string
	if *current != "" {
		attuale, _, err = leggi(*current)
		if err != nil {
			fmt.Fprintln(os.Stderr, "attenzione: robots.txt attuale non leggibile:", err)
		}
	}

	a := advise.Analyze(obs)
	fmt.Fprintf(os.Stderr, "Osservate %d richieste da %d user-agent distinti.\n", a.Total, len(obs))
	fmt.Fprintln(os.Stderr, "\nCosa consiglio e perché:")
	for _, d := range a.Decisions {
		etichetta := map[advise.Policy]string{
			advise.Allow: "PASSA   ", advise.Block: "BLOCCA  ", advise.Candidate: "VALUTA  ",
		}[d.Policy]
		fmt.Fprintf(os.Stderr, "  %s %-22s %6.2f%%  %s\n", etichetta, d.Name, d.Share, d.Why)
	}
	if a.Saving > 0 {
		fmt.Fprintf(os.Stderr, "\nI blocchi consigliati toccano il %.1f%% delle richieste osservate.\n", a.Saving)
	}
	for _, w := range a.Warnings {
		fmt.Fprintln(os.Stderr, "\n⚠️ ", w)
	}
	testo := advise.Render(a, attuale, *host)
	if *out != "" {
		if err := os.WriteFile(*out, []byte(testo), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Fprintln(os.Stderr, "\nscritto:", *out)
		return 0
	}
	fmt.Print(testo)
	return 0
}

// leggi accetta un percorso locale o un URL.
func leggi(src string) (body, host string, err error) {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		u, h, e := check.RobotsURL(src)
		if e != nil {
			return "", "", e
		}
		cl := &http.Client{Timeout: 20 * time.Second}
		resp, e := cl.Get(u)
		if e != nil {
			return "", h, e
		}
		defer resp.Body.Close()
		b, e := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if e != nil {
			return "", h, e
		}
		if resp.StatusCode != 200 {
			return "", h, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return string(b), h, nil
	}
	b, e := os.ReadFile(src)
	return string(b), "", e
}

var reConteggio = regexp.MustCompile(`^\s*(\d+)[\s\t]+(.+?)\s*$`)

func leggiConteggi(path string) ([]advise.Observation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var obs []advise.Observation
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		m := reConteggio.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		obs = append(obs, advise.Observation{UA: m[2], Requests: n})
	}
	return obs, sc.Err()
}

// leggiLog estrae gli user-agent da un access log. Euristica volutamente semplice: si prendono i
// campi fra apici e si scarta quello che comincia con un metodo HTTP (la request line). Funziona
// sia con HAProxy sia con il formato `combined` di nginx, che mettono l'UA in posizioni diverse.
func leggiLog(path string) ([]advise.Observation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	conta := map[string]int64{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		parti := strings.Split(sc.Text(), `"`)
		for i := 1; i < len(parti); i += 2 {
			c := strings.TrimSpace(parti[i])
			if c == "" || c == "-" {
				continue
			}
			if inizioHTTP(c) {
				continue
			}
			if !strings.Contains(c, "/") && !strings.Contains(c, " ") {
				continue // un referer nudo o un host: non è un UA
			}
			if strings.HasPrefix(c, "http://") || strings.HasPrefix(c, "https://") {
				continue // referer
			}
			conta[c]++
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	var obs []advise.Observation
	for ua, n := range conta {
		obs = append(obs, advise.Observation{UA: ua, Requests: n})
	}
	sort.Slice(obs, func(i, j int) bool { return obs[i].Requests > obs[j].Requests })
	return obs, nil
}

func inizioHTTP(s string) bool {
	for _, m := range []string{"GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "PATCH "} {
		if strings.HasPrefix(s, m) {
			return true
		}
	}
	return false
}
