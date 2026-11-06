// Package kiratime is P103 Part 3 §6.4's hoist of both apps' own storage/model/time.go —
// byte-identical in each, no dependencies of its own.
package kiratime

import "time"

// jsISOFormat matches JavaScript's Date.prototype.toISOString() byte for byte: UTC, exactly
// three fractional digits, literal Z. time.RFC3339Nano is NOT this — it trims trailing zeros,
// so "…:05.1Z" would sort after "…:05.05Z" lexicographically while being earlier in time, and
// op_log.started_at (written by the Node engine's toISOString()) is compared lexicographically
// by pruneOps's retention cut. Every TEXT timestamp Go writes must use this format.
const jsISOFormat = "2006-01-02T15:04:05.000Z"

// NowISO returns the current time formatted exactly as JavaScript's toISOString() would.
func NowISO() string {
	return FormatISO(time.Now())
}

// FormatISO formats t exactly as JavaScript's toISOString() would — used for any TEXT timestamp
// derived from something other than "now" (e.g. a retention cutoff computed by subtracting a
// duration).
func FormatISO(t time.Time) string {
	return t.UTC().Format(jsISOFormat)
}

// ParseISO parses a timestamp written by FormatISO/NowISO (or by the Node engine's own
// toISOString()). It also accepts RFC3339Nano as a fallback, since not every producer of an
// "ISO" timestamp trims to exactly three fractional digits.
func ParseISO(s string) (time.Time, error) {
	if t, err := time.Parse(jsISOFormat, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// NowMs returns the current time as unix milliseconds, the form INTEGER instant columns store.
func NowMs() int64 { return time.Now().UnixMilli() }

// ISOToMs converts an ISO timestamp to unix milliseconds at the repo boundary.
func ISOToMs(s string) (int64, error) {
	t, err := ParseISO(s)
	if err != nil {
		return 0, err
	}
	return t.UnixMilli(), nil
}

// MsToISO is the inverse of ISOToMs: it returns the wire form of a stored instant.
func MsToISO(ms int64) string { return FormatISO(time.UnixMilli(ms)) }

// ISOOrZero is ISOToMs for a value that is always written by this code; a malformed one reads as 0.
func ISOOrZero(s string) int64 {
	ms, err := ISOToMs(s)
	if err != nil {
		return 0
	}
	return ms
}
