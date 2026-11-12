package ade

import (
	"testing"
	"time"
)

func TestOrderCacheDropDuringBuild(t *testing.T) {
	var c orderCache
	now := time.Now()
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan []string)
	go func() {
		done <- c.get(now, "d", func() ([]string, bool) {
			close(started)
			<-release
			return []string{"stale"}, true
		})
	}()
	<-started
	// A drop while a build runs returns at once and keeps that build's result out of the cache.
	c.drop()
	close(release)
	if got := <-done; len(got) != 1 {
		t.Fatalf("overtaken build returned %v", got)
	}
	builds := 0
	build := func() ([]string, bool) { builds++; return []string{"fresh"}, true }
	if got := c.get(now, "d", build); got[0] != "fresh" || builds != 1 {
		t.Fatalf("after a drop the order = %v with %d builds, want one fresh build", got, builds)
	}
	c.get(now.Add(leaseOrderFresh-time.Nanosecond), "d", build)
	if builds != 1 {
		t.Fatalf("a fresh order was built again: %d builds", builds)
	}
	c.drop()
	c.get(now, "d", build)
	if builds != 2 {
		t.Fatalf("a dropped order was served: %d builds", builds)
	}
}
