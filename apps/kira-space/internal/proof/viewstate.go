package proof

import "slices"

// Bounds on saved view state: sized so a hostile caller cannot grow a row without limit.
const (
	maxViewStateBytes = 64 << 10
	maxScroll         = 1e9
	maxSelectionIdx   = 10_000_000
	maxSelectionLen   = 10_000
	maxExpanded       = 10_000
	maxExpandedLen    = 256
	maxWidths         = 500
	maxWidth          = 100_000
	maxChartHidden    = 50
	maxChartHiddenLen = 512
	maxChartSeriesIdx = 49
)

// Selection mirrors the frontend's slick Selection union.
type Selection struct {
	Kind      string `json:"kind"`
	Row       *int   `json:"row,omitempty"`
	Col       *int   `json:"col,omitempty"`
	AnchorRow *int   `json:"anchorRow,omitempty"`
	AnchorCol *int   `json:"anchorCol,omitempty"`
	Rows      []int  `json:"rows,omitempty"`
	Cols      []int  `json:"cols,omitempty"`
}

// ViewState is where one tab was scrolled and what was highlighted, restored on every mount and
// matched by the screenshot (packages/shared/domain/proof.ts holds the zod twin).
type ViewState struct {
	QueryScrollTop   float64            `json:"queryScrollTop"`
	ResultScrollTop  float64            `json:"resultScrollTop"`
	ResultScrollLeft float64            `json:"resultScrollLeft"`
	Selection        *Selection         `json:"selection"`
	SelectedRow      *int               `json:"selectedRow"`
	Expanded         []string           `json:"expanded"`
	ColumnWidths     map[string]float64 `json:"columnWidths"`
	Chart            *ChartViewState    `json:"chart,omitempty"`
}

// ChartViewState is a chart step's legend, clicked point and zoom window (percent of the axis).
type ChartViewState struct {
	Hidden   []string    `json:"hidden"`
	Selected *ChartPoint `json:"selected"`
	Zoom     *ChartZoom  `json:"zoom"`
}

// ChartPoint addresses one point by series position and index within the series.
type ChartPoint struct {
	Series int `json:"series"`
	Index  int `json:"index"`
}

// ChartZoom is the visible x window, 0 <= Start <= End <= 100.
type ChartZoom struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

func (c *ChartViewState) check() error {
	if len(c.Hidden) > maxChartHidden || slices.ContainsFunc(c.Hidden, func(s string) bool { return len(s) > maxChartHiddenLen }) {
		return invalid("view state chart.hidden holds at most %d names of %d bytes", maxChartHidden, maxChartHiddenLen)
	}
	if c.Selected != nil && (c.Selected.Series < 0 || c.Selected.Series > maxChartSeriesIdx || !okIdx(c.Selected.Index)) {
		return invalid("view state chart.selected is out of range")
	}
	if z := c.Zoom; z != nil && !(z.Start >= 0 && z.Start <= z.End && z.End <= 100) {
		return invalid("view state chart.zoom needs 0 <= start <= end <= 100")
	}
	if c.Hidden == nil {
		c.Hidden = []string{}
	}
	return nil
}

func okIdx(n int) bool { return n >= 0 && n <= maxSelectionIdx }

func okIdxPtr(p *int) bool { return p != nil && okIdx(*p) }

func (s *Selection) check() error {
	bad := invalid("view state selection is not valid")
	switch s.Kind {
	case "cell":
		if !okIdxPtr(s.Row) || !okIdxPtr(s.Col) {
			return bad
		}
	case "range":
		if !okIdxPtr(s.Row) || !okIdxPtr(s.Col) || !okIdxPtr(s.AnchorRow) || !okIdxPtr(s.AnchorCol) {
			return bad
		}
	case "row":
		if len(s.Rows) == 0 || len(s.Rows) > maxSelectionLen || slices.ContainsFunc(s.Rows, func(n int) bool { return !okIdx(n) }) {
			return bad
		}
	case "column":
		if len(s.Cols) == 0 || len(s.Cols) > maxSelectionLen || slices.ContainsFunc(s.Cols, func(n int) bool { return !okIdx(n) }) {
			return bad
		}
	default:
		return bad
	}
	return nil
}

// Check validates v and fills the empty collections so a stored state decodes to non-null.
func (v *ViewState) Check() error {
	for _, f := range []float64{v.QueryScrollTop, v.ResultScrollTop, v.ResultScrollLeft} {
		if !(f >= 0 && f <= maxScroll) {
			return invalid("view state scroll offsets must be between 0 and %g", maxScroll)
		}
	}
	if v.SelectedRow != nil && !okIdx(*v.SelectedRow) {
		return invalid("view state selectedRow is out of range")
	}
	if v.Selection != nil {
		if err := v.Selection.check(); err != nil {
			return err
		}
	}
	if len(v.Expanded) > maxExpanded || slices.ContainsFunc(v.Expanded, func(s string) bool { return len(s) > maxExpandedLen }) {
		return invalid("view state expanded holds at most %d ids of %d bytes", maxExpanded, maxExpandedLen)
	}
	if len(v.ColumnWidths) > maxWidths {
		return invalid("view state columnWidths holds at most %d columns", maxWidths)
	}
	for k, w := range v.ColumnWidths {
		if len(k) > maxColumnName || !(w >= 0 && w <= maxWidth) {
			return invalid("view state column width is out of range")
		}
	}
	if v.Chart != nil {
		if err := v.Chart.check(); err != nil {
			return err
		}
	}
	if v.Expanded == nil {
		v.Expanded = []string{}
	}
	if v.ColumnWidths == nil {
		v.ColumnWidths = map[string]float64{}
	}
	return nil
}
