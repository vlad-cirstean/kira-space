//go:build !darwin || server

package main

import "github.com/kirathecat/kira-studio/internal/startupfail"

// refuseOldStudio is a no-op off macOS: only the retired Kira Studio.app bundle needs refusing.
func refuseOldStudio(*startupfail.Reporter) {}
