//go:build darwin && !server

package main

import (
	"errors"
	"log/slog"

	"github.com/kirathecat/kira-studio/internal/metrics"
	"github.com/kirathecat/kira-studio/internal/startupfail"
)

// refuseOldStudio exits with an alert while a Kira Studio.app process runs: Kira Space now owns
// that data, and two writers would corrupt it. A failed process scan never blocks launch.
func refuseOldStudio(reporter *startupfail.Reporter) {
	pids, err := metrics.AppProcessSet([]string{"Kira Studio.app/Contents/MacOS"}, nil)
	if err != nil {
		slog.Warn("old Kira Studio check failed", "scope", "startup", "err", err)
		return
	}
	if len(pids) > 0 {
		reporter.Fatal(startupfail.StepOldStudio, errors.New("a Kira Studio.app process is running"))
	}
}
