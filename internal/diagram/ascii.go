package diagram

import (
	"math"
	"strings"
)

// ASCII draws a coarse map of the diagram, at most cols×rows characters: nodes as +-| boxes
// with their label's first line inside, edges as * along their path. A character is about
// twice as tall as it is wide, so rows are halved to keep the shape.
func ASCII(g *Geometry, cols, rows int) string {
	if g.W <= 0 || g.H <= 0 {
		return ""
	}
	sx := float64(cols-1) / g.W
	if h := int(math.Round(g.H*sx/2)) + 1; h > rows {
		sx = float64(rows-1) * 2 / g.H
	}
	sy := sx / 2
	w, h := int(math.Round(g.W*sx))+1, int(math.Round(g.H*sy))+1
	grid := make([][]rune, h)
	for i := range grid {
		grid[i] = []rune(strings.Repeat(" ", w))
	}
	set := func(x, y int, c rune) {
		if y >= 0 && y < h && x >= 0 && x < w {
			grid[y][x] = c
		}
	}
	for _, e := range g.Edges {
		p := e.Path()
		for k := 1; k < len(p); k++ {
			x0, y0, x1, y1 := p[k-1][0]*sx, p[k-1][1]*sy, p[k][0]*sx, p[k][1]*sy
			n := int(math.Max(math.Abs(x1-x0), math.Abs(y1-y0))) + 1
			for s := 0; s <= n; s++ {
				t := float64(s) / float64(n)
				set(int(math.Round(x0+(x1-x0)*t)), int(math.Round(y0+(y1-y0)*t)), '*')
			}
		}
	}
	// Containers first (they come before their children in D2's order), so children draw on top.
	for _, n := range g.Nodes {
		x0, y0 := int(math.Round(n.X*sx)), int(math.Round(n.Y*sy))
		x1, y1 := max(x0+2, int(math.Round(n.right()*sx))), max(y0+2, int(math.Round(n.bottom()*sy)))
		for x := x0; x <= x1; x++ {
			set(x, y0, '-')
			set(x, y1, '-')
		}
		for y := y0; y <= y1; y++ {
			set(x0, y, '|')
			set(x1, y, '|')
		}
		for _, c := range [][2]int{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
			set(c[0], c[1], '+')
		}
		if !n.Container {
			for y := y0 + 1; y < y1; y++ {
				for x := x0 + 1; x < x1; x++ {
					set(x, y, ' ')
				}
			}
		}
		first, _, _ := strings.Cut(n.Label, "\n")
		text := []rune(first)
		room := x1 - x0 - 1
		if room <= 0 || len(text) == 0 {
			continue
		}
		if len(text) > room {
			text = text[:room]
		}
		ly := y0 + 1
		if !n.Container {
			ly = (y0 + y1) / 2
		}
		lx := x0 + 1 + (room-len(text))/2
		for i, c := range text {
			set(lx+i, ly, c)
		}
	}
	lines := make([]string, h)
	for i, r := range grid {
		lines[i] = strings.TrimRight(string(r), " ")
	}
	return strings.Join(lines, "\n")
}
