package ade

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func goSpawn(wg *sync.WaitGroup) func(func()) {
	return func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
}

func TestOrderCacheStaleServedWhileOneRebuildRuns(t *testing.T) {
	var c orderCache
	var wg sync.WaitGroup
	spawn := goSpawn(&wg)
	now := time.Now()
	var builds atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	build := func() ([]string, bool) {
		if builds.Add(1) == 2 { // the background rebuild parks
			close(started)
			<-release
			return []string{"new"}, true
		}
		return []string{"old"}, true
	}
	if got := c.get(now, "d", build, spawn); !slices.Equal(got, []string{"old"}) {
		t.Fatalf("first call = %v", got)
	}
	c.drop()
	for range 10 { // the first call after the drop starts the rebuild; the rest join it
		if got := c.get(now, "d", build, spawn); !slices.Equal(got, []string{"old"}) {
			t.Fatalf("stale order = %v", got)
		}
	}
	<-started
	if got := builds.Load(); got != 2 {
		t.Fatalf("%d builds, want the first plus one rebuild", got)
	}
	close(release)
	wg.Wait()
	if got := c.get(now, "d", build, spawn); !slices.Equal(got, []string{"new"}) || builds.Load() != 2 {
		t.Fatalf("after the rebuild: %v with %d builds", got, builds.Load())
	}
}

func TestOrderCacheOlderGenNeverOverwritesNewer(t *testing.T) {
	var c orderCache
	var wg sync.WaitGroup
	spawn := goSpawn(&wg)
	now := time.Now()
	c.get(now, "d", func() ([]string, bool) { return []string{"v0"}, true }, spawn)
	c.drop()
	started, release := make(chan struct{}), make(chan struct{})
	c.get(now, "d", func() ([]string, bool) { // rebuild for gen 1 parks
		close(started)
		<-release
		return []string{"gen1"}, true
	}, spawn)
	<-started
	c.drop()
	c.mu.Lock()
	c.store([]string{"gen2"}, now, "d", c.gen.Load()) // a newer gen's order lands first
	c.mu.Unlock()
	close(release)
	wg.Wait()
	builds := 0
	got := c.get(now, "d", func() ([]string, bool) { builds++; return nil, false }, spawn)
	wg.Wait()
	if !slices.Equal(got, []string{"gen2"}) {
		t.Fatalf("older build overwrote the newer order: %v", got)
	}
}

func TestOrderCacheFirstCallsShareOneBuild(t *testing.T) {
	var c orderCache
	var wg sync.WaitGroup
	var builds atomic.Int32
	release := make(chan struct{})
	build := func() ([]string, bool) {
		builds.Add(1)
		<-release
		return []string{"x"}, true
	}
	got := make(chan []string, 5)
	for range 5 {
		go func() { got <- c.get(time.Now(), "d", build, goSpawn(&wg)) }()
	}
	for builds.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	for range 5 {
		if o := <-got; !slices.Equal(o, []string{"x"}) {
			t.Fatalf("order %v", o)
		}
	}
	if builds.Load() != 1 {
		t.Fatalf("%d builds for 5 first calls", builds.Load())
	}
}
