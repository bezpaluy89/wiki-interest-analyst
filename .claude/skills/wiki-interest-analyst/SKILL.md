---
name: wiki-interest-analyst
description: Analyze Wikipedia pageview trends to help decide which topics to build and which languages to launch in. Use when the user asks about interest in a topic over time, compares interest across languages/countries, wants to know if a trend is trustworthy, or needs a shareable one-page PDF report backed by Wikipedia data. Triggers on questions like "is interest in X growing", "compare interest in X across languages", "which language market should we enter", "how reliable is this trend". Also triggers on Ukrainian queries such as "чи зростає інтерес до X", "порівняй інтерес до X різними мовами", "який мовний ринок обрати наступним", "наскільки можна довіряти цьому тренду".
---

# Wikipedia Interest Analyst

A Go CLI that fetches Wikipedia pageview data, computes trend + **trust** metrics,
and renders a one-page PDF. You (the agent) only pick a subcommand and read the
compact JSON it prints — all data work happens in the binary.

## Setup (once per environment)

```bash
cd "$(dirname SKILL.md)"          # the skill directory
go build -o bin/wiki ./cmd/wiki   # ~10s; needs Go 1.21+, network for deps
```

If the build fails, run `go mod download` first. Do **not** commit `bin/`.
Responses are cached (default: OS cache dir), so repeat/refined queries are fast.

## The one command you usually want: `brief`

Resolves the topic to per-language articles, fetches data, analyzes it, prints
JSON, and writes a PDF + PNG — in a single call.

```bash
./bin/wiki brief --theme "інтервальне голодування" --langs pl,cs \
  --start 2022-01-01 --end 2024-12-31 --lang uk --out report.pdf
```

- `--theme` topic in natural language (any language).
- `--langs` comma-separated Wikipedia language codes to compare (`uk,pl,cs,en`, …).
- `--start` / `--end` `YYYY-MM-DD`. Data exists from **2015-07** onward.
- `--lang` PDF prose language: `uk` (default) or `en`.
- Optional: `--granularity monthly|daily` (default monthly), `--title "…"`,
  `--pivot-lang uk` (see Gotchas), `--titles "pl=Post,cs=Půst"` to override
  resolved article titles.

## Reading the JSON

Each series reports (all pre-computed — do not recompute):

- `direction`: **`growing` / `declining` / `stable`**. Only `growing`/`declining`
  when the trend is statistically significant; noisy data is `stable`.
- `growth_pct`, `cagr_pct`, `yoy`: magnitude of change.
- `median_views`: the baseline (low = noisy signal).
- `r2`, `p_value`, `significant`: how real the trend is.
- `spikes`, `spike_driven`: whether a "trend" is really one-off events.
- `trust` (`high`/`medium`/`low`) + `trust_reasons` (codes): **confidence in the
  stated `direction`.** Lead your answer with this. High-quality flat data → high
  trust in "stable"; a rising line from thin/spiky data → low trust in "growth".
- `comparison.leader` + `comparison.ranking`: languages ranked by growth.

When answering, state the direction, the trust level **and why** (from
`trust_reasons`), and name the key limitation. Never claim growth the data does
not support.

## Granular commands (for refinement / debugging)

- `./bin/wiki resolve --theme T --langs uk,pl` — see which article title was
  picked per language, and which are `unresolved`. Use to confirm the topic
  mapped correctly before trusting a comparison.
- `./bin/wiki fetch --project uk.wikipedia --article "Астрономія" --start .. --end ..`
  — one raw series.
- `./bin/wiki analyze --theme T --langs .. --start .. --end .. --json a.json`
  — metrics JSON only; `--json` also saves full data for `report`.
- `./bin/wiki report --input a.json --out report.pdf --lang uk` — render a PDF
  from a saved analysis (redraws the chart, no re-fetch).

## Gotchas

- **Pivot language.** The topic phrase is matched on some Wikipedia. Auto-detect
  works for most cases; if `resolve` finds the wrong article, pass
  `--pivot-lang` = the language the phrase is written in (usually the user's).
- **Unresolved languages** mean that language has no interwiki-linked article —
  report it honestly; do not substitute a different topic. Offer `--titles` to
  pin a title the user knows.
- **Interest ≠ willingness to pay.** Always relay the report's limitations.

For the full trust methodology (thresholds, formulas, when to distrust a result),
read `references/methodology.md`.
