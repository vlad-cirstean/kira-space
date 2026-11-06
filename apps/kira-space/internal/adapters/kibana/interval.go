package kibana

import "time"

type interval struct {
	value    string
	ms       int64
	calendar bool
}

const (
	second = int64(time.Second / time.Millisecond)
	minute = 60 * second
	hour   = 60 * minute
	day    = 24 * hour
)

// niceIntervals is every bucket size offered, ascending. Calendar sizes use a nominal length only
// to count buckets; Elasticsearch buckets them on real calendar boundaries.
var niceIntervals = []interval{
	{"1s", second, false}, {"5s", 5 * second, false}, {"10s", 10 * second, false}, {"30s", 30 * second, false},
	{"1m", minute, false}, {"5m", 5 * minute, false}, {"10m", 10 * minute, false}, {"30m", 30 * minute, false},
	{"1h", hour, false}, {"3h", 3 * hour, false}, {"12h", 12 * hour, false}, {"1d", day, false},
	{"1w", 7 * day, true}, {"1M", 30 * day, true}, {"1y", 365 * day, true},
}

const (
	targetBuckets = 50
	maxBuckets    = 100
)

var intervalChoices = map[string]string{"m": "1m", "h": "1h", "d": "1d", "w": "1w", "M": "1M", "y": "1y"}

func bucketCount(spanMs, ms int64) int64 { return (spanMs + ms - 1) / ms }

// pickInterval resolves a histogram interval for the range. Auto takes the size whose bucket count
// is nearest 50 without passing 100; a chosen unit is used as is, then scaled up while it would
// pass 100 buckets, as Kibana does. The key is the Elasticsearch parameter name.
func pickInterval(fromMs, toMs int64, choice string) (key, value string) {
	span := max(toMs-fromMs, 1)
	start := -1
	if want, ok := intervalChoices[choice]; ok {
		for i, iv := range niceIntervals {
			if iv.value == want {
				start = i
			}
		}
	}
	best := len(niceIntervals) - 1
	if start >= 0 {
		best = start
		for best < len(niceIntervals)-1 && bucketCount(span, niceIntervals[best].ms) > maxBuckets {
			best++
		}
	} else {
		bestGap := int64(-1)
		for i, iv := range niceIntervals {
			n := bucketCount(span, iv.ms)
			if n > maxBuckets {
				continue
			}
			gap := n - targetBuckets
			if gap < 0 {
				gap = -gap
			}
			if bestGap < 0 || gap < bestGap {
				best, bestGap = i, gap
			}
		}
	}
	iv := niceIntervals[best]
	if iv.calendar {
		return "calendar_interval", iv.value
	}
	return "fixed_interval", iv.value
}
