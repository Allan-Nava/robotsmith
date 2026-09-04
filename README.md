# robotsmith

Verifica un `robots.txt` — e **consiglia come scriverlo** partendo dal traffico che il sito riceve
davvero.

Nasce da un problema concreto: dei crawler stavano prendendo il 5% delle richieste di un sito senza
portare una visita, e la domanda «cosa metto nel robots.txt?» non aveva una risposta basata sui dati.
Le liste che si trovano in rete sono generiche; il traffico di un sito è specifico.

```
robotsmith check   esempio.it [--origin https://origin.interno/robots.txt]
robotsmith lint    ./robots.txt
robotsmith advise  --log access.log --current https://esempio.it/robots.txt --host esempio.it
```

## Perché non basta scrivere quattro righe a mano

Il rischio è **asimmetrico**. Bloccare uno scraper non produce nessun effetto visibile; bloccare per
sbaglio Googlebot fa uscire il sito dall'indice in poche settimane — e te ne accorgi quando il
traffico è già sparito. Un `robots.txt` va quindi verificato, e il file può essere **giusto e
comunque non funzionare**:

| Trappola | Cosa succede |
|---|---|
| File servito **200 con 0 byte** | non è «tutto vietato», è **tutto permesso** |
| Una **riga vuota** dentro un gruppo | chiude il record: tutte le regole dopo restano orfane e un parser stretto le ignora |
| `Allow: /` **prima** dei `Disallow` | con un parser a prima-corrispondenza annulla tutti i divieti |
| Modifica in **cache** sulla CDN | il file è corretto sull'origin e i crawler vedono ancora il vecchio |
| `Sitemap:` di un **altro host** | una sitemap cross-domain non viene considerata |

⚠️ Il **cache-buster non serve** a scoprire l'ultima: se la query string non fa parte della cache key
— configurazione normale per un file statico — anche `?cb=123` torna la copia vecchia. L'unico
confronto affidabile è **pubblico contro origin**, ed è cosa fa `--origin`.

## L'algoritmo che consiglia il file

`advise` non applica una lista preconfezionata: legge il traffico e decide caso per caso. Per ogni
crawler osservato si fa **una** domanda — *questo traffico mi porta qualcosa?* — e la risposta
determina la policy.

```
   log / conteggi UA
          │
          ▼
   ① CLASSIFICA          tabella ordinata di pattern → famiglia
          │              ⚠️ l'ordine conta: "Googlebot" contiene "bot"
          ▼
   ② APPLICA LA POLICY   per famiglia:
          │                motori, social      → ALLOW  (portano visite)
          │                ai-utente           → ALLOW  (fetch innescata da una persona)
          │                ai-training, seo    → BLOCK  (prendono senza dare)
          │                tool, app, browser  → IGNORA (il robots.txt non li riguarda)
          │                ignoto + voluminoso → VALUTA (decide una persona)
          ▼
   ③ ORDINA PER VOLUME   bloccare chi fa il 5% vale più di dieci che fanno lo 0,1%
          │
          ▼
   ④ GENERA IL FILE      allowlist esplicita, poi i blocchi, poi le REGOLE ESISTENTI invariate
          │              mai `Allow: /` prima dei divieti · mai righe vuote dentro un gruppo
          ▼
   ⑤ DICHIARA I LIMITI   cosa il robots.txt non può fermare, e quanto traffico non sa giudicare
```

Le scelte non ovvie, e il motivo:

- **`ChatGPT-User` e `OAI-SearchBot` non sono scraper di addestramento.** Il primo è una fetch
  innescata da una persona, il secondo alimenta i risultati di ricerca. Bloccarli toglie visibilità
  senza togliere carico, quindi la policy è **allow** anche se il nome sembra dire il contrario.
- **`Google-Extended` e `Applebot-Extended` non sono User-Agent**: esistono **solo** per il
  `robots.txt` (dicono «non usare i miei contenuti per addestrare»). Metterli in una regola su UA non
  fa nulla; qui vengono emessi nel file, dove hanno senso.
- **Un crawler ignoto non è automaticamente inutile.** Sopra una soglia di volume viene proposto
  **attivo** con il numero accanto (l'onere della prova si ribalta: costa troppo per ignorarlo);
  sotto, viene proposto **commentato**, perché un blocco sbagliato non si vede.
- **Le regole preesistenti si riportano invariate.** Sono state scritte per quel sito e l'algoritmo
  non sa perché: se nel file precedente c'erano regole **orfane** (dopo una riga vuota) vengono
  **recuperate** e segnalate, non perse.
- **L'avviso finale non accusa nessuno.** Uno User-Agent da browser con quota alta è normale: dietro
  una stringa ci sono migliaia di persone. Il tool dice solo che su quella fetta **non può
  pronunciarsi**, perché per distinguere una persona da uno scraper travestito serve il **tasso per
  IP** — che `advise` non guarda.

### Esempio, su dati reali

```
$ robotsmith advise --ua-counts ua.txt --current https://esempio.es/robots.txt --host esempio.es
Osservate 182761 richieste da 60 user-agent distinti.

Cosa consiglio e perché:
  PASSA    ChatGPT-User             4.49%  fetch innescata da una persona: bloccarla toglie visibilità, non carico
  PASSA    Googlebot                3.86%  porta visite: bloccarlo costa traffico reale
  BLOCCA   Bytespider               3.15%  prende contenuti per addestramento senza portare visite
  VALUTA   YisouSpider              6.27%  crawler non riconosciuto ma pesante (6.3%): da valutare

I blocchi consigliati toccano il 11.4% delle richieste osservate.

⚠️ il 16.0% del traffico dichiara uno User-Agent da browser: questo comando non può dire se dietro
   ci siano persone o scraper travestiti, perché non guarda il tasso per IP.
```

## Il matcher segue la RFC 9309, non la prima corrispondenza

Diverse implementazioni storiche (compresa quella nella stdlib di Python) applicano la **prima**
regola che combacia. La RFC 9309 — e Google — usano la corrispondenza **più lunga**, con l'`Allow`
che vince a pari lunghezza. Non è un dettaglio accademico:

```
User-agent: *
Allow: /
Disallow: /login
```

`/login` risulta **permesso** con la prima-corrispondenza e **bloccato** con la RFC. Un tool che
consiglia cosa scrivere deve modellare il comportamento dei crawler veri, quindi implementa la
seconda — e `lint` segnala comunque quella disposizione, perché non tutti i crawler sono conformi.

## Uso

```bash
go install github.com/Allan-Nava/robotsmith@latest

# verifica: il file c'è, è fresco, dice il giusto (31 casi)
robotsmith check esempio.it --origin https://origin.interno/robots.txt

# difetti strutturali di un file locale o remoto
robotsmith lint ./robots.txt

# consiglio dai log (HAProxy o nginx), preservando le regole attuali
robotsmith advise --log access.log --current https://esempio.it/robots.txt --host esempio.it --out robots.txt

# oppure da un conteggio già fatto
awk '{n=split($0,q,"\""); if(n>=5) print q[4]}' access.log | sort | uniq -c | sort -rn > ua.txt
robotsmith advise --ua-counts ua.txt
```

Uscite: **0** tutto come dovrebbe · **1** qualcosa non lo è · **2** errore d'uso · **4** file non
raggiungibile. Adatto a una pipeline CI: un `robots.txt` che perde regole o resta bloccato in cache
diventa una build rossa invece di una scoperta tardiva.

## ⛔ Cosa questo tool non fa

Non dice se i crawler **obbediscono**: `robots.txt` è una richiesta, non un controllo. Si misura dai
log nei giorni successivi. E contro gli scraper che si travestono da browser non può nulla **per
costruzione** — non c'è un token da scrivere: lì servono un tetto di richieste per IP o un WAF.

## Licenza

MIT.
