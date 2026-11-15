package proof

import (
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Chart limits. Bars and scatter points are one SVG node each, so their cap is lower than the path
// types, and the cap bounds the screenshot capture cost.
const (
	maxChartSeries     = 20
	maxCategories      = 1000
	maxPathPoints      = 20_000
	maxMarkPoints      = 1_000
	maxSlices          = 50
	maxSeriesNameRunes = 120
	maxCategoryBytes   = 256
	maxAxisNameRunes   = 80
	maxUnitRunes       = 16
	maxTimeMs          = 253402300799999 // 9999-12-31T23:59:59.999Z
)

// Chart types.
const (
	chartBar     = "bar"
	chartLine    = "line"
	chartArea    = "area"
	chartScatter = "scatter"
	chartPie     = "pie"
)

const (
	axisCategory = "category"
	axisTime     = "time"
	axisValue    = "value"
)

type chartIn struct {
	Type       *string         `json:"type"`
	X          *chartAxisIn    `json:"x"`
	Y          *chartYIn       `json:"y"`
	Stacked    *bool           `json:"stacked"`
	Horizontal *bool           `json:"horizontal"`
	Series     *[]chartSeries  `json:"series"`
	Unit       *string         `json:"unit"`
	Slices     *[]chartSliceIn `json:"slices"`
}

type chartAxisIn struct {
	Type *string `json:"type"`
	Name string  `json:"name"`
}

type chartYIn struct {
	Name string `json:"name"`
	Unit string `json:"unit"`
}

type chartSeries struct {
	Name   *string           `json:"name"`
	Points []json.RawMessage `json:"points"`
}

type chartSliceIn struct {
	Name  *string         `json:"name"`
	Value json.RawMessage `json:"value"`
}

// Stored shapes: every field present, so a reader never branches on absence.
type chartAxis struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type chartY struct {
	Name string `json:"name"`
	Unit string `json:"unit"`
}

// chartPoint is [x, y]: x a string (category), int64 epoch ms (time) or json.Number (value); y a
// json.Number or nil for a gap.
type chartPoint [2]any

type chartOutSeries struct {
	Name   string       `json:"name"`
	Points []chartPoint `json:"points"`
}

type chartCartesian struct {
	Type       string           `json:"type"`
	X          chartAxis        `json:"x"`
	Y          chartY           `json:"y"`
	Stacked    bool             `json:"stacked"`
	Horizontal bool             `json:"horizontal"`
	Series     []chartOutSeries `json:"series"`
}

type chartOutSlice struct {
	Name  string      `json:"name"`
	Value json.Number `json:"value"`
}

type chartPieOut struct {
	Type   string          `json:"type"`
	Unit   string          `json:"unit"`
	Slices []chartOutSlice `json:"slices"`
}

func checkChart(data json.RawMessage) (any, error) {
	var in chartIn
	if err := decodeStrict(data, &in); err != nil {
		return nil, err
	}
	if in.Type == nil {
		return nil, invalid("data.type is required: one of bar, line, area, scatter, pie")
	}
	switch *in.Type {
	case chartPie:
		return checkPie(in)
	case chartBar, chartLine, chartArea, chartScatter:
		return checkCartesian(in)
	}
	return nil, invalid("data.type must be one of bar, line, area, scatter, pie")
}

func checkPie(in chartIn) (any, error) {
	switch {
	case in.X != nil:
		return nil, invalid("data.x is not valid for type pie")
	case in.Y != nil:
		return nil, invalid("data.y is not valid for type pie; use data.unit")
	case in.Series != nil:
		return nil, invalid("data.series is not valid for type pie; use data.slices")
	case in.Stacked != nil:
		return nil, invalid("data.stacked is not valid for type pie")
	case in.Horizontal != nil:
		return nil, invalid("data.horizontal is not valid for type pie")
	}
	out := chartPieOut{Type: chartPie, Slices: []chartOutSlice{}}
	if in.Unit != nil {
		if err := checkLabel("data.unit", *in.Unit, maxUnitRunes); err != nil {
			return nil, err
		}
		out.Unit = *in.Unit
	}
	if in.Slices == nil || len(*in.Slices) < 1 || len(*in.Slices) > maxSlices {
		return nil, invalid("data.slices must hold 1-%d slices", maxSlices)
	}
	seen := map[string]bool{}
	sum := 0.0
	for i, s := range *in.Slices {
		if s.Name == nil {
			return nil, invalid("data.slices[%d].name is required", i)
		}
		if err := checkSeriesName("data.slices", i, *s.Name); err != nil {
			return nil, err
		}
		if seen[*s.Name] {
			return nil, invalid("data.slices[%d].name %q is a duplicate", i, *s.Name)
		}
		seen[*s.Name] = true
		num, f, err := parseNumber(s.Value)
		if err != nil || num == "" {
			return nil, invalid("data.slices[%d].value must be a finite number", i)
		}
		if f < 0 {
			return nil, invalid("data.slices[%d].value must be 0 or more", i)
		}
		sum += f
		out.Slices = append(out.Slices, chartOutSlice{Name: *s.Name, Value: json.Number(num)})
	}
	if !(sum > 0) || math.IsInf(sum, 0) {
		return nil, invalid("data.slices values must sum to more than 0 and stay finite")
	}
	return out, nil
}

func checkCartesian(in chartIn) (any, error) {
	typ := *in.Type
	if err := checkCartesianFlags(in, typ); err != nil {
		return nil, err
	}
	out := chartCartesian{Type: typ}
	out.Stacked = in.Stacked != nil && *in.Stacked
	out.Horizontal = in.Horizontal != nil && *in.Horizontal
	if err := checkAxes(in, &out); err != nil {
		return nil, err
	}
	axis := out.X.Type
	if in.Series == nil || len(*in.Series) < 1 || len(*in.Series) > maxChartSeries {
		return nil, invalid("data.series must hold 1-%d series", maxChartSeries)
	}
	pointCap := maxMarkPoints
	if typ == chartLine || typ == chartArea {
		pointCap = maxPathPoints
	}
	sorted := axis != axisCategory && (typ == chartLine || typ == chartArea)
	names := map[string]bool{}
	categories := map[string]bool{}
	total := 0
	out.Series = make([]chartOutSeries, 0, len(*in.Series))
	for si, s := range *in.Series {
		if err := checkSeriesHead(si, s, names); err != nil {
			return nil, err
		}
		total += len(s.Points)
		if total > pointCap {
			return nil, invalid("data.series hold more than %d points in total; the limit for type %s is %d", pointCap, typ, pointCap)
		}
		pts, keys, err := parsePoints(si, s.Points, axis, categories)
		if err != nil {
			return nil, err
		}
		if sorted {
			pts = sortByKey(pts, keys)
		}
		out.Series = append(out.Series, chartOutSeries{Name: *s.Name, Points: pts})
	}
	if len(categories) > maxCategories {
		return nil, invalid("data.series use %d distinct categories; the limit is %d", len(categories), maxCategories)
	}
	return out, nil
}

// checkCartesianFlags rejects pie-only fields and flags invalid for typ.
func checkCartesianFlags(in chartIn, typ string) error {
	switch {
	case in.Slices != nil:
		return invalid("data.slices is only valid for type pie; use data.series")
	case in.Unit != nil:
		return invalid("data.unit is only valid for type pie; use data.y.unit")
	case in.Stacked != nil && *in.Stacked && typ != chartBar && typ != chartArea:
		return invalid("data.stacked is only valid for type bar and area")
	case in.Horizontal != nil && *in.Horizontal && typ != chartBar:
		return invalid("data.horizontal is only valid for type bar")
	}
	return nil
}

// checkAxes validates the x and y axis and stores them in out.
func checkAxes(in chartIn, out *chartCartesian) error {
	if in.X == nil || in.X.Type == nil {
		return invalid("data.x.type is required: one of category, time, value")
	}
	axis := *in.X.Type
	if axis != axisCategory && axis != axisTime && axis != axisValue {
		return invalid("data.x.type must be one of category, time, value")
	}
	if err := checkLabel("data.x.name", in.X.Name, maxAxisNameRunes); err != nil {
		return err
	}
	out.X = chartAxis{Type: axis, Name: in.X.Name}
	if in.Y != nil {
		if err := checkLabel("data.y.name", in.Y.Name, maxAxisNameRunes); err != nil {
			return err
		}
		if err := checkLabel("data.y.unit", in.Y.Unit, maxUnitRunes); err != nil {
			return err
		}
		out.Y = chartY{Name: in.Y.Name, Unit: in.Y.Unit}
	}
	return nil
}

// checkSeriesHead validates one series' name and point presence, recording the name in names.
func checkSeriesHead(si int, s chartSeries, names map[string]bool) error {
	if s.Name == nil {
		return invalid("data.series[%d].name is required", si)
	}
	if err := checkSeriesName("data.series", si, *s.Name); err != nil {
		return err
	}
	if names[*s.Name] {
		return invalid("data.series[%d].name %q is a duplicate", si, *s.Name)
	}
	names[*s.Name] = true
	if len(s.Points) < 1 {
		return invalid("data.series[%d].points must hold at least 1 point", si)
	}
	return nil
}

// sortByKey returns pts stably ordered by keys.
func sortByKey(pts []chartPoint, keys []float64) []chartPoint {
	idx := make([]int, len(pts))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int {
		switch {
		case keys[a] < keys[b]:
			return -1
		case keys[a] > keys[b]:
			return 1
		}
		return 0
	})
	ordered := make([]chartPoint, len(pts))
	for i, j := range idx {
		ordered[i] = pts[j]
	}
	return ordered
}

// parsePoints validates one series; keys carries each point's numeric x for sorting.
func parsePoints(si int, raw []json.RawMessage, axis string, categories map[string]bool) ([]chartPoint, []float64, error) {
	pts := make([]chartPoint, 0, len(raw))
	keys := make([]float64, 0, len(raw))
	inSeries := map[string]bool{}
	for pi, r := range raw {
		var pair []json.RawMessage
		if err := json.Unmarshal(r, &pair); err != nil || len(pair) != 2 {
			return nil, nil, invalid("data.series[%d].points[%d] must be [x, y]", si, pi)
		}
		var pt chartPoint
		key := 0.0
		switch axis {
		case axisCategory:
			var s string
			if err := json.Unmarshal(pair[0], &s); err != nil || s == "" || len(s) > maxCategoryBytes || !utf8.ValidString(s) {
				return nil, nil, invalid("data.series[%d].points[%d][0] must be a category label of 1-%d bytes", si, pi, maxCategoryBytes)
			}
			if inSeries[s] {
				return nil, nil, invalid("data.series[%d].points[%d][0] category %q is a duplicate in its series", si, pi, s)
			}
			inSeries[s] = true
			categories[s] = true
			pt[0] = s
		case axisTime:
			ms, err := parseTime(pair[0])
			if err != nil {
				return nil, nil, invalid("data.series[%d].points[%d][0] %v", si, pi, err)
			}
			pt[0], key = ms, float64(ms)
		default:
			num, f, err := parseNumber(pair[0])
			if err != nil || num == "" {
				return nil, nil, invalid("data.series[%d].points[%d][0] must be a finite number", si, pi)
			}
			pt[0], key = json.Number(num), f
		}
		num, _, err := parseNumber(pair[1])
		if err != nil {
			return nil, nil, invalid("data.series[%d].points[%d][1] must be a finite number or null", si, pi)
		}
		if num != "" {
			pt[1] = json.Number(num)
		}
		pts = append(pts, pt)
		keys = append(keys, key)
	}
	return pts, keys, nil
}

// parseNumber reads a JSON number literal; "" with no error means null. A quoted number is refused.
func parseNumber(raw json.RawMessage) (text string, f float64, err error) {
	s := strings.TrimSpace(string(raw))
	if s == "null" {
		return "", 0, nil
	}
	if s == "" || (s[0] != '-' && (s[0] < '0' || s[0] > '9')) {
		return "", 0, invalid("not a number")
	}
	f, perr := strconv.ParseFloat(s, 64)
	if perr != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return "", 0, invalid("not a finite number")
	}
	return s, f, nil
}

// parseTime accepts RFC 3339, a UTC date (YYYY-MM-DD) or integer epoch milliseconds.
func parseTime(raw json.RawMessage) (int64, error) {
	s := strings.TrimSpace(string(raw))
	const msg = "must be an RFC 3339 time, a YYYY-MM-DD date or integer epoch milliseconds, within years 1970-9999"
	var ms int64
	if strings.HasPrefix(s, `"`) {
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			return 0, invalid(msg)
		}
		t, err := time.Parse(time.RFC3339Nano, str)
		if err != nil {
			if t, err = time.Parse(time.DateOnly, str); err != nil {
				return 0, invalid(msg)
			}
		}
		ms = t.UnixMilli()
	} else {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, invalid(msg)
		}
		ms = n
	}
	if ms < 0 || ms > maxTimeMs {
		return 0, invalid(msg)
	}
	return ms, nil
}

func checkLabel(field, s string, maxRunes int) error {
	if err := CheckText(field, s, maxRunes); err != nil {
		return err
	}
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return invalid("%s must not contain control characters", field)
	}
	return nil
}

func checkSeriesName(field string, i int, name string) error {
	if n := utf8.RuneCountInString(name); n < 1 || n > maxSeriesNameRunes || !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return invalid("%s[%d].name must be one line of 1-%d characters", field, i, maxSeriesNameRunes)
	}
	return nil
}
