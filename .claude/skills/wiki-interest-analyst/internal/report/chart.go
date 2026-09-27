// Package report renders analysis results into a shareable chart (PNG) and a
// one-page PDF. Chart labels use ASCII language codes so the plot renders with
// the default font on any machine; all human-language prose lives in the PDF,
// which embeds a Unicode font and handles Cyrillic.
package report

import (
	"fmt"
	"image/color"
	"sort"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"

	"wiki-interest-analyst/internal/wiki"
)

// palette is a set of visually distinct, colour-blind-friendly line colours.
var palette = []color.Color{
	color.RGBA{R: 0x1f, G: 0x77, B: 0xb4, A: 0xff}, // blue
	color.RGBA{R: 0xd6, G: 0x27, B: 0x28, A: 0xff}, // red
	color.RGBA{R: 0x2c, G: 0xa0, B: 0x2c, A: 0xff}, // green
	color.RGBA{R: 0xff, G: 0x7f, B: 0x0e, A: 0xff}, // orange
	color.RGBA{R: 0x94, G: 0x67, B: 0xbd, A: 0xff}, // purple
	color.RGBA{R: 0x8c, G: 0x56, B: 0x4b, A: 0xff}, // brown
}

// RenderChart draws one line per series (views over time) and saves a PNG.
// widthPx/heightPx are approximate pixel dimensions.
func RenderChart(series []*wiki.Series, path string, widthPx, heightPx int) error {
	p := plot.New()
	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Views (agent=user)"
	p.X.Tick.Marker = plot.TimeTicks{Format: "2006-01"}
	p.Add(plotter.NewGrid())

	for i, s := range series {
		pts := make(plotter.XYs, 0, len(s.Points))

		for _, pt := range s.Points {
			t, err := time.Parse("2006-01-02", pt.Date)
			if err != nil {
				continue
			}

			pts = append(pts, plotter.XY{X: float64(t.Unix()), Y: float64(pt.Views)})
		}

		if len(pts) == 0 {
			continue
		}

		sort.Slice(pts, func(a, b int) bool { return pts[a].X < pts[b].X })

		line, err := plotter.NewLine(pts)
		if err != nil {
			return err
		}

		c := palette[i%len(palette)]
		line.Color = c
		line.Width = vg.Points(1.6)
		p.Add(line)
		p.Legend.Add(s.Lang, line)
	}

	p.Legend.Top = true
	p.Legend.XOffs = -vg.Points(2)

	w := vg.Length(widthPx) * vg.Inch / 96
	h := vg.Length(heightPx) * vg.Inch / 96
	canvas, err := draw.NewFormattedCanvas(w, h, "png")
	if err != nil {
		return err
	}

	p.Draw(draw.New(canvas))

	f, err := create(path)
	if err != nil {
		return err
	}

	defer f.Close()
	if _, err := canvas.WriteTo(f); err != nil {
		return fmt.Errorf("writing chart png: %w", err)
	}

	return nil
}
