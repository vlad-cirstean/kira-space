package queryplan

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// decodeRawObject decodes raw as a JSON object into a map of its own raw field values — the
// "once into map[string]json.RawMessage" half of each parser's double decode. A non-object (or
// invalid JSON) yields an empty map rather than an error: a parser's caller has already committed
// to parsing raw as this dialect's plan, and a malformed sub-object should cost that one node's
// metrics, not the whole parse.
func decodeRawObject(raw json.RawMessage) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &out)
	return out
}

// formatMetricValue ports metricsFrom's own `Array.isArray(value) ? value.join(', ') :
// String(value)` — a JS array of scalars joins with ", "; anything else stringifies the way JS's
// String() does for a JSON-decoded value (a JSON number never carries a trailing ".0" the way
// Go's float64 default formatting would).
func formatMetricValue(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// Not valid JSON on its own (defensive only — every value here came from json.Unmarshal
		// of the parent object, so this cannot happen in practice) — the raw bytes are the best
		// available text.
		return string(raw)
	}
	return formatMetricScalar(v)
}

func formatMetricScalar(v any) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(val)
	case string:
		return val
	case float64:
		return formatJSNumber(val)
	case []any:
		parts := make([]string, len(val))
		for i, e := range val {
			parts[i] = formatMetricScalar(e)
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(val)
	}
}

// formatJSNumber ports Number.prototype.toString for a finite number: shortest round-trip digits,
// fixed notation for 1e-6 <= |n| < 1e21, JS-shaped exponent form (`1e+21`, `1.5e-7`) otherwise.
func formatJSNumber(n float64) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	// 'e' with precision -1 yields the shortest digits: "d.ddde±XX".
	repr := strconv.FormatFloat(n, 'e', -1, 64)
	mantissa, expText, _ := strings.Cut(repr, "e")
	exp, _ := strconv.Atoi(expText)
	digits := strings.Replace(mantissa, ".", "", 1)
	k := len(digits)
	point := exp + 1 // decimal point position relative to digits (ECMA-262 "n")
	switch {
	case k <= point && point <= 21:
		return sign + digits + strings.Repeat("0", point-k)
	case 0 < point && point <= 21:
		return sign + digits[:point] + "." + digits[point:]
	case -6 < point && point <= 0:
		return sign + "0." + strings.Repeat("0", -point) + digits
	}
	e := point - 1
	expSign := "+"
	if e < 0 {
		expSign, e = "-", -e
	}
	if k == 1 {
		return sign + digits + "e" + expSign + strconv.Itoa(e)
	}
	return sign + digits[:1] + "." + digits[1:] + "e" + expSign + strconv.Itoa(e)
}

// collectMetrics ports every parser's own metricsFrom/tableMetrics loop: every key of raw not in
// typedKeys becomes one Metric, sorted by label (the order Object.entries would have
// produced is not reproducible from a Go map, so this is the one difference the fixtures
// deliberately do not pin).
func collectMetrics(raw map[string]json.RawMessage, typedKeys map[string]bool) []Metric {
	out := make([]Metric, 0, len(raw))
	for key, value := range raw {
		if typedKeys[key] {
			continue
		}
		out = append(out, Metric{Label: key, Value: formatMetricValue(value)})
	}
	return sortMetricsByLabel(out)
}
