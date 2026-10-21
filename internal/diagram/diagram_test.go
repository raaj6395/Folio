package diagram

import (
	"bytes"
	"context"
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
	if bytes.Contains(svg.Data, []byte("@font-face")) || !bytes.Contains(svg.Data, []byte(`font-family: "Inter"; font-weight: bold`)) {
		t.Fatal("fonts not retargeted")
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
