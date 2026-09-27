package wiki

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type Resolution struct {
	Theme      string            `json:"theme"`
	PivotLang  string            `json:"pivot_lang"`
	PivotTitle string            `json:"pivot_title"`
	Titles     map[string]string `json:"titles"`     // lang -> article title
	Unresolved []string          `json:"unresolved"` // langs with no equivalent article
}

// Resolve finds the best-matching article for theme in pivotLang, then follows
// Wikipedia's language links to obtain the equivalent title in each target
// language. This keeps comparisons about the *same concept* even though the
// article name differs per language (e.g. "Astronomy" vs "Астрономія").
//
// Article-title choice is an assumption worth surfacing: Resolve returns what it
// picked so the agent (or user) can confirm or override it.
func (c *Client) Resolve(theme, pivotLang string, langs []string) (*Resolution, error) {
	if len(langs) == 0 {
		return nil, fmt.Errorf("resolve: no languages given")
	}

	var pivotTitle string
	if pivotLang != "" {
		t, found, err := c.search(pivotLang, theme)
		if err != nil {
			return nil, err
		}

		if !found {
			return nil, fmt.Errorf("no %s.wikipedia article found for %q", pivotLang, theme)
		}

		pivotTitle = t
	} else {
		for _, cand := range pivotCandidates(langs) {
			t, found, err := c.search(cand, theme)
			if err != nil {
				return nil, err
			}

			if found {
				pivotLang, pivotTitle = cand, t

				break
			}
		}

		if pivotLang == "" {
			return nil, fmt.Errorf("no Wikipedia article found for %q in %v (try --pivot-lang)", theme, langs)
		}
	}

	res := &Resolution{
		Theme:      theme,
		PivotLang:  pivotLang,
		PivotTitle: pivotTitle,
		Titles:     map[string]string{pivotLang: pivotTitle},
	}

	links, err := c.langLinks(pivotLang, pivotTitle)
	if err != nil {
		return nil, err
	}

	for _, lang := range langs {
		if lang == pivotLang {
			continue
		}

		if t, ok := links[lang]; ok {
			res.Titles[lang] = t
		} else {
			res.Unresolved = append(res.Unresolved, lang)
		}
	}

	if len(res.Unresolved) > 0 {
		if enTitle, ok := links["en"]; ok && pivotLang != "en" {
			if enLinks, err := c.langLinks("en", enTitle); err == nil {
				var still []string
				for _, lang := range res.Unresolved {
					if t, ok := enLinks[lang]; ok {
						res.Titles[lang] = t
					} else {
						still = append(still, lang)
					}
				}

				res.Unresolved = still
			}
		}
	}

	return res, nil
}

func (c *Client) actionAPI(lang string, params url.Values) string {
	params.Set("format", "json")
	params.Set("formatversion", "2")
	base := c.ActionBase
	if base == "" {
		base = defaultActionBase
	}

	return fmt.Sprintf(base, lang) + "?" + params.Encode()
}

type searchResponse struct {
	Query struct {
		Search []struct {
			Title string `json:"title"`
		} `json:"search"`
	} `json:"query"`
}

func pivotCandidates(langs []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(l string) {
		if l != "" && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	for _, l := range langs {
		add(l)
	}

	add("en")
	add("uk")

	return out
}

// search returns the best article title for theme in lang. found is false when
// the query simply has no results (not an error); err is for transport failures.
func (c *Client) search(lang, theme string) (title string, found bool, err error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "search")
	params.Set("srsearch", theme)
	params.Set("srlimit", "1")
	params.Set("srnamespace", "0")

	body, err := c.getJSON(c.actionAPI(lang, params))
	if err != nil {
		return "", false, err
	}

	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return "", false, fmt.Errorf("decoding search response: %w", err)
	}

	if len(sr.Query.Search) == 0 {
		return "", false, nil
	}

	return sr.Query.Search[0].Title, true, nil
}

type langLinksResponse struct {
	Query struct {
		Pages []struct {
			Title     string `json:"title"`
			LangLinks []struct {
				Lang  string `json:"lang"`
				Title string `json:"title"`
			} `json:"langlinks"`
		} `json:"pages"`
	} `json:"query"`
}

func (c *Client) langLinks(lang, title string) (map[string]string, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", title)
	params.Set("prop", "langlinks")
	params.Set("lllimit", "500")

	body, err := c.getJSON(c.actionAPI(lang, params))
	if err != nil {
		return nil, err
	}

	var lr langLinksResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return nil, fmt.Errorf("decoding langlinks response: %w", err)
	}

	out := make(map[string]string)
	for _, pg := range lr.Query.Pages {
		for _, ll := range pg.LangLinks {
			out[strings.ToLower(ll.Lang)] = ll.Title
		}
	}

	return out, nil
}
