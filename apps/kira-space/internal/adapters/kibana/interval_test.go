package kibana

import (
	"testing"
	"time"
)

func TestPickInterval(t *testing.T) {
	ms := func(d time.Duration) int64 { return int64(d / time.Millisecond) }
	cases := []struct {
		name   string
		span   time.Duration
		choice string
		key    string
		value  string
	}{
		{"auto one minute", time.Minute, "auto", "fixed_interval", "1s"},
		{"auto fifteen minutes", 15 * time.Minute, "auto", "fixed_interval", "30s"},
		{"auto one hour", time.Hour, "auto", "fixed_interval", "1m"},
		{"auto one day", 24 * time.Hour, "auto", "fixed_interval", "30m"},
		{"auto one week", 7 * 24 * time.Hour, "auto", "fixed_interval", "3h"},
		{"auto thirty days", 30 * 24 * time.Hour, "auto", "fixed_interval", "12h"},
		{"auto one year", 365 * 24 * time.Hour, "auto", "calendar_interval", "1w"},
		{"auto century", 100 * 365 * 24 * time.Hour, "auto", "calendar_interval", "1y"},
		{"auto zero span", 0, "auto", "fixed_interval", "1s"},
		{"unknown choice is auto", time.Hour, "x", "fixed_interval", "1m"},
		{"hour fits", 24 * time.Hour, "h", "fixed_interval", "1h"},
		{"minute scales up", 24 * time.Hour, "m", "fixed_interval", "30m"},
		{"minute at the limit", 100 * time.Minute, "m", "fixed_interval", "1m"},
		{"minute past the limit", 101 * time.Minute, "m", "fixed_interval", "5m"},
		{"week is calendar", 90 * 24 * time.Hour, "w", "calendar_interval", "1w"},
		{"day scales to week", 400 * 24 * time.Hour, "d", "calendar_interval", "1w"},
		{"year caps", 200 * 365 * 24 * time.Hour, "y", "calendar_interval", "1y"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key, value := pickInterval(0, ms(c.span), c.choice)
			if key != c.key || value != c.value {
				t.Errorf("pickInterval(%s, %q) = %s %s, want %s %s", c.span, c.choice, key, value, c.key, c.value)
			}
		})
	}
}
