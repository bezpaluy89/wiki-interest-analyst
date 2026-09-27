// Package analyze turns raw pageview series into trend metrics and, crucially,
// a *trust* assessment: how much a growth conclusion can be believed. A rising
// line drawn from noisy, low-volume, or spike-driven data is a trap for product
// decisions, so every conclusion here ships with the evidence for and against it.
package analyze

import (
	"fmt"
	"math"
	"sort"
	"time"

	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/gonum/stat/distuv"

	"wiki-interest-analyst/internal/wiki"
)

const (
	TrustHigh   = "high"
	TrustMedium = "medium"
	TrustLow    = "low"
)

type SeriesAnalysis struct {
	Lang    string `json:"lang"`
	Project string `json:"project"`
	Article string `json:"article"`

	N            int     `json:"n"`
	Expected     int     `json:"expected"`
	Completeness float64 `json:"completeness"`
	Total        int     `json:"total"`
	MeanViews    float64 `json:"mean_views"`
	MedianViews  float64 `json:"median_views"`
	CV           float64 `json:"coef_variation"`

	SlopePerBucket float64 `json:"slope_per_bucket"`
	R2             float64 `json:"r2"`
	PValue         float64 `json:"p_value"`
	Significant    bool    `json:"significant"`

	GrowthPct float64 `json:"growth_pct"`
	CAGRPct   float64 `json:"cagr_pct"`
	YoY       []YoY   `json:"yoy,omitempty"`

	Direction string `json:"direction"`

	Spikes      []Spike `json:"spikes,omitempty"`
	SpikeDriven bool    `json:"spike_driven"`

	Trust        string   `json:"trust"`
	TrustScore   float64  `json:"trust_score"`
	TrustReasons []string `json:"trust_reasons"`
}

// YoY is one year-over-year comparison of full-year totals.
type YoY struct {
	Year int     `json:"year"`
	Pct  float64 `json:"pct"`
}

// Spike is an unusually high bucket (> mean + k*stddev).
type Spike struct {
	Date  string  `json:"date"`
	Views int     `json:"views"`
	Z     float64 `json:"z"` // standard deviations above the mean
}

type Analysis struct {
	Theme       string           `json:"theme"`
	Start       string           `json:"start"`
	End         string           `json:"end"`
	Granularity string           `json:"granularity"`
	GeneratedAt string           `json:"generated_at"`
	Series      []SeriesAnalysis `json:"series"`
	Comparison  Comparison       `json:"comparison"`
	RawSeries   []*wiki.Series   `json:"raw_series,omitempty"`
}

type Comparison struct {
	RankBy  string      `json:"rank_by"`
	Ranking []RankEntry `json:"ranking"`
	Leader  string      `json:"leader"`
	Notes   []string    `json:"notes"`
}

type RankEntry struct {
	Lang      string  `json:"lang"`
	GrowthPct float64 `json:"growth_pct"`
	Trust     string  `json:"trust"`
}

const (
	spikeK           = 2.5
	lowBaseline      = 100.0
	strongBaseline   = 1000.0
	minPointsForTest = 4
)

func AnalyzeSeries(s *wiki.Series) SeriesAnalysis {
	a := SeriesAnalysis{
		Lang: s.Lang, Project: s.Project, Article: s.Article,
		N:        len(s.Points),
		Total:    s.Total,
		Expected: len(s.Points) + len(s.Missing),
	}

	if a.Expected > 0 {
		a.Completeness = float64(a.N) / float64(a.Expected)
	}

	if a.N == 0 {
		a.Trust = TrustLow
		a.TrustReasons = []string{"no_data"}

		return a
	}

	views := make([]float64, a.N)
	xs := make([]float64, a.N)
	for i, p := range s.Points {
		views[i] = float64(p.Views)
		xs[i] = bucketOffset(s.Points[0].Date, p.Date, s.Granularity)
	}

	a.MeanViews = stat.Mean(views, nil)
	a.MedianViews = median(views)
	std := stat.StdDev(views, nil)
	if a.MeanViews > 0 {
		a.CV = std / a.MeanViews
	}

	if a.N >= 2 {
		alpha, beta := stat.LinearRegression(xs, views, nil, false)
		a.SlopePerBucket = beta
		a.R2 = stat.RSquared(xs, views, nil, alpha, beta)
		a.PValue = slopePValue(xs, views, alpha, beta)
		a.Significant = a.N >= minPointsForTest && a.PValue < 0.05
	}

	a.GrowthPct = windowGrowth(views)
	a.CAGRPct = cagr(s.Points, s.Granularity)
	a.YoY = yearOverYear(s.Points, s.Granularity)

	a.Spikes = detectSpikes(s.Points, a.MeanViews, std)
	a.SpikeDriven = spikeDriven(views, a.GrowthPct, a.MeanViews, std)

	a.Direction = classifyDirection(&a)
	a.TrustScore, a.Trust, a.TrustReasons = scoreTrust(&a)

	return a
}

const (
	DirGrowing   = "growing"
	DirDeclining = "declining"
	DirStable    = "stable"
)

func classifyDirection(a *SeriesAnalysis) string {
	if a.SpikeDriven || !a.Significant {
		return DirStable
	}

	switch {
	case a.SlopePerBucket > 0:
		return DirGrowing
	case a.SlopePerBucket < 0:
		return DirDeclining
	default:
		return DirStable
	}
}

func Analyze(theme, start, end, granularity string, series []*wiki.Series) Analysis {
	out := Analysis{
		Theme: theme, Start: start, End: end, Granularity: granularity,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	for _, s := range series {
		out.Series = append(out.Series, AnalyzeSeries(s))
	}

	out.RawSeries = series
	out.Comparison = compare(out.Series)

	return out
}

func compare(series []SeriesAnalysis) Comparison {
	c := Comparison{RankBy: "growth_pct"}

	for _, s := range series {
		c.Ranking = append(c.Ranking, RankEntry{Lang: s.Lang, GrowthPct: s.GrowthPct, Trust: s.Trust})
	}

	sort.SliceStable(c.Ranking, func(i, j int) bool {
		return c.Ranking[i].GrowthPct > c.Ranking[j].GrowthPct
	})
	if len(c.Ranking) > 0 {
		c.Leader = c.Ranking[0].Lang
		if c.Ranking[0].Trust == TrustLow {
			c.Notes = append(c.Notes, "leader_low_trust")
		}
	}

	for _, s := range series {
		if s.Lang == c.Leader && s.SpikeDriven {
			c.Notes = append(c.Notes, "leader_spike_driven")
		}
	}

	return c
}

// scoreTrust rates confidence in the *stated conclusion* (a.Direction), not in
// the raw data alone. The rule of thumb: you can only trust a conclusion if the
// data is good AND (for a growth/decline claim) the trend is statistically real.
// A high-quality but flat series earns high trust in "stable"; a rising line
// from thin or spiky data earns low trust in "growth".
func scoreTrust(a *SeriesAnalysis) (float64, string, []string) {
	var reasons []string

	volScore := clamp(math.Log10(a.MedianViews+1)/math.Log10(strongBaseline), 0, 1)
	dataQuality := 0.5*volScore + 0.5*a.Completeness

	if a.MedianViews < lowBaseline {
		reasons = append(reasons, "low_baseline")
	} else if a.MedianViews >= strongBaseline {
		reasons = append(reasons, "strong_baseline")
	}

	if a.Completeness < 0.9 {
		reasons = append(reasons, "gaps")
	}

	var score float64
	switch {
	case a.SpikeDriven:
		score = 0.3 * dataQuality
		reasons = append(reasons, "spike_driven")
	case a.Direction == DirStable:
		score = 0.85 * dataQuality
		if a.N >= minPointsForTest {
			reasons = append(reasons, "not_significant")
		}
	default:
		trendConf := clamp(0.4+0.6*a.R2, 0, 1)
		score = dataQuality * trendConf
		if a.R2 < 0.5 {
			reasons = append(reasons, "weak_fit")
		}
	}

	if a.MedianViews < lowBaseline {
		score = math.Min(score, 0.5)
	}

	var label string
	switch {
	case score >= 0.66:
		label = TrustHigh
	case score >= 0.4:
		label = TrustMedium
	default:
		label = TrustLow
	}

	if label == TrustHigh && a.Direction != DirStable && !a.SpikeDriven && len(reasons) <= 1 {
		reasons = append(reasons, "strong_signal")
	}

	return score, label, reasons
}

// slopePValue returns the two-sided p-value for H0: slope == 0, via a t-test on
// the OLS slope. Fewer than 3 points has no residual degrees of freedom.
func slopePValue(xs, ys []float64, alpha, beta float64) float64 {
	n := len(xs)
	if n < 3 {
		return 1
	}

	var sse, ssx float64
	xbar := stat.Mean(xs, nil)

	for i := range xs {
		resid := ys[i] - (alpha + beta*xs[i])
		sse += resid * resid
		ssx += (xs[i] - xbar) * (xs[i] - xbar)
	}

	if ssx == 0 {
		return 1
	}

	df := float64(n - 2)
	s2 := sse / df
	se := math.Sqrt(s2 / ssx)
	if se == 0 {
		return 0
	}

	t := beta / se
	dist := distuv.StudentsT{Mu: 0, Sigma: 1, Nu: df}

	return 2 * (1 - dist.CDF(math.Abs(t)))
}

// windowGrowth compares the mean of the last window to the first window, which
// is far more robust to single-bucket noise than comparing endpoints.
func windowGrowth(views []float64) float64 {
	n := len(views)
	if n < 2 {
		return 0
	}

	w := n / 4
	if w < 1 {
		w = 1
	}

	if w > 3 {
		w = 3
	}

	first := stat.Mean(views[:w], nil)
	last := stat.Mean(views[n-w:], nil)
	if first == 0 {
		return math.NaN()
	}

	return (last - first) / first * 100
}

func cagr(points []wiki.Point, granularity string) float64 {
	if len(points) < 2 {
		return 0
	}

	first := float64(points[0].Views)
	last := float64(points[len(points)-1].Views)
	if first <= 0 || last <= 0 {
		return math.NaN()
	}

	years := bucketOffset(points[0].Date, points[len(points)-1].Date, granularity)
	if granularity == "monthly" {
		years /= 12
	} else {
		years /= 365
	}

	if years <= 0 {
		return 0
	}

	return (math.Pow(last/first, 1/years) - 1) * 100
}

func yearOverYear(points []wiki.Point, granularity string) []YoY {
	if granularity != "monthly" {
		return nil
	}

	totals := map[int]int{}
	counts := map[int]int{}
	for _, p := range points {
		t, err := time.Parse("2006-01-02", p.Date)
		if err != nil {
			continue
		}

		totals[t.Year()] += p.Views
		counts[t.Year()]++
	}

	var years []int
	for y := range totals {
		years = append(years, y)
	}

	sort.Ints(years)
	var out []YoY
	for i := 1; i < len(years); i++ {
		prev, cur := years[i-1], years[i]
		if counts[prev] < 12 || counts[cur] < 12 {
			continue
		}

		if totals[prev] == 0 {
			continue
		}

		pct := float64(totals[cur]-totals[prev]) / float64(totals[prev]) * 100
		out = append(out, YoY{Year: cur, Pct: pct})
	}

	return out
}

func detectSpikes(points []wiki.Point, mean, std float64) []Spike {
	if std == 0 {
		return nil
	}

	var spikes []Spike
	for _, p := range points {
		z := (float64(p.Views) - mean) / std
		if z >= spikeK {
			spikes = append(spikes, Spike{Date: p.Date, Views: p.Views, Z: round2(z)})
		}
	}

	return spikes
}

// spikeDriven reports whether the growth conclusion collapses once spike buckets
// are removed — i.e. the "trend" is really one or two news events.
func spikeDriven(views []float64, fullGrowth, mean, std float64) bool {
	if std == 0 || len(views) < 4 {
		return false
	}

	var trimmed []float64
	for _, v := range views {
		if (v-mean)/std < spikeK {
			trimmed = append(trimmed, v)
		}
	}

	if len(trimmed) == len(views) || len(trimmed) < 2 {
		return false // no spikes, or too little left to judge
	}
	trimmedGrowth := windowGrowth(trimmed)

	if math.IsNaN(trimmedGrowth) {
		return true
	}

	if sign(trimmedGrowth) != sign(fullGrowth) {
		return true
	}

	return math.Abs(trimmedGrowth) < math.Abs(fullGrowth)/3
}

// bucketOffset returns the number of buckets between two YYYY-MM-DD dates.
func bucketOffset(from, to, granularity string) float64 {
	f, err1 := time.Parse("2006-01-02", from)
	t, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil {
		return 0
	}

	if granularity == "monthly" {
		return float64((t.Year()-f.Year())*12 + int(t.Month()) - int(f.Month()))
	}

	return t.Sub(f).Hours() / 24
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}

	c := append([]float64(nil), v...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}

	return (c[n/2-1] + c[n/2]) / 2
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func sign(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// FormatSummary is a compact human line used in logs/CLI (English, ASCII-safe).
func (a SeriesAnalysis) FormatSummary() string {
	return fmt.Sprintf("%s: growth %.0f%%, median %.0f/bucket, R2 %.2f, trust %s",
		a.Lang, a.GrowthPct, a.MedianViews, a.R2, a.Trust)
}
