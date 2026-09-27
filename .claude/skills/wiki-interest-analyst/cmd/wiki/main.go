// Command wiki is the executable behind the wiki-interest-analyst skill.
//
// Subcommands (each prints JSON to stdout):
//
//	resolve  topic  -> per-language article titles
//	fetch    one series of pageviews
//	analyze  resolve+fetch+metrics+trust for several languages (JSON only)
//	report   render chart + one-page PDF from an analyze --json file
//	brief    the golden path: analyze + PDF in one call
//
// Design: all heavy lifting (HTTP, stats, charts, PDF) is in code so a cheap
// model only has to pick a subcommand and read compact JSON.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wiki-interest-analyst/internal/analyze"
	"wiki-interest-analyst/internal/report"
	"wiki-interest-analyst/internal/wiki"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "resolve":
		err = cmdResolve(os.Args[2:])
	case "fetch":
		err = cmdFetch(os.Args[2:])
	case "analyze":
		err = cmdAnalyze(os.Args[2:])
	case "report":
		err = cmdReport(os.Args[2:])
	case "brief":
		err = cmdBrief(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `wiki — Wikipedia interest analyst

  wiki resolve  --theme T --langs uk,pl[,..] [--pivot-lang uk]
  wiki fetch    --project uk.wikipedia --article A --start YYYY-MM-DD --end YYYY-MM-DD
                [--granularity monthly|daily] [--agent user] [--access all-access]
  wiki analyze  --theme T --langs uk,pl --start .. --end .. [--granularity ..]
                [--pivot-lang uk] [--titles "uk=Title,pl=Tytuł"] [--json out.json]
  wiki report   --input analysis.json --out report.pdf [--lang uk|en] [--title ..]
  wiki brief    --theme T --langs uk,pl --start .. --end .. --out report.pdf
                [--lang uk|en] [--granularity ..] [--pivot-lang uk] [--titles ..] [--title ..]

Common: --cache DIR (default: OS cache dir), --no-cache
`)
}

func cacheFlags(fs *flag.FlagSet) (*string, *bool) {
	cache := fs.String("cache", defaultCacheDir(), "response cache directory")
	noCache := fs.Bool("no-cache", false, "disable the response cache")

	return cache, noCache
}

func defaultCacheDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "wiki-interest-analyst")
	}

	return filepath.Join(os.TempDir(), "wiki-interest-analyst-cache")
}

func newClient(cacheDir string, noCache bool) *wiki.Client {
	if noCache {
		cacheDir = ""
	}

	return wiki.NewClient(cacheDir)
}

func splitLangs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToLower(p))
		}
	}

	return out
}

func parseTitles(s string) map[string]string {
	out := map[string]string{}

	if s == "" {
		return out
	}

	for _, pair := range strings.Split(s, ",") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			out[strings.ToLower(strings.TrimSpace(kv[0]))] = strings.TrimSpace(kv[1])
		}
	}

	return out
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}

func cmdResolve(args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	theme := fs.String("theme", "", "topic to resolve")
	langs := fs.String("langs", "", "comma-separated language codes")
	pivot := fs.String("pivot-lang", "", "language to search first (default: first of --langs)")
	cache, noCache := cacheFlags(fs)
	fs.Parse(args)

	if *theme == "" || *langs == "" {
		return fmt.Errorf("resolve requires --theme and --langs")
	}

	c := newClient(*cache, *noCache)
	res, err := c.Resolve(*theme, *pivot, splitLangs(*langs))
	if err != nil {
		return err
	}

	return printJSON(res)
}

func cmdFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	project := fs.String("project", "", "e.g. uk.wikipedia")
	article := fs.String("article", "", "article title")
	start := fs.String("start", "", "YYYY-MM-DD")
	end := fs.String("end", "", "YYYY-MM-DD")
	gran := fs.String("granularity", "monthly", "monthly|daily")
	agent := fs.String("agent", "user", "user|all-agents|spider|automated")
	access := fs.String("access", "all-access", "all-access|desktop|mobile-app|mobile-web")
	cache, noCache := cacheFlags(fs)
	fs.Parse(args)

	if *project == "" || *article == "" || *start == "" || *end == "" {
		return fmt.Errorf("fetch requires --project --article --start --end")
	}

	c := newClient(*cache, *noCache)
	s, err := c.Pageviews(wiki.PageviewParams{
		Project: *project, Article: *article, Start: *start, End: *end,
		Granularity: *gran, Agent: *agent, Access: *access,
	})
	if err != nil {
		return err
	}

	return printJSON(s)
}

type analyzeConfig struct {
	theme, langs, pivot, titles string
	start, end, gran            string
	projectSuffix               string
}

func buildAnalysis(cfg analyzeConfig, c *wiki.Client) (analyze.Analysis, error) {
	langs := splitLangs(cfg.langs)
	if cfg.theme == "" || len(langs) == 0 || cfg.start == "" || cfg.end == "" {
		return analyze.Analysis{}, fmt.Errorf("require --theme --langs --start --end")
	}

	overrides := parseTitles(cfg.titles)

	titles := map[string]string{}
	for k, v := range overrides {
		titles[k] = v
	}

	var need []string
	for _, l := range langs {
		if _, ok := titles[l]; !ok {
			need = append(need, l)
		}
	}

	if len(need) > 0 {
		res, err := c.Resolve(cfg.theme, cfg.pivot, need)
		if err != nil {
			return analyze.Analysis{}, err
		}

		for l, t := range res.Titles {
			titles[l] = t
		}
	}

	suffix := cfg.projectSuffix
	if suffix == "" {
		suffix = "wikipedia"
	}

	var series []*wiki.Series
	for _, l := range langs {
		title, ok := titles[l]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: no article resolved for %q, skipping\n", l)

			continue
		}

		s, err := c.Pageviews(wiki.PageviewParams{
			Project: l + "." + suffix, Article: title,
			Start: cfg.start, End: cfg.end, Granularity: cfg.gran,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", l, err)

			continue
		}

		series = append(series, s)
	}

	if len(series) == 0 {
		return analyze.Analysis{}, fmt.Errorf("no data fetched for any language")
	}

	return analyze.Analyze(cfg.theme, cfg.start, cfg.end, cfg.gran, series), nil
}

func leanCopy(a analyze.Analysis) analyze.Analysis {
	a.RawSeries = nil

	return a
}

func cmdAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	cfg := analyzeConfig{}
	fs.StringVar(&cfg.theme, "theme", "", "topic")
	fs.StringVar(&cfg.langs, "langs", "", "comma-separated language codes")
	fs.StringVar(&cfg.pivot, "pivot-lang", "", "search language (default: first of --langs)")
	fs.StringVar(&cfg.titles, "titles", "", `override titles, e.g. "uk=Title,pl=Tytuł"`)
	fs.StringVar(&cfg.start, "start", "", "YYYY-MM-DD")
	fs.StringVar(&cfg.end, "end", "", "YYYY-MM-DD")
	fs.StringVar(&cfg.gran, "granularity", "monthly", "monthly|daily")
	fs.StringVar(&cfg.projectSuffix, "project-suffix", "wikipedia", "wiki project suffix")
	jsonOut := fs.String("json", "", "also write full analysis (with raw series) to this file")
	cache, noCache := cacheFlags(fs)
	fs.Parse(args)

	c := newClient(*cache, *noCache)
	a, err := buildAnalysis(cfg, c)
	if err != nil {
		return err
	}

	if *jsonOut != "" {
		if err := writeAnalysis(a, *jsonOut); err != nil {
			return err
		}
	}

	return printJSON(leanCopy(a))
}

func writeAnalysis(a analyze.Analysis, path string) error {
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, b, 0o644)
}

func cmdReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	input := fs.String("input", "", "analysis JSON from `analyze --json`")
	out := fs.String("out", "report.pdf", "output PDF path")
	lang := fs.String("lang", "uk", "report language: uk|en")
	title := fs.String("title", "", "override report title")
	fs.Parse(args)

	if *input == "" {
		return fmt.Errorf("report requires --input")
	}

	b, err := os.ReadFile(*input)
	if err != nil {
		return err
	}

	var a analyze.Analysis
	if err := json.Unmarshal(b, &a); err != nil {
		return fmt.Errorf("parsing analysis json: %w", err)
	}

	return renderReport(a, *out, *lang, *title)
}

func renderReport(a analyze.Analysis, out, lang, title string) error {
	chartPath := strings.TrimSuffix(out, filepath.Ext(out)) + ".png"
	if len(a.RawSeries) > 0 {
		if err := report.RenderChart(a.RawSeries, chartPath, 1000, 460); err != nil {
			return fmt.Errorf("chart: %w", err)
		}
	} else {
		chartPath = ""
	}

	if err := report.RenderPDF(a, chartPath, out, lang, title); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "wrote %s", out)

	if chartPath != "" {
		fmt.Fprintf(os.Stderr, " and %s", chartPath)
	}

	fmt.Fprintln(os.Stderr)

	return nil
}

func cmdBrief(args []string) error {
	fs := flag.NewFlagSet("brief", flag.ExitOnError)
	cfg := analyzeConfig{}
	fs.StringVar(&cfg.theme, "theme", "", "topic")
	fs.StringVar(&cfg.langs, "langs", "", "comma-separated language codes")
	fs.StringVar(&cfg.pivot, "pivot-lang", "", "search language (default: first of --langs)")
	fs.StringVar(&cfg.titles, "titles", "", `override titles, e.g. "uk=Title,pl=Tytuł"`)
	fs.StringVar(&cfg.start, "start", "", "YYYY-MM-DD")
	fs.StringVar(&cfg.end, "end", "", "YYYY-MM-DD")
	fs.StringVar(&cfg.gran, "granularity", "monthly", "monthly|daily")
	fs.StringVar(&cfg.projectSuffix, "project-suffix", "wikipedia", "wiki project suffix")
	out := fs.String("out", "report.pdf", "output PDF path")
	lang := fs.String("lang", "uk", "report language: uk|en")
	title := fs.String("title", "", "override report title")
	cache, noCache := cacheFlags(fs)
	fs.Parse(args)

	c := newClient(*cache, *noCache)
	a, err := buildAnalysis(cfg, c)
	if err != nil {
		return err
	}

	if err := renderReport(a, *out, *lang, *title); err != nil {
		return err
	}

	return printJSON(leanCopy(a))
}
