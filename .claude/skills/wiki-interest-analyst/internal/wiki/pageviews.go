package wiki

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// EarliestData is the first day the Pageviews API has data for.
const EarliestData = "2015-07-01"

// Point is a single time bucket of views.
type Point struct {
	Date  string `json:"date"`
	Views int    `json:"views"`
}

// Series is a full pageview time series for one article in one project.
type Series struct {
	Lang        string   `json:"lang"`
	Project     string   `json:"project"`
	Article     string   `json:"article"`
	Granularity string   `json:"granularity"`
	Access      string   `json:"access"`
	Agent       string   `json:"agent"`
	Points      []Point  `json:"points"`
	Missing     []string `json:"missing_buckets"`
	Total       int      `json:"total"`
}

// PageviewParams configures a Pageviews query.
type PageviewParams struct {
	Project     string
	Article     string
	Start       string
	End         string
	Granularity string
	Access      string
	Agent       string
}

func (p *PageviewParams) applyDefaults() {
	if p.Granularity == "" {
		p.Granularity = "monthly"
	}

	if p.Access == "" {
		p.Access = "all-access"
	}

	if p.Agent == "" {
		p.Agent = "user"
	}
}

type pvResponse struct {
	Items []struct {
		Timestamp string `json:"timestamp"`
		Views     int    `json:"views"`
	} `json:"items"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func (c *Client) Pageviews(p PageviewParams) (*Series, error) {
	p.applyDefaults()

	start, err := time.Parse("2006-01-02", p.Start)
	if err != nil {
		return nil, fmt.Errorf("bad --start %q (want YYYY-MM-DD): %w", p.Start, err)
	}

	end, err := time.Parse("2006-01-02", p.End)
	if err != nil {
		return nil, fmt.Errorf("bad --end %q (want YYYY-MM-DD): %w", p.End, err)
	}

	if end.Before(start) {
		return nil, fmt.Errorf("--end %s is before --start %s", p.End, p.Start)
	}

	article := strings.ReplaceAll(strings.TrimSpace(p.Article), " ", "_")
	base := c.PageviewBase
	if base == "" {
		base = defaultPageviewBase
	}

	endpoint := fmt.Sprintf(
		"%s/%s/%s/%s/%s/%s/%s/%s",
		base, p.Project, p.Access, p.Agent,
		url.PathEscape(article),
		p.Granularity,
		start.Format("20060102"), end.Format("20060102"),
	)

	body, err := c.getJSON(endpoint)
	if err != nil {
		return nil, err
	}

	var raw pvResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decoding pageviews response: %w", err)
	}

	if raw.Type != "" && len(raw.Items) == 0 {

		return nil, fmt.Errorf("no data for %q on %s: %s", p.Article, p.Project, raw.Title)
	}

	s := &Series{
		Lang:        strings.SplitN(p.Project, ".", 2)[0],
		Project:     p.Project,
		Article:     article,
		Granularity: p.Granularity,
		Access:      p.Access,
		Agent:       p.Agent,
	}

	present := make(map[string]bool)
	for _, it := range raw.Items {
		ts, perr := time.Parse("2006010215", it.Timestamp)
		if perr != nil {
			continue
		}
		date := ts.Format("2006-01-02")
		s.Points = append(s.Points, Point{Date: date, Views: it.Views})
		s.Total += it.Views
		present[bucketKey(ts, p.Granularity)] = true
	}

	s.Missing = missingBuckets(start, end, p.Granularity, present)

	return s, nil
}

func bucketKey(t time.Time, granularity string) string {
	if granularity == "daily" {
		return t.Format("2006-01-02")
	}

	return t.Format("2006-01")
}

// missingBuckets lists expected buckets in [start,end] that returned no data.
// Gaps matter for trust: a series full of holes is not a reliable trend.
func missingBuckets(start, end time.Time, granularity string, present map[string]bool) []string {
	var missing []string
	if granularity == "daily" {
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			if k := d.Format("2006-01-02"); !present[k] {
				missing = append(missing, k)
			}
		}

		return missing
	}

	cur := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	for ; !cur.After(last); cur = cur.AddDate(0, 1, 0) {
		if k := cur.Format("2006-01"); !present[k] {
			missing = append(missing, k)
		}
	}

	return missing
}
