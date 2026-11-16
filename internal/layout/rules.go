package layout

const (
	UnderFilledPct       = 50.0
	SpillOverPct         = 15.0
	AlmostEmptyPct       = 5.0
	PushedBlockFillPct   = 70.0
	TargetFillPct        = 60.0
	OversizedDiagramFrac = 0.40
	MinHeadingGapMM      = 7.0
	MinBoxGapMM          = 5.2
	BoxInsetMM           = 2.8
	MonotoneRun          = 4
	MaxHints             = 6
	MaxCanvasMM          = 5000.0
	CanvasMarginMM       = 24.0
)

var penalty = map[string]float64{
	"overflow":           30,
	"almost_empty":       30,
	"under_filled":       25,
	"spill_over":         20,
	"orphan_heading":     20,
	"unreadable_diagram": 20,
	"pushed_block":       15,
	"wide_table":         20,
	"wide_code":          20,
	"block_overlap":      20,
	"node_overlap":       15,
	"wrong_cell":         15,
	"off_canvas":         15,
	"edge_through_node":  10,
	"edge_misroute":      10,
	"label_overflow":     5, // one crowded label can overlap several things
	"tiny_text":          5,
	"edge_crossing":      5,
	"empty_cell":         5,
	"aspect_extreme":     5,
	"oversized_diagram":  15,
	"monotone_stack":     10,
	"cramped":            10,
	"canvas_too_large":   15,
	"page_break_ignored": 5,
}

var insetTypes = map[string]bool{"callout": true, "qa": true}

var crampedNext = map[string]bool{"table": true, "diagram": true, "callout": true}

var crampedSkipPrev = map[string]bool{"heading": true, "spacer": true, "page_break": true, "divider": true}

var stackSkip = map[string]bool{"spacer": true, "page_break": true, "divider": true}
