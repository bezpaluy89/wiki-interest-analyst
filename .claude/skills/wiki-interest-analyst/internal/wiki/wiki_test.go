package wiki

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageviewsParsingAndMissingBuckets(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	var gotUA, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotUA = req.Header.Get("User-Agent")
		gotPath = req.URL.EscapedPath() // raw, still percent-encoded
		w.Write([]byte(`{"items":[
			{"timestamp":"2022010100","views":100},
			{"timestamp":"2022030100","views":300}
		]}`))
	}))
	defer srv.Close()

	c := NewClient("")
	c.PageviewBase = srv.URL

	s, err := c.Pageviews(PageviewParams{
		Project: "uk.wikipedia", Article: "Астрономія",
		Start: "2022-01-01", End: "2022-03-31", Granularity: "monthly",
	})
	r.NoError(err)

	a.NotEmpty(gotUA, "User-Agent header must be sent (Wikimedia rejects requests without one)")
	a.Len(s.Points, 2)
	a.Equal(400, s.Total)
	a.Equal([]string{"2022-02"}, s.Missing)
	a.Contains(gotPath, "%D0%90", "article title should be percent-encoded in the path")
	a.Equal("user", s.Agent, "default agent should exclude bots")
}

func TestPageviewsRejectsBadDates(t *testing.T) {
	r := require.New(t)
	c := NewClient("")
	_, err := c.Pageviews(PageviewParams{
		Project: "uk.wikipedia", Article: "X", Start: "2022-13-01", End: "2022-12-31",
	})
	r.Error(err, "invalid --start must be rejected")
}

func TestResolveSearchThenLangLinks(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		if q.Get("list") == "search" {
			w.Write([]byte(`{"query":{"search":[{"title":"Астрономія"}]}}`))

			return
		}

		if q.Get("prop") == "langlinks" {
			w.Write([]byte(`{"query":{"pages":[{"title":"Астрономія","langlinks":[
				{"lang":"pl","title":"Astronomia"},
				{"lang":"cs","title":"Astronomie"}
			]}]}}`))

			return
		}

		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("")
	c.ActionBase = srv.URL + "/%s"

	res, err := c.Resolve("астрономія", "uk", []string{"uk", "pl", "cs"})
	r.NoError(err)
	a.Equal("Астрономія", res.Titles["uk"])
	a.Equal("Astronomia", res.Titles["pl"])
	a.Equal("Astronomie", res.Titles["cs"])
	a.Empty(res.Unresolved)
}

func TestResolveAutoPivotAndUnresolved(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		lang := strings.Trim(strings.TrimPrefix(req.URL.Path, "/"), "/")
		if q.Get("list") == "search" {
			if lang == "uk" {
				w.Write([]byte(`{"query":{"search":[{"title":"Тема"}]}}`))
			} else {
				w.Write([]byte(`{"query":{"search":[]}}`))
			}

			return
		}

		if q.Get("prop") == "langlinks" {
			w.Write([]byte(`{"query":{"pages":[{"title":"Тема","langlinks":[{"lang":"pl","title":"Temat"}]}]}}`))

			return
		}

		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("")
	c.ActionBase = srv.URL + "/%s"

	res, err := c.Resolve("тема", "", []string{"pl", "uk", "de"})
	r.NoError(err)
	a.Equal("uk", res.PivotLang, "pivot should auto-detect to the language that has the article")
	a.Equal("Temat", res.Titles["pl"])
	a.Equal([]string{"de"}, res.Unresolved)
}
