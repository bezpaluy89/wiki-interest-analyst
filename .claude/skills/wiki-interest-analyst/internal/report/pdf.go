package report

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/go-pdf/fpdf"

	"wiki-interest-analyst/internal/analyze"
)

// DejaVuSans ships with the skill (permissive licence) so the PDF renders
// Cyrillic and other Unicode text on any machine, with no system font needed.
//
//go:embed fonts/DejaVuSans.ttf
var dejaVu []byte

// create is a small shared helper (used by chart.go too).
func create(path string) (*os.File, error) { return os.Create(path) }

// RenderPDF writes a one-page A4 report: title, chart, data-driven findings and
// an explicit assumptions/limitations box. lang selects the prose language
// ("uk" or "en"); title overrides the default heading when non-empty.
func RenderPDF(a analyze.Analysis, chartPath, outPath, lang, title string) error {
	if lang != "uk" && lang != "en" {
		lang = "uk"
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(tr(lang, "doc_title"), true)
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(false, 15) // one page by design
	pdf.AddPage()

	const font = "DejaVu"
	pdf.AddUTF8FontFromBytes(font, "", dejaVu)
	pdf.SetFont(font, "", 18)

	if title == "" {
		title = fmt.Sprintf("%s: %s", tr(lang, "doc_title"), a.Theme)
	}

	pdf.SetTextColor(20, 20, 20)
	pdf.MultiCell(0, 8, title, "", "L", false)

	pdf.SetFont(font, "", 10)
	pdf.SetTextColor(90, 90, 90)
	period := fmt.Sprintf("%s: %s — %s · %s: %s",
		tr(lang, "period"), a.Start, a.End, tr(lang, "granularity"), a.Granularity)
	pdf.MultiCell(0, 5, period, "", "L", false)
	pdf.Ln(2)

	// Chart.
	if chartPath != "" {
		if _, err := os.Stat(chartPath); err == nil {
			y := pdf.GetY()
			pdf.ImageOptions(chartPath, 15, y, 180, 0, false,
				fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, 0, "")
			pdf.SetY(y + 92)
		}
	}

	// Findings.
	pdf.SetFont(font, "", 13)
	pdf.SetTextColor(20, 20, 20)
	pdf.MultiCell(0, 7, tr(lang, "findings"), "", "L", false)
	pdf.SetFont(font, "", 10)
	pdf.SetTextColor(40, 40, 40)

	for _, line := range Findings(a, lang) {
		pdf.MultiCell(0, 5, "•  "+line, "", "L", false)
	}

	pdf.Ln(2)

	// Assumptions & limitations box.
	pdf.SetFont(font, "", 11)
	pdf.SetTextColor(20, 20, 20)
	pdf.MultiCell(0, 6, tr(lang, "limitations"), "", "L", false)
	pdf.SetFont(font, "", 9)
	pdf.SetTextColor(70, 70, 70)
	pdf.SetFillColor(245, 245, 245)

	for _, line := range limitationLines(lang) {
		pdf.MultiCell(0, 4.5, "–  "+line, "", "L", false)
	}

	// Footer.
	pdf.SetY(-15)
	pdf.SetFont(font, "", 8)
	pdf.SetTextColor(120, 120, 120)
	pdf.MultiCell(0, 4, tr(lang, "source")+" · "+a.GeneratedAt, "", "C", false)

	if err := pdf.OutputFileAndClose(outPath); err != nil {
		return fmt.Errorf("writing pdf: %w", err)
	}

	return nil
}

// Findings turns the analysis into factual, data-grounded bullet lines. Because
// they are generated from computed metrics (not free-written), the report stays
// reproducible and defensible even on a weak model.
func Findings(a analyze.Analysis, lang string) []string {
	var out []string
	for _, s := range a.Series {
		var dir string
		switch s.Direction {
		case analyze.DirGrowing:
			dir = tr(lang, "growing")
		case analyze.DirDeclining:
			dir = tr(lang, "declining")
		default:
			dir = tr(lang, "flat")
		}

		line := fmt.Sprintf("%s (%s): %s %+.0f%%, %s ~%.0f/%s, R²=%.2f. %s: %s.",
			strings.ToUpper(s.Lang), s.Article, dir, s.GrowthPct,
			tr(lang, "baseline"), s.MedianViews, tr(lang, "bucket"), s.R2,
			tr(lang, "trust"), trustText(lang, s.Trust))

		if reasons := reasonsText(lang, s.TrustReasons); reasons != "" {
			line += " (" + reasons + ")"
		}

		out = append(out, line)
	}

	if a.Comparison.Leader != "" && len(a.Series) > 1 {
		key := "leader_rel"
		for _, s := range a.Series {
			if s.Lang == a.Comparison.Leader && s.Direction == analyze.DirGrowing {
				key = "leader"
			}
		}

		out = append(out, fmt.Sprintf("%s: %s.",
			tr(lang, key), strings.ToUpper(a.Comparison.Leader)))
	}

	for _, code := range a.Comparison.Notes {
		if t := reasonText(lang, code); t != "" {
			out = append(out, "⚠ "+t)
		}
	}

	return out
}
