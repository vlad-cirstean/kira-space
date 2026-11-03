package scripts

import (
	"strings"
	"time"
	// Embeds the IANA zone data so a zone resolves on a host without it.
	_ "time/tzdata"

	"github.com/adhocore/gronx"
)

// Schedule bounds.
const (
	DefaultScheduleTimeout = "30m"
	MinScheduleTimeout     = time.Minute
	MaxScheduleTimeout     = 24 * time.Hour
	// MaxNextFires caps how many upcoming fires NextFires returns.
	MaxNextFires = 10
)

// Schedule makes a script recurring; nil on a script that is not.
type Schedule struct {
	Cron     string `json:"cron"`
	Timezone string `json:"timezone"`
	Enabled  bool   `json:"enabled"`
	// Confirm asks before each run.
	Confirm bool `json:"confirm"`
	// Timeout bounds a normal script's headless run; "" means DefaultScheduleTimeout.
	Timeout string `json:"timeout"`
	// Params are fixed non-secret values.
	Params map[string][]string `json:"params"`
	// TaskID targets a Kira Space ADE task; "" for none.
	TaskID   string `json:"taskId"`
	BranchID string `json:"branchId"`
}

// TimeoutDuration parses Timeout; Validate has already bounded it.
func (s Schedule) TimeoutDuration() time.Duration {
	d, err := time.ParseDuration(s.Timeout)
	if err != nil || d <= 0 {
		d, _ = time.ParseDuration(DefaultScheduleTimeout)
	}
	return d
}

// ValidCron checks a 5-field cron expression: no @ tags, seconds or years.
func ValidCron(expr string) error {
	expr = strings.TrimSpace(expr)
	if len(strings.Fields(expr)) != 5 || strings.HasPrefix(expr, "@") {
		return invalid("/schedule/cron", "backend.core.runs.cronFields.body")
	}
	if !gronx.IsValid(expr) {
		return invalid("/schedule/cron", "backend.core.runs.cronInvalid.body")
	}
	return nil
}

// NextFires returns the next n fire instants after `after` in the zone. A local time that does not
// exist in the zone is skipped, and a wall time that repeats fires once.
func NextFires(expr, tz string, after time.Time, n int) ([]time.Time, error) {
	if err := ValidCron(expr); err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, invalid("/schedule/timezone", "backend.core.runs.timezoneUnknown.body")
	}
	n = min(max(n, 0), MaxNextFires)
	expr = strings.TrimSpace(expr)
	out := make([]time.Time, 0, n)
	t := after.In(loc)
	for tries := 0; len(out) < n && tries < n*8+16; tries++ {
		next, err := gronx.NextTickAfter(expr, t, false)
		if err != nil {
			return nil, invalid("/schedule/cron", "backend.core.runs.cronInvalid.body")
		}
		t = next.In(loc)
		// Go shifts a nonexistent wall time forward: the shifted instant no longer matches the cron.
		if due, _ := gronx.New().IsDue(expr, t); !due || repeatedWall(t) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// repeatedWall reports whether t is the second occurrence of its wall time (a clock set back).
// time.Date resolves an ambiguous wall time to the first instant.
func repeatedWall(t time.Time) bool {
	y, mo, d := t.Date()
	h, mi, _ := t.Clock()
	return !time.Date(y, mo, d, h, mi, 0, 0, t.Location()).Equal(t.Truncate(time.Minute))
}

// validSchedule checks and normalises s against the script it belongs to.
func validSchedule(s *Schedule, kind string, params []Param) error {
	s.Cron = strings.TrimSpace(s.Cron)
	if err := ValidCron(s.Cron); err != nil {
		return err
	}
	if s.Timezone == "" {
		s.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return invalid("/schedule/timezone", "backend.core.runs.timezoneUnknown.body")
	}
	if err := validScheduleTimeout(s, kind); err != nil {
		return err
	}
	if err := validScheduleParams(s, params); err != nil {
		return err
	}
	if s.TaskID == "" {
		s.BranchID = ""
	}
	return nil
}

func validScheduleTimeout(s *Schedule, kind string) error {
	if kind == KindSmart {
		if s.Timeout != "" {
			return invalid("/schedule/timeout", "backend.core.runs.scheduleSmartTimeout.body")
		}
		return nil
	}
	if s.Timeout == "" {
		s.Timeout = DefaultScheduleTimeout
	}
	d, err := time.ParseDuration(s.Timeout)
	if err != nil || d < MinScheduleTimeout || d > MaxScheduleTimeout {
		return invalid("/schedule/timeout", "backend.core.runs.scheduleTimeout.body")
	}
	return nil
}

func validScheduleParams(s *Schedule, params []Param) error {
	if s.Params == nil {
		s.Params = map[string][]string{}
	}
	for name := range s.Params {
		for _, p := range params {
			if p.Name == name && p.Secret {
				return invalid("/schedule/params", "backend.core.runs.scheduleParamSecret.body", "name", name)
			}
		}
	}
	if _, _, err := ParamValues(params, s.Params); err != nil {
		return invalid("/schedule/params", "backend.core.runs.scheduleParams.body")
	}
	return nil
}
