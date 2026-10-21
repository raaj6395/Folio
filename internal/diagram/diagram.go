package diagram

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	d, _, err := d2lib.Compile(ctx, src, co, ro)
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
	w, h := size(out)
	return &SVG{Data: out, WidthPx: w, HeightPx: h}, nil
}

var canvas = regexp.MustCompile(`<rect x="[-\d.]+" y="[-\d.]+" width="[\d.]+" height="[\d.]+" rx="[\d.]+" fill="#[0-9A-Fa-f]+" class=" fill-N7" stroke-width="0" />`)

var (
	fontFace   = regexp.MustCompile(`@font-face\s*\{[^}]*\}`)
	d2Family   = regexp.MustCompile(`font-family:\s*"?d2-\d+-font-(\w+)"?`)
	styleStart = []byte("<![CDATA[")
)

func retarget(svg []byte, font string) []byte {
	if font == "" {
		return svg
	}
	svg = fontFace.ReplaceAll(svg, nil)
	svg = d2Family.ReplaceAllFunc(svg, func(m []byte) []byte {
		decl := fmt.Sprintf(`font-family: "%s"`, font)
		switch d2Family.FindSubmatch(m)[1][0] {
		case 'b':
			decl += "; font-weight: bold"
		case 'i':
			decl += "; font-style: italic"
		}
		return []byte(decl)
	})
	if i := bytes.Index(svg, styleStart); i >= 0 {
		i += len(styleStart)
		svg = append(svg[:i:i], append([]byte(fmt.Sprintf(`text{font-family:"%s";}`, font)), svg[i:]...)...)
	}
	return svg
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

func size(svg []byte) (float64, float64) {
	m := viewBox.FindSubmatch(svg)
	if m == nil {
		return 0, 0
	}
	w, _ := strconv.ParseFloat(string(m[3]), 64)
	h, _ := strconv.ParseFloat(string(m[4]), 64)
	return w, h
}

func Key(src string, st Style) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("r3|%d|%t|%s|%s", st.ThemeID, st.Sketch, st.Font, src)))
	return hex.EncodeToString(sum[:8])
}

func Cached(ctx context.Context, dir, src string, st Style) (string, *SVG, error) {
	path := filepath.Join(dir, Key(src, st)+".svg")
	if data, err := os.ReadFile(path); err == nil {
		w, h := size(data)
		return path, &SVG{Data: data, WidthPx: w, HeightPx: h}, nil
	}
	svg, err := Render(ctx, src, st)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	return path, svg, os.WriteFile(path, svg.Data, 0o644)
}
