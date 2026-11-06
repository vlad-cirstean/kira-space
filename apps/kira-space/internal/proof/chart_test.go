package proof

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// catPoints builds a JSON points array of n category points c0..c(n-1) with y = i.
func catPoints(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf(`["c%d",%d]`, i, i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func timePoints(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf(`[%d,%d]`, 1000+i, i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func series(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"name":"s%d","points":[["a",1]]}`, i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestCheckChart(t *testing.T) {
	const ms = `1768478400000`
	timeLine := func(x string) string {
		return `{"type":"line","x":{"type":"time"},"series":[{"name":"s","points":[[` + x + `,1]]}]}`
	}
	cases := []struct {
		name, in string
		// want is the stored payload; wantErr a substring of the rejection. Exactly one is set.
		want, wantErr string
	}{
		{name: "rfc3339 Z", in: timeLine(`"2026-01-15T12:00:00Z"`), want: `[[` + ms + `,1]]`},
		{name: "rfc3339 offset", in: timeLine(`"2026-01-15T13:00:00+01:00"`), want: `[[` + ms + `,1]]`},
		{name: "rfc3339 fraction", in: timeLine(`"2026-01-15T12:00:00.500Z"`), want: `[[1768478400500,1]]`},
		{name: "date only", in: timeLine(`"2026-01-15"`), want: `[[1768435200000,1]]`},
		{name: "epoch ms", in: timeLine(ms), want: `[[` + ms + `,1]]`},
		{name: "fractional epoch", in: timeLine(`1.5`), wantErr: "epoch milliseconds"},
		{name: "year 10000", in: timeLine(`"10000-01-01T00:00:00Z"`), wantErr: "1970-9999"},
		{name: "before 1970", in: timeLine(`"1969-12-31T23:59:59Z"`), wantErr: "1970-9999"},
		{name: "stacked on line", in: `{"type":"line","stacked":true,"x":{"type":"time"},"series":[{"name":"s","points":[[1,1]]}]}`, wantErr: "data.stacked"},
		{name: "stacked false on line", in: `{"type":"line","stacked":false,"x":{"type":"time"},"series":[{"name":"s","points":[[1,1]]}]}`, want: `[[1,1]]`},
		{name: "horizontal on area", in: `{"type":"area","horizontal":true,"x":{"type":"time"},"series":[{"name":"s","points":[[1,1]]}]}`, wantErr: "data.horizontal"},
		{name: "slices on bar", in: `{"type":"bar","x":{"type":"category"},"slices":[{"name":"a","value":1}],"series":[{"name":"s","points":[["a",1]]}]}`, wantErr: "data.slices"},
		{name: "unit on bar", in: `{"type":"bar","unit":"req","x":{"type":"category"},"series":[{"name":"s","points":[["a",1]]}]}`, wantErr: "data.unit"},
		{name: "series on pie", in: `{"type":"pie","series":[],"slices":[{"name":"a","value":1}]}`, wantErr: "data.series"},
		{name: "unknown key", in: `{"type":"pie","slice":[],"slices":[{"name":"a","value":1}]}`, wantErr: "unknown field"},
		{name: "missing type", in: `{"x":{"type":"time"}}`, wantErr: "data.type"},
		{name: "missing x", in: `{"type":"bar","series":[{"name":"s","points":[["a",1]]}]}`, wantErr: "data.x.type"},
		{name: "duplicate category", in: `{"type":"bar","x":{"type":"category"},"series":[{"name":"s","points":[["a",1],["a",2]]}]}`, wantErr: "duplicate in its series"},
		{name: "same category across series", in: `{"type":"bar","x":{"type":"category"},"series":[{"name":"s","points":[["a",1]]},{"name":"t","points":[["a",2]]}]}`, want: `[["a",1]]`},
		{name: "1001 categories", in: `{"type":"bar","x":{"type":"category"},"series":[{"name":"s","points":` + catPoints(1001) + `}]}`, wantErr: "1000"},
		{name: "1001 bar points", in: `{"type":"bar","x":{"type":"value"},"series":[{"name":"s","points":` + timePoints(1001) + `}]}`, wantErr: "1000"},
		{name: "2001 line points", in: `{"type":"line","x":{"type":"value"},"series":[{"name":"s","points":` + timePoints(2001) + `}]}`, want: `[[1000,0]`},
		{name: "20001 line points", in: `{"type":"line","x":{"type":"value"},"series":[{"name":"s","points":` + timePoints(20001) + `}]}`, wantErr: "20000"},
		{name: "21 series", in: `{"type":"bar","x":{"type":"category"},"series":` + series(21) + `}`, wantErr: "1-20"},
		{name: "duplicate series name", in: `{"type":"bar","x":{"type":"category"},"series":[{"name":"s","points":[["a",1]]},{"name":"s","points":[["a",1]]}]}`, wantErr: "duplicate"},
		{name: "string y", in: `{"type":"bar","x":{"type":"category"},"series":[{"name":"s","points":[["a","NaN"]]}]}`, wantErr: "finite number or null"},
		{name: "quoted number y", in: `{"type":"bar","x":{"type":"category"},"series":[{"name":"s","points":[["a","5"]]}]}`, wantErr: "finite number or null"},
		{name: "overflow y", in: `{"type":"bar","x":{"type":"category"},"series":[{"name":"s","points":[["a",1e999]]}]}`, wantErr: "finite number or null"},
		{name: "null y kept", in: `{"type":"line","x":{"type":"value"},"series":[{"name":"s","points":[[1,null]]}]}`, want: `[[1,null]]`},
		{name: "point of three", in: `{"type":"bar","x":{"type":"category"},"series":[{"name":"s","points":[["a",1,2]]}]}`, wantErr: "[x, y]"},
		{name: "line sorted", in: `{"type":"line","x":{"type":"value"},"series":[{"name":"s","points":[[3,1],[1,2],[2,3]]}]}`, want: `[[1,2],[2,3],[3,1]]`},
		{name: "bar not sorted", in: `{"type":"bar","x":{"type":"value"},"series":[{"name":"s","points":[[3,1],[1,2],[2,3]]}]}`, want: `[[3,1],[1,2],[2,3]]`},
		{name: "scatter not sorted", in: `{"type":"scatter","x":{"type":"value"},"series":[{"name":"s","points":[[3,1],[1,2]]}]}`, want: `[[3,1],[1,2]]`},
		{name: "number text kept", in: `{"type":"bar","x":{"type":"value"},"series":[{"name":"s","points":[[1.50,12345678901234567890]]}]}`, want: `[[1.50,12345678901234567890]]`},
		{name: "pie ok", in: `{"type":"pie","unit":"req","slices":[{"name":"a","value":5},{"name":"b","value":0}]}`, want: `"slices":[{"name":"a","value":5},{"name":"b","value":0}]`},
		{name: "pie all zero", in: `{"type":"pie","slices":[{"name":"a","value":0}]}`, wantErr: "sum to more than 0"},
		{name: "pie negative", in: `{"type":"pie","slices":[{"name":"a","value":-1},{"name":"b","value":4}]}`, wantErr: "0 or more"},
		{name: "pie duplicate", in: `{"type":"pie","slices":[{"name":"a","value":1},{"name":"a","value":1}]}`, wantErr: "duplicate"},
		{name: "pie none", in: `{"type":"pie","slices":[]}`, wantErr: "1-50"},
		{name: "defaults filled", in: `{"type":"bar","x":{"type":"category"},"series":[{"name":"s","points":[["a",1]]}]}`, want: `"x":{"type":"category","name":""},"y":{"name":"","unit":""},"stacked":false,"horizontal":false`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := checkChart(json.RawMessage(tc.in))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			raw, merr := json.Marshal(out)
			if merr != nil {
				t.Fatal(merr)
			}
			if !strings.Contains(string(raw), tc.want) {
				t.Errorf("payload = %s, want containing %s", raw, tc.want)
			}
		})
	}
}

func TestChartViewStateCheck(t *testing.T) {
	zoom := func(s, e float64) *ChartZoom { return &ChartZoom{Start: s, End: e} }
	cases := []struct {
		name    string
		chart   ChartViewState
		wantErr string
	}{
		{name: "empty", chart: ChartViewState{}},
		{name: "full", chart: ChartViewState{Hidden: []string{"a"}, Selected: &ChartPoint{Series: 1, Index: 4}, Zoom: zoom(25, 75)}},
		{name: "start after end", chart: ChartViewState{Zoom: zoom(80, 20)}, wantErr: "chart.zoom"},
		{name: "end over 100", chart: ChartViewState{Zoom: zoom(0, 101)}, wantErr: "chart.zoom"},
		{name: "negative start", chart: ChartViewState{Zoom: zoom(-1, 5)}, wantErr: "chart.zoom"},
		{name: "series 50", chart: ChartViewState{Selected: &ChartPoint{Series: 50}}, wantErr: "chart.selected"},
		{name: "negative index", chart: ChartViewState{Selected: &ChartPoint{Index: -1}}, wantErr: "chart.selected"},
		{name: "51 hidden", chart: ChartViewState{Hidden: make([]string, 51)}, wantErr: "chart.hidden"},
		{name: "long hidden", chart: ChartViewState{Hidden: []string{strings.Repeat("x", 513)}}, wantErr: "chart.hidden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := ViewState{Chart: &tc.chart}
			err := v.Check()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if v.Chart.Hidden == nil {
				t.Error("hidden stays nil, want []")
			}
		})
	}
}
