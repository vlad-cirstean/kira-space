//go:build race

package taskfiles

// Race detector slows a stat pass about 2x; timing budgets scale by it.
const timingSlack = 3
