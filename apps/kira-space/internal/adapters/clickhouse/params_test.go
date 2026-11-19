package clickhouse

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestBoundParamsEncoding(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`[1,2]`, paramRaw + `[1,2]`},
		{`["a","it's","b\\c","line\nbreak","é"]`, paramRaw + "['a','it\\'s','b\\\\c','line\nbreak','é']"},
		{`[true,false]`, paramRaw + `[true,false]`},
		{`12.50`, `12.50`},
		{`"x"`, `x`},
		{`null`, paramNull},
	}
	for _, c := range cases {
		got, err := boundParams([]model.BindParam{{Name: "p", Label: "{{kept.p}}", Value: json.RawMessage(c.raw)}})
		if err != nil || got["p"] != c.want {
			t.Errorf("boundParams(%s) = %q, %v; want %q", c.raw, got["p"], err, c.want)
		}
	}
}

func TestBoundParamsCap(t *testing.T) {
	big := json.RawMessage(`"` + strings.Repeat("a", maxBoundParamBytes) + `"`)
	if _, err := boundParams([]model.BindParam{{Name: "p", Label: "x", Value: big}}); err == nil {
		t.Fatal("values over the cap accepted")
	}
}
