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
)

var penalty = map[string]float64{
	"overflow":           30,
	"almost_empty":       30,
	"under_filled":       25,
	"spill_over":         20,
	"orphan_heading":     20,
	"unreadable_diagram": 20,
	"pushed_block":       15,
	"oversized_diagram":  15,
	"monotone_stack":     10,
	"cramped":            10,
}

var insetTypes = map[string]bool{"callout": true, "qa": true}

var crampedNext = map[string]bool{"table": true, "diagram": true, "callout": true}

var crampedSkipPrev = map[string]bool{"heading": true, "spacer": true, "page_break": true, "divider": true}

var stackSkip = map[string]bool{"spacer": true, "page_break": true, "divider": true}
