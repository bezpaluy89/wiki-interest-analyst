package analyze

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"wiki-interest-analyst/internal/wiki"
)

// monthly builds a synthetic monthly series starting 2022-01 with the given views.
func monthly(lang string, views ...int) *wiki.Series {
	s := &wiki.Series{Lang: lang, Project: lang + ".wikipedia", Article: "X", Granularity: "monthly"}
	year, month := 2022, 1
	for _, v := range views {
		s.Points = append(s.Points, wiki.Point{Date: fmt.Sprintf("%04d-%02d-01", year, month), Views: v})
		s.Total += v
		month++
		if month > 12 {
			month = 1
			year++
		}
	}

	return s
}

func TestGrowingTrendIsSignificantAndTrusted(t *testing.T) {
	r := require.New(t)
	var v []int
	for i := 0; i < 24; i++ {
		v = append(v, 1000+i*200)
	}
	a := AnalyzeSeries(monthly("uk", v...))

	r.Equal(DirGrowing, a.Direction)
	r.True(a.Significant, "expected significant trend, p=%v", a.PValue)
	r.Greater(a.R2, 0.9, "clean ramp should fit a line tightly")
	r.Equal(TrustHigh, a.Trust, "trust score %.2f", a.TrustScore)
	r.Positive(a.GrowthPct)
}

func TestFlatNoisySeriesIsStableNotATrend(t *testing.T) {
	r := require.New(t)
	// Oscillates around 2000 with no real direction.
	v := []int{2000, 1800, 2200, 1900, 2100, 2000, 1850, 2150, 1950, 2050, 2000, 1900,
		2100, 1950, 2050, 2000, 1900, 2100, 1980, 2020, 2000, 1900, 2100, 2000}
	a := AnalyzeSeries(monthly("uk", v...))

	r.Equal(DirStable, a.Direction, "noisy flat data must not be sold as a trend")
	r.False(a.Significant, "flat data should not be significant (p=%v)", a.PValue)
	r.NotEqual(TrustLow, a.Trust, "stable + good data should not be low trust (score %.2f)", a.TrustScore)
	r.Contains(a.TrustReasons, "not_significant")
}

func TestSpikeDrivenGrowthIsFlagged(t *testing.T) {
	r := require.New(t)
	// Flat baseline with one huge one-off spike at the end.
	v := []int{300, 320, 310, 305, 315, 300, 310, 320, 300, 315, 305, 8000}
	a := AnalyzeSeries(monthly("uk", v...))

	r.NotEmpty(a.Spikes, "expected a spike to be detected")
	r.True(a.SpikeDriven)
	r.Equal(DirStable, a.Direction, "spike-driven data must not be sold as a trend")
	r.NotEqual(TrustHigh, a.Trust, "spike-driven series should not be high trust (score %.2f)", a.TrustScore)
}

func TestLowBaselineLowersTrust(t *testing.T) {
	r := require.New(t)
	// Rising but tiny numbers: statistically a trend, but too noisy to bank on.
	v := []int{2, 3, 2, 4, 3, 5, 4, 6, 5, 7, 6, 8, 7, 9, 8, 10, 9, 11, 10, 12, 11, 13, 12, 14}
	a := AnalyzeSeries(monthly("uk", v...))

	r.Contains(a.TrustReasons, "low_baseline")
	r.NotEqual(TrustHigh, a.Trust, "low-volume series should not earn high trust (score %.2f)", a.TrustScore)
}

func TestCompletenessAndMissingBuckets(t *testing.T) {
	r := require.New(t)
	s := monthly("uk", 100, 200, 300)
	s.Missing = []string{"2022-04", "2022-05"} // pretend two months had no data
	a := AnalyzeSeries(s)

	r.Equal(5, a.Expected)
	r.Less(a.Completeness, 1.0, "completeness must be < 1 with gaps")
	r.Contains(a.TrustReasons, "gaps")
}

func TestComparisonRanksByGrowth(t *testing.T) {
	r := require.New(t)
	up := monthly("pl", 100, 200, 300, 400, 500, 600, 700, 800, 900, 1000, 1100, 1200)
	down := monthly("cs", 1200, 1100, 1000, 900, 800, 700, 600, 500, 400, 300, 200, 100)
	res := Analyze("t", "2022-01-01", "2022-12-31", "monthly", []*wiki.Series{down, up})

	r.Equal("pl", res.Comparison.Leader, "leader should be the highest-growth language")
	r.Len(res.Comparison.Ranking, 2)
	r.Equal("pl", res.Comparison.Ranking[0].Lang, "ranking must be sorted by growth")
}

func TestEmptySeriesIsLowTrustNoData(t *testing.T) {
	r := require.New(t)
	a := AnalyzeSeries(&wiki.Series{Lang: "uk", Granularity: "monthly"})
	r.Equal(TrustLow, a.Trust)
	r.Contains(a.TrustReasons, "no_data")
}
