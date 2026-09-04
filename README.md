<p align="center">
  <picture>
    <source srcset="docs/logo-dark.svg" media="(prefers-color-scheme: dark)">
    <img src="docs/logo.svg" alt="robotsmith" width="300">
  </picture>
</p>

<p align="center">
  <a href="https://github.com/Allan-Nava/robotsmith/actions/workflows/ci.yml"><img src="https://github.com/Allan-Nava/robotsmith/actions/workflows/ci.yml/badge.svg" alt="ci"></a>
  <a href="https://allan-nava.github.io/robotsmith/"><img src="https://img.shields.io/badge/docs-github%20pages-14161a" alt="docs"></a>
  <img src="https://img.shields.io/badge/go-1.24-00ADD8" alt="go 1.24">
  <img src="https://img.shields.io/badge/dependencies-none-2ea043" alt="no dependencies">
</p>

# robotsmith

Verifies a `robots.txt` — and **advises how to write it** starting from the traffic the site
actually receives.

It comes out of a concrete problem: some crawlers were taking 5% of a site's requests without
bringing a single visit, and the question «what do I put in robots.txt?» had no data-backed answer.
The lists you find online are generic; a site's traffic is specific.

```
robotsmith check   example.com [--origin https://internal.origin/robots.txt]
robotsmith lint    ./robots.txt
robotsmith advise  --log access.log --current https://example.com/robots.txt --host example.com
```

📖 Full documentation: **https://allan-nava.github.io/robotsmith/** ·
🧭 Design rationale: [INTENT.md](INTENT.md) · 🛠️ Contributing: [CLAUDE.md](CLAUDE.md) ·
🤖 Coding agents: [AGENTS.md](AGENTS.md) · 🗺️ Roadmap: [BACKLOG.md](BACKLOG.md) ·
📓 Changes: [CHANGELOG.md](CHANGELOG.md)

Current: **0.2.0** — *Fit for a pipeline* ([milestone closed](https://github.com/Allan-Nava/robotsmith/milestone/1)).
Next: [v0.3.0 — Sharper advice](https://github.com/Allan-Nava/robotsmith/milestone/2) ·
[v0.4.0 — Your policy, verified continuously](https://github.com/Allan-Nava/robotsmith/milestone/3).

## Why writing four lines by hand is not enough

The risk is **asymmetric**. Blocking a scraper produces no visible effect; accidentally blocking
Googlebot drops the site out of the index within weeks — and you notice once the traffic is already
gone. So a `robots.txt` has to be verified, and the file can be **right and still not work**:

| Pitfall | What happens |
|---|---|
| File served **200 with 0 bytes** | that is not «everything forbidden», it is **everything allowed** |
| A **blank line** inside a group | it closes the record: every rule after it is orphaned and a strict parser ignores it |
| `Allow: /` **before** the `Disallow` rules | with a first-match parser it voids every prohibition |
| Change stuck in the **CDN cache** | the file is correct on the origin and crawlers still see the old one |
| `Sitemap:` on **another host** | a cross-domain sitemap is not considered |

⚠️ A **cache-buster does not help** you find that last one: if the query string is not part of the
cache key — the normal setup for a static file — even `?cb=123` returns the old copy. The only
reliable comparison is **public against origin**, which is what `--origin` does.

## The algorithm that advises the file

`advise` does not apply a canned list: it reads the traffic and decides case by case. For every
observed crawler it asks **one** question — *does this traffic bring me anything?* — and the answer
determines the policy.

```
   log / UA counts
          │
          ▼
   ① CLASSIFY            ordered table of pattern → family
          │              ⚠️ order matters: "Googlebot" contains "bot"
          ▼
   ② APPLY THE POLICY    per family:
          │                search, social      → ALLOW  (they bring visits)
          │                ai-user             → ALLOW  (fetch triggered by a person)
          │                ai-training, seo    → BLOCK  (they take without giving)
          │                tool, app, browser  → IGNORE (robots.txt does not concern them)
          │                unknown + heavy     → REVIEW (a person decides)
          ▼
   ③ SORT BY VOLUME      blocking the one doing 5% beats blocking ten doing 0.1%
          │
          ▼
   ④ GENERATE THE FILE   explicit allowlist, then the blocks, then the EXISTING RULES unchanged
          │              never `Allow: /` before the prohibitions · never blank lines inside a group
          ▼
   ⑤ STATE THE LIMITS    what robots.txt cannot stop, and how much traffic it cannot judge
```

The non-obvious choices, and why:

- **`ChatGPT-User` and `OAI-SearchBot` are not training scrapers.** The first is a fetch triggered
  by a person, the second feeds search results. Blocking them costs visibility without removing
  load, so the policy is **allow** even though the name suggests otherwise.
- **`Google-Extended` and `Applebot-Extended` are not User-Agents**: they exist **only** for
  `robots.txt` (they mean «do not use my content for training»). Putting them in a UA-based rule
  does nothing; here they are emitted into the file, where they mean something.
- **An unknown crawler is not automatically useless.** Above a volume threshold it is proposed
  **active** with the number next to it (the burden of proof flips: it costs too much to ignore);
  below it, it is proposed **commented out**, because a wrong block is invisible.
- **Pre-existing rules are carried over unchanged.** They were written for that site and the
  algorithm does not know why: if the previous file had **orphaned** rules (after a blank line)
  they are **recovered** and flagged, not lost.
- **The closing warning accuses nobody.** A browser User-Agent with a high share is normal: behind
  one string there are thousands of people. The tool only says that on that slice it **cannot reach
  a verdict**, because telling a person from a disguised scraper needs the **per-IP rate** — which
  `advise` does not look at.

### Example, on real data

```
$ robotsmith advise --ua-counts ua.txt --current https://example.com/robots.txt --host example.com
Observed 182761 requests from 60 distinct user-agents.

What I advise, and why:
  ALLOW    ChatGPT-User             4.49%  fetch triggered by a person: blocking it costs visibility, not load
  ALLOW    Googlebot                3.86%  brings visits: blocking it costs real traffic
  BLOCK    Bytespider               3.15%  takes content for training without bringing visits
  REVIEW   YisouSpider              6.27%  unrecognised but heavy crawler (6.3%): to be reviewed

The advised blocks touch 11.4% of the observed requests.

⚠️ 16.0% of the traffic declares a browser User-Agent: this command cannot tell whether there are
   people or disguised scrapers behind it, because it does not look at the per-IP rate.
```

## Longest match, not first match

The parser is written in-house on purpose. The "historical" implementations (including Python's own
stdlib `robotparser`) apply the **first** matching rule; RFC 9309 — and Google — use the **longest
match**, with `Allow` winning ties:

```
User-agent: *
Allow: /
Disallow: /login
```

`/login` comes out **allowed** under first-match and **disallowed** under the RFC. A tool that
advises what to write has to model how real crawlers behave, so it implements the second one — and
`lint` still flags that layout, because not every crawler is compliant.

## Usage

```bash
# Homebrew — macOS and Linuxbrew. Homebrew 6+ asks to trust a third-party tap the first
# time: brew trust --cask Allan-Nava/tap/robotsmith
brew install --cask Allan-Nava/tap/robotsmith

# Docker — scratch image, runs as nobody, ~7 MB
docker run --rm ghcr.io/allan-nava/robotsmith check example.com
docker run --rm -v "$PWD:/w:ro" ghcr.io/allan-nava/robotsmith lint /w/robots.txt

# Go
go install github.com/Allan-Nava/robotsmith@latest

# or a prebuilt binary (linux/darwin × amd64/arm64, with checksums) from the Releases page

# verify: the file is there, it is fresh, it says the right thing (32 cases)
robotsmith check example.com --origin https://internal.origin/robots.txt

# structural defects of a local or remote file
robotsmith lint ./robots.txt

# ...and does every Sitemap: line actually answer? (asks the network, so it is opt-in)
robotsmith check example.com --sitemaps

# what would change if I applied the advice? (the review, not a second file to eyeball)
robotsmith advise --log access.log --current https://example.com/robots.txt --diff

# the opinion this tool applies, readable before you trust it with your logs
robotsmith crawlers

# advice from the logs (HAProxy or nginx), preserving the current rules
robotsmith advise --log access.log --current https://example.com/robots.txt --host example.com --out robots.txt

# rotated logs, straight off the pipe — gzip is detected by content, not by file name
zcat access.log.*.gz | robotsmith advise --log - --host example.com

# only an ERROR fails by default; --strict fails on warnings too
robotsmith lint ./robots.txt --strict

# or from a count you already have
awk '{n=split($0,q,"\""); if(n>=5) print q[4]}' access.log | sort | uniq -c | sort -rn > ua.txt
robotsmith advise --ua-counts ua.txt
```

### Flags

<!-- flags:start -->
| Command | Flag | What it does |
|---|---|---|
| `check` | `--origin <url>` | compares the public copy with the origin — the only reliable way to catch a stale CDN copy |
| `check` | `--path <path>` | the path the expected cases are evaluated against (default `/`) |
| `check` | `--sitemaps` | also ask every `Sitemap:` URL whether it answers — off by default, because a verification must not make network calls nobody asked for |
| `check` | `--quiet` | print the verdict only |
| `lint` | `--strict` | make warnings fail too, for a file that must be correct under a first-match parser as well |
| `advise` | `--log <file\|->` | access log (HAProxy or nginx); `-` reads stdin and gzipped input is decompressed transparently |
| `advise` | `--ua-counts <file>` | a count already made: the output of `… \| sort \| uniq -c` |
| `advise` | `--current <file\|url>` | the current `robots.txt`, whose rules are preserved verbatim |
| `advise` | `--host <host>` | the site's host, used to validate the `Sitemap:` line |
| `advise` | `--out <file>` | write the advised file there instead of stdout |
| `advise` | `--diff` | review what would change against `--current`, instead of printing the whole file |
| all three | `--json` | emit a machine-readable document on stdout (see below) |
<!-- flags:end -->

The flag table above is checked against the binary by a test: a flag that exists and is not
documented — or documented and no longer exposed — fails the build.

### Machine-readable output

`--json` puts one document on stdout and nothing else there; the exit code is unchanged. Consumers
pin the `schema` field, which is the only stable promise (the prose is free to be reworded):

| Command | Schema | Carries |
|---|---|---|
| `check` | `robotsmith.check/1` | cases, failures, deindexing flag, problems, structural findings, cache headers, sitemap answers |
| `lint` | `robotsmith.lint/1` | findings with severity and line — what a CI annotation needs |
| `advise` | `robotsmith.advise/1` | decisions (family, policy, share, reason, and for a log input the paths and time span behind them), warnings **and** the advised file |
| `crawlers` | `robotsmith.crawlers/1` | the whole classification table in evaluation order |

```bash
robotsmith lint ./robots.txt --json | jq -r '.findings[] | "::error line=\(.line)::\(.message)"'
robotsmith advise --ua-counts ua.txt --json | jq -r '.decisions[] | select(.policy=="block") | .name'
```

A breaking change to a document bumps its schema number; it is never edited in place.

### Exit codes

**0** everything as it should be · **1** something is not · **2** usage error ·
**4** file unreachable. Fit for a CI pipeline: a `robots.txt` that loses rules or stays stuck in a
cache becomes a red build instead of a late discovery. The four codes live in one place in the code
and a test asserts every document repeats them.

## ⛔ What this tool does not do

It does not tell you whether crawlers **obey**: `robots.txt` is a request, not a control. That is
measured from the logs over the following days. And against scrapers disguised as browsers it can do
nothing **by construction** — there is no token to write: that calls for a per-IP request cap or a
WAF.

## License

MIT.
