# Methodology: how trends and trust are computed

This file explains what the `analyze` package computes and why. Read it when a
user questions a result, asks how trust is derived, or wants to tune thresholds.

## Data source & hygiene

- **Wikimedia Pageviews REST API** (`per-article`), data from **2015-07-01**,
  ~1 day lag. Free, no key; a descriptive `User-Agent` is required.
- Default `agent=user` excludes known bots/spiders/automated traffic — the
  honest proxy for human interest. It cannot remove *all* automation.
- Default `access=all-access` (desktop + mobile).
- Missing buckets in the requested range are tracked as gaps.

## Resolving a topic across languages

A topic is one *concept* but a different *article title* per language. We search
the topic on a pivot Wikipedia, take the best article, then follow its language
links (Wikidata-backed interwiki) to each target language. Because per-article
interwiki data is sometimes incomplete, we re-check gaps through the English
article, which is the best-connected hub. Languages with no linked article are
returned as `unresolved` rather than silently guessed.

## Per-series metrics

Let the series be views `y_i` at time offsets `x_i` (months or days from start).

- **Baseline** — `median_views`. Robust to spikes; the headline "is there enough
  traffic to mean anything" number.
- **Volatility** — coefficient of variation `stddev/mean`.
- **Trend** — ordinary least squares of `y` on `x`: `slope_per_bucket` and `r2`
  (how well a straight line explains the data).
- **Significance** — a two-sided t-test on the slope. `SE(slope)` comes from the
  residual variance; `p_value = 2·(1 − T_cdf(|slope/SE|, n−2))`. `significant`
  is `p < 0.05` with at least 4 points.
- **Window growth** — mean of the last ~¼ of buckets vs the first ~¼
  (`growth_pct`). More robust than comparing single endpoints.
- **CAGR** — annualised change between first and last bucket.
- **Year-over-year** — full-calendar-year totals only (partial years skipped).
- **Spikes** — buckets more than `2.5·stddev` above the mean.
- **Spike-driven** — recompute window growth with spike buckets removed; if the
  direction flips or the magnitude collapses to under a third, the "trend" is
  really one or two events (often a news cycle), not sustained interest.

## Direction: the defensible conclusion

`direction` is deliberately conservative:

- `growing` / `declining` **only** when the slope is statistically significant
  and the series is not spike-driven.
- otherwise `stable` — "no clear trend". A noisy series is never sold as a trend.

## Trust: confidence in the stated direction

Trust answers *"how much should I believe this conclusion for a product bet?"* —
not "is the data pretty". It combines data quality with trend reliability.

```
volScore     = clamp( log10(median+1) / log10(1000), 0..1 )   # 1000+/bucket ⇒ 1
dataQuality  = 0.5·volScore + 0.5·completeness

spike-driven:            score = 0.30 · dataQuality
stable (not significant): score = 0.85 · dataQuality           # trust "it's flat"
significant growth/decline: score = dataQuality · (0.4 + 0.6·r2)   # data GATES trend

hard cap: if median < 100 views/bucket  →  score = min(score, 0.5)
```

Label: `high ≥ 0.66`, `medium ≥ 0.40`, else `low`.

Rationale for the multiply (not average) on real trends: a perfectly clean line
drawn through 8 views/month is statistically significant but practically noise,
so low volume must *gate* the score, not merely average it down.

### Reason codes (surfaced in the report)

`low_baseline`, `strong_baseline`, `gaps`, `weak_fit`, `not_significant`,
`spike_driven`, `strong_signal`, `no_data`, `leader_low_trust`,
`leader_spike_driven`.

## Limitations to always communicate

1. Pageview interest is attention, **not willingness to pay**.
2. Data starts 2015-07 and lags ~1 day; recent buckets may be incomplete.
3. Language sections differ in size — absolute views are not directly comparable
   across languages (compare *trends/shapes*, not raw levels).
4. The chosen article title drives the result; confirm it fits the topic.
5. `agent=user` filters bots but not all automated traffic.
6. Wikipedia interest need not track commercial demand for a specific product.

## Tuning

Thresholds live as constants in `internal/analyze/analyze.go`
(`spikeK`, `lowBaseline`, `strongBaseline`, `minPointsForTest`) and the score
weights in `scoreTrust`. Change them there; unit tests in `analyze_test.go`
pin the intended behaviour.
