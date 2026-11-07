package diagram

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

func TestRenderAndRetarget(t *testing.T) {
	svg, err := Render(context.Background(), "a -> b: go", Style{Font: "Inter"})
	if err != nil {
		t.Fatal(err)
	}
	if svg.WidthPx == 0 || svg.HeightPx == 0 {
		t.Fatal("no size parsed")
	}
	if !regexp.MustCompile(`font-family: "d2-\d+-font-bold", "Inter"; font-weight: bold;`).Match(svg.Data) {
		t.Fatal("text must fall back from D2's embedded font (browsers) to the theme font (Typst)")
	}
	if !regexp.MustCompile(`@font-face \{\s*font-family: d2-\d+-font-bold; font-weight: bold;\s*src: url\("data:application/font-woff;base64,`).Match(svg.Data) {
		t.Fatal("the embedded bold face must declare its weight (or browsers fake-bold it) and keep its font data intact")
	}
}

func TestCompileErrorHasLine(t *testing.T) {
	_, err := Render(context.Background(), "a -> b\nc -> ", Style{})
	if err == nil || !strings.HasPrefix(err.Error(), "D2 error: 2:") {
		t.Fatalf("got %v", err)
	}
}

func TestCached(t *testing.T) {
	dir := t.TempDir()
	p1, _, err := Cached(context.Background(), dir, "x -> y", Style{})
	if err != nil {
		t.Fatal(err)
	}
	p2, s2, _ := Cached(context.Background(), dir, "x -> y", Style{})
	p3, _, _ := Cached(context.Background(), dir, "x -> y", Style{Sketch: true})
	if p1 != p2 || p1 == p3 || s2.WidthPx == 0 {
		t.Fatal("cache keys wrong")
	}
}

func TestDarkHasNoCanvas(t *testing.T) {
	dark, err := Render(context.Background(), "a -> b", Style{ThemeID: 200, Font: "Inter"})
	if err != nil {
		t.Fatal(err)
	}
	light, _ := Render(context.Background(), "a -> b", Style{Font: "Inter"})
	if canvas.Match(dark.Data) || !canvas.Match(light.Data) {
		t.Fatal("dark diagrams must drop the canvas rect; light ones keep it")
	}
}
