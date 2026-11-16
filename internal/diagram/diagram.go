package diagram

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/raaj6395/folio"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

type Style struct {
	ThemeID int64
	Sketch  bool
	Font    string
}

type SVG struct {
	Data     []byte
	WidthPx  float64
	HeightPx float64
	Geom     *Geometry
}

var (
	rulerOnce sync.Once
	ruler     *textmeasure.Ruler
	rulerErr  error
	mu        sync.Mutex
)

func Render(ctx context.Context, src string, st Style) (*SVG, error) {
	rulerOnce.Do(func() {
		if rulerErr = registerFonts(); rulerErr == nil {
			ruler, rulerErr = textmeasure.NewRuler()
		}
	})
	if rulerErr != nil {
		return nil, rulerErr
	}
	mu.Lock()
	defer mu.Unlock()
	pad := int64(8)
	ro := &d2svg.RenderOpts{Pad: &pad, ThemeID: &st.ThemeID, Sketch: &st.Sketch}
	co := &d2lib.CompileOptions{
		Ruler:          ruler,
		FontFamily:     families[st.Font],
		LayoutResolver: func(string) (d2graph.LayoutGraph, error) { return d2dagrelayout.DefaultLayout, nil },
	}
	ctx = log.WithDefault(ctx)
	d, g, err := d2lib.Compile(ctx, src, co, ro)
	if err != nil {
		return nil, fmt.Errorf("D2 error: %s", strings.TrimSpace(err.Error()))
	}
	out, err := d2svg.Render(d, ro)
	if err != nil {
		return nil, fmt.Errorf("D2 render error: %v", err)
	}
	out = retarget(out, st.Font)
	if st.ThemeID >= 200 {
		out = canvas.ReplaceAll(out, nil)
	}
	x, y, w, h := viewport(out)
	return &SVG{Data: out, WidthPx: w, HeightPx: h, Geom: geometry(d, g, x, y, w, h)}, nil
}

var canvas = regexp.MustCompile(`<rect x="[-\d.]+" y="[-\d.]+" width="[\d.]+" height="[\d.]+" rx="[\d.]+" fill="#[0-9A-Fa-f]+" class=" fill-N7" stroke-width="0" />`)

var (
	fontFace   = regexp.MustCompile(`@font-face\s*\{[^}]*\}`)
	faceFamily = regexp.MustCompile(`font-family:\s*d2-\d+-font-(\w+);`)
	d2Family   = regexp.MustCompile(`font-family:\s*"(d2-\d+-font-(\w+))"`)
	styleStart = []byte("<![CDATA[")
)

// retarget makes diagram text use the theme font in both renderers.
// Typst ignores @font-face, so each rule falls back from D2's embedded subset font to the theme font, which Typst has.
// Browsers (a canvas SVG embeds this SVG as an image) have no theme font, so they use the embedded subset.
// The faces get their weight/style so browsers don't fake bold on an already bold face.
func retarget(svg []byte, font string) []byte {
	if font == "" {
		return svg
	}
	svg = fontFace.ReplaceAllFunc(svg, func(face []byte) []byte {
		return faceFamily.ReplaceAllFunc(face, func(m []byte) []byte {
			if v := variant(faceFamily.FindSubmatch(m)[1]); v != "" {
				return []byte(string(m) + " " + v + ";")
			}
			return m
		})
	})
	svg = d2Family.ReplaceAllFunc(svg, func(m []byte) []byte {
		sub := d2Family.FindSubmatch(m)
		decl := fmt.Sprintf(`font-family: "%s", "%s"`, sub[1], font)
		if v := variant(sub[2]); v != "" {
			decl += "; " + v
		}
		return []byte(decl)
	})
	if i := bytes.Index(svg, styleStart); i >= 0 {
		i += len(styleStart)
		svg = append(svg[:i:i], append([]byte(fmt.Sprintf(`text{font-family:"%s";}`, font)), svg[i:]...)...)
	}
	return svg
}

// variant is the CSS for a D2 font kind ("regular", "bold", "italic", "semibold").
func variant(kind []byte) string {
	switch kind[0] {
	case 'b':
		return "font-weight: bold"
	case 'i':
		return "font-style: italic"
	}
	return ""
}

var families = map[string]*d2fonts.FontFamily{}

var familyFiles = map[string][4]string{
	"Inter": {"inter/Inter-Regular.ttf", "inter/Inter-Italic.ttf", "inter/Inter-Bold.ttf", "inter/Inter-SemiBold.ttf"},
	"Kalam": {"kalam/Kalam-Regular.ttf", "kalam/Kalam-Regular.ttf", "kalam/Kalam-Bold.ttf", "kalam/Kalam-Bold.ttf"},
}

func registerFonts() error {
	for name, files := range familyFiles {
		var ttf [4][]byte
		for i, f := range files {
			data, err := fs.ReadFile(folio.Fonts, "fonts/"+f)
			if err != nil {
				return err
			}
			ttf[i] = data
		}
		fam, err := d2fonts.AddFontFamily(name, ttf[0], ttf[1], ttf[2], ttf[3])
		if err != nil {
			return fmt.Errorf("registering %s with D2: %w", name, err)
		}
		families[name] = fam
	}
	return nil
}

var viewBox = regexp.MustCompile(`viewBox="([-\d.]+) ([-\d.]+) ([\d.]+) ([\d.]+)"`)

// viewport is the image size and the D2 coordinates of its top left: D2 nests its drawing in an
// inner <svg> whose viewBox starts at the bounding box minus the padding.
func viewport(svg []byte) (x, y, w, h float64) {
	ms := viewBox.FindAllSubmatch(svg, 2)
	if len(ms) == 0 {
		return 0, 0, 0, 0
	}
	f := func(b []byte) float64 { v, _ := strconv.ParseFloat(string(b), 64); return v }
	w, h = f(ms[0][3]), f(ms[0][4])
	if len(ms) > 1 {
		x, y = f(ms[1][1]), f(ms[1][2])
	}
	return x, y, w, h
}

func Key(src string, st Style) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("r4|%d|%t|%s|%s", st.ThemeID, st.Sketch, st.Font, src)))
	return hex.EncodeToString(sum[:8])
}

// Cached renders src once and keeps the SVG and its geometry (<key>.svg, <key>.json) in dir.
func Cached(ctx context.Context, dir, src string, st Style) (string, *SVG, error) {
	path := filepath.Join(dir, Key(src, st)+".svg")
	geomPath := strings.TrimSuffix(path, ".svg") + ".json"
	if data, err := os.ReadFile(path); err == nil {
		if gj, err := os.ReadFile(geomPath); err == nil {
			var g Geometry
			if json.Unmarshal(gj, &g) == nil {
				_, _, w, h := viewport(data)
				return path, &SVG{Data: data, WidthPx: w, HeightPx: h, Geom: &g}, nil
			}
		}
	}
	svg, err := Render(ctx, src, st)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	gj, _ := json.Marshal(svg.Geom)
	if err := os.WriteFile(geomPath, gj, 0o644); err != nil {
		return "", nil, err
	}
	return path, svg, os.WriteFile(path, svg.Data, 0o644)
}
