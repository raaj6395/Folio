package layout

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/raaj6395/folio/internal/diagram"
	"github.com/raaj6395/folio/internal/model"
)

type Theme struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Paper       string        `json:"default_paper"`
	D2          diagram.Style `json:"-"`
}

var Themes = []Theme{
	{"clean", "Clean typeset look: Inter, crisp boxes, plain paper.", "plain", diagram.Style{ThemeID: 0, Font: "Inter"}},
	{"notebook", "Handwritten notebook: Kalam handwriting font, ruled paper with a red margin line, hand-drawn (sketch) diagrams.", "ruled", diagram.Style{ThemeID: 0, Sketch: true, Font: "Kalam"}},
	{"dotgrid", "Bullet-journal look: clean Inter fonts on dot-grid paper.", "dotted", diagram.Style{ThemeID: 0, Font: "Inter"}},
}

var Modes = []string{"dark", "light"}

const d2DarkTheme = 200

func (t Theme) D2For(mode string) diagram.Style {
	st := t.D2
	if mode == "dark" {
		st.ThemeID = d2DarkTheme
	}
	return st
}

func ThemeByName(name string) (Theme, error) {
	i := slices.IndexFunc(Themes, func(t Theme) bool { return t.Name == name })
	if i < 0 {
		var names []string
		for _, t := range Themes {
			names = append(names, t.Name)
		}
		return Theme{}, fmt.Errorf("unknown theme %q; valid themes: %s (see list_themes)", name, strings.Join(names, ", "))
	}
	return Themes[i], nil
}

const (
	textWidthMM      = 174.0
	maxDiagramHeight = 0.6 * contentHeightMM
	pxToMM           = 25.4 / 96
	d2LabelPt        = 12.0
	maxUpscale       = 0.9
	minReadablePt    = 6.0
)

var sizeFrac = map[string]float64{"full": 1, "half": 0.5, "small": 0.35}

func (b *Builder) docBlocks(ctx context.Context, dir string, style diagram.Style, blocks []model.Block) ([]map[string]any, []Issue, error) {
	out := make([]map[string]any, len(blocks))
	var warns []Issue
	var walk func(m map[string]any) error
	walk = func(m map[string]any) error {
		for _, side := range []string{"left", "right"} {
			if kids, ok := m[side].([]map[string]any); ok {
				for _, k := range kids {
					if err := walk(k); err != nil {
						return err
					}
				}
			}
		}
		if m["type"] != "diagram" {
			return nil
		}
		id := m["id"].(string)
		path, svg, err := diagram.Cached(ctx, filepath.Join(dir, "diagrams"), m["source"].(string), style)
		if err != nil {
			return fmt.Errorf("diagram %s: %w", id, err)
		}
		w, h, pt := Fit(svg, m["size"].(string))
		m["svg"] = "diagrams/" + filepath.Base(path)
		m["width_mm"] = w
		m["height_mm"] = h
		if pt < minReadablePt {
			warns = append(warns, Issue{Check: "unreadable_diagram", Blocks: []string{id}, Problem: fmt.Sprintf("diagram %s is scaled down so its labels render at ~%.1fpt (unreadable below %.0fpt)", id, pt, minReadablePt),
				Fix: `simplify it (at most ~6 nodes), split it in two, change direction, or use size "full"`})
		}
		return nil
	}
	for i, bl := range blocks {
		out[i] = bl.Doc()
		if err := walk(out[i]); err != nil {
			return nil, nil, err
		}
	}
	return out, warns, nil
}

func Fit(svg *diagram.SVG, size string) (wmm, hmm, labelPt float64) {
	natural := svg.WidthPx * pxToMM
	if natural == 0 || svg.HeightPx == 0 {
		return textWidthMM * sizeFrac[size], 0, d2LabelPt
	}
	aspect := svg.HeightPx / svg.WidthPx
	wmm = min(textWidthMM*sizeFrac[size], natural*maxUpscale)
	if wmm*aspect > maxDiagramHeight {
		wmm = maxDiagramHeight / aspect
	}
	return round1(wmm), round1(wmm * aspect), round1(d2LabelPt * wmm / natural)
}

func (b *Builder) PreviewPNG(ctx context.Context, src string, th Theme, mode, size string) ([]byte, float64, float64, float64, error) {
	dir := filepath.Join(b.Home, "build", "_preview")
	path, svg, err := diagram.Cached(ctx, dir, src, th.D2For(mode))
	if err != nil {
		return nil, 0, 0, 0, err
	}
	w, h, pt := Fit(svg, size)
	b.mu.Lock()
	defer b.mu.Unlock()
	bg := "white"
	if mode == "dark" {
		bg = `rgb("#141925")`
	}
	typ := fmt.Sprintf("#set page(width: auto, height: auto, margin: 4mm, fill: %s)\n#image(%q, width: %.1fmm)\n", bg, filepath.Base(path), w)
	base := strings.TrimSuffix(filepath.Base(path), ".svg") + "-" + size + "-" + mode
	if err := os.WriteFile(filepath.Join(dir, base+".typ"), []byte(typ), 0o644); err != nil {
		return nil, 0, 0, 0, err
	}
	if _, err := b.typst(ctx, dir, "compile", base+".typ", base+".png", "--ppi", "100"); err != nil {
		return nil, 0, 0, 0, err
	}
	png, err := os.ReadFile(filepath.Join(dir, base+".png"))
	return png, w, h, pt, err
}
