package redis

import (
	"reflect"
	"testing"
)

func TestPlanBounded(t *testing.T) {
	const want = 11
	length := func(cmd, key string) (int64, error) { return 100, nil }
	tests := []struct {
		name    string
		command string
		args    []string
		wantArg []string // nil: run as typed
	}{
		{"lrange all", "LRANGE", []string{"k", "0", "-1"}, []string{"k", "0", "10"}},
		{"lrange negative start", "LRANGE", []string{"k", "-50", "-1"}, []string{"k", "50", "60"}},
		{"lrange negative start past head", "LRANGE", []string{"k", "-500", "-1"}, []string{"k", "0", "10"}},
		{"lrange within cap", "LRANGE", []string{"k", "5", "9"}, nil},
		{"lrange empty range", "LRANGE", []string{"k", "9", "5"}, nil},
		{"lrange bad index", "LRANGE", []string{"k", "a", "5"}, nil},
		{"zrange rev withscores", "ZRANGE", []string{"z", "0", "-1", "REV", "WITHSCORES"}, []string{"z", "0", "10", "REV", "WITHSCORES"}},
		{"zrange byscore adds limit", "ZRANGE", []string{"z", "-inf", "+inf", "BYSCORE", "WITHSCORES"}, []string{"z", "-inf", "+inf", "BYSCORE", "WITHSCORES", "LIMIT", "0", "11"}},
		{"zrange bylex keeps small limit", "ZRANGE", []string{"z", "-", "+", "BYLEX", "LIMIT", "0", "5"}, nil},
		{"zrangebyscore unbounded limit lowered", "ZRANGEBYSCORE", []string{"z", "0", "9", "LIMIT", "2", "-1"}, []string{"z", "0", "9", "LIMIT", "2", "11"}},
		{"zrevrangebylex adds limit", "ZREVRANGEBYLEX", []string{"z", "+", "-"}, []string{"z", "+", "-", "LIMIT", "0", "11"}},
		{"xrange adds count", "XRANGE", []string{"s", "-", "+"}, []string{"s", "-", "+", "COUNT", "11"}},
		{"xrevrange lowers count", "XREVRANGE", []string{"s", "+", "-", "COUNT", "5000"}, []string{"s", "+", "-", "COUNT", "11"}},
		{"xrange keeps small count", "xrange", []string{"s", "-", "+", "COUNT", "3"}, nil},
		{"get untouched", "GET", []string{"k"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := planBounded(tc.command, tc.args, want, length)
			if err != nil {
				t.Fatal(err)
			}
			if plan.scan != nil {
				t.Fatalf("unexpected scan plan")
			}
			if !reflect.DeepEqual(plan.args, tc.wantArg) {
				t.Errorf("args = %v, want %v", plan.args, tc.wantArg)
			}
		})
	}

	scans := map[string][]int{"HGETALL": {0, 1}, "HKEYS": {0}, "HVALS": {1}}
	for cmd, emit := range scans {
		plan, _ := planBounded(cmd, []string{"h"}, want, length)
		if plan.scan == nil || plan.scan.head[0] != "HSCAN" || !reflect.DeepEqual(plan.scan.emit, emit) {
			t.Errorf("%s plan = %+v", cmd, plan.scan)
		}
	}
	if plan, _ := planBounded("KEYS", []string{"a*"}, want, length); plan.scan == nil || plan.scan.head[0] != "SCAN" {
		t.Errorf("KEYS plan = %+v", plan.scan)
	}
	if plan, _ := planBounded("KEYS", nil, want, length); plan.scan != nil || plan.args != nil {
		t.Errorf("KEYS without a pattern must run as typed, got %+v", plan)
	}
}
