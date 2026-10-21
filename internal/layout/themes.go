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

func (b *Builder) docBlocks(ctx context.Context, dir string, th Theme, blocks []model.Block) ([]map[string]any, []string, error) {
	out := make([]map[string]any, len(blocks))
	var warns []string
	for i, bl := range blocks {
		out[i] = bl.Doc()
		src, size, ok := bl.DiagramSource()
		if !ok {
			continue
		}
		path, svg, err := diagram.Cached(ctx, dir+"/diagrams", src, th.D2)
		if err != nil {
			return nil, nil, fmt.Errorf("diagram %s: %w", bl.ID, err)
		}
		w, h, pt := Fit(svg, size)
		out[i]["svg"] = "diagrams/" + path[strings.LastIndex(path, "/")+1:]
		out[i]["width_mm"] = w
		out[i]["height_mm"] = h
		if pt < minReadablePt {
			warns = append(warns, fmt.Sprintf("diagram %s is scaled down so its labels render at ~%.1fpt (unreadable below %.0fpt); simplify it, split it in two, change direction, or use size \"full\"", bl.ID, pt, minReadablePt))
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

func (b *Builder) PreviewPNG(ctx context.Context, src string, th Theme, size string) ([]byte, float64, float64, float64, error) {
	dir := filepath.Join(b.Home, "build", "_preview")
	path, svg, err := diagram.Cached(ctx, dir, src, th.D2)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	w, h, pt := Fit(svg, size)
	b.mu.Lock()
	defer b.mu.Unlock()
	typ := fmt.Sprintf("#set page(width: auto, height: auto, margin: 4mm)\n#image(%q, width: %.1fmm)\n", filepath.Base(path), w)
	if err := os.WriteFile(filepath.Join(dir, "preview.typ"), []byte(typ), 0o644); err != nil {
		return nil, 0, 0, 0, err
	}
	if _, err := b.typst(ctx, dir, "compile", "preview.typ", "preview.png", "--ppi", "100"); err != nil {
		return nil, 0, 0, 0, err
	}
	png, err := os.ReadFile(filepath.Join(dir, "preview.png"))
	return png, w, h, pt, err
}
