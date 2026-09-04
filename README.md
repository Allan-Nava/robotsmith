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
go install github.com/Allan-Nava/robotsmith@latest

# verify: the file is there, it is fresh, it says the right thing (32 cases)
robotsmith check example.com --origin https://internal.origin/robots.txt

# structural defects of a local or remote file
robotsmith lint ./robots.txt

# advice from the logs (HAProxy or nginx), preserving the current rules
robotsmith advise --log access.log --current https://example.com/robots.txt --host example.com --out robots.txt

# or from a count you already have
awk '{n=split($0,q,"\""); if(n>=5) print q[4]}' access.log | sort | uniq -c | sort -rn > ua.txt
robotsmith advise --ua-counts ua.txt
```

Exit codes: **0** everything as it should be · **1** something is not · **2** usage error ·
**4** file unreachable. Fit for a CI pipeline: a `robots.txt` that loses rules or stays stuck in a
cache becomes a red build instead of a late discovery.

## ⛔ What this tool does not do

It does not tell you whether crawlers **obey**: `robots.txt` is a request, not a control. That is
measured from the logs over the following days. And against scrapers disguised as browsers it can do
nothing **by construction** — there is no token to write: that calls for a per-IP request cap or a
WAF.

## License

MIT.
