package ade

import "testing"

// A call that arrives while a task's confirm job runs is kept as the latest and handed back at end;
// only the latest survives, and the job stays claimed until none is left.
func TestArtifactHostKeepsLatestCall(t *testing.T) {
	var h artifactHost
	first := confirmJob{servedGaps: nil}
	if !h.begin("t", first) {
		t.Fatal("first begin refused")
	}
	for _, n := range []int{1, 2} {
		job := confirmJob{set: repoShareSet{repoIDs: []string{string(rune('a' + n))}}}
		if h.begin("t", job) {
			t.Fatal("begin claimed a running task")
		}
	}
	if !h.begin("other", confirmJob{}) {
		t.Fatal("another task refused")
	}
	job, more := h.end("t")
	if !more || len(job.set.repoIDs) != 1 || job.set.repoIDs[0] != "c" {
		t.Fatalf("end = %+v %v, want the latest call", job, more)
	}
	if h.begin("t", confirmJob{}) {
		t.Fatal("task released while its follow-up pass is due")
	}
	if _, more := h.end("t"); !more {
		t.Fatal("the call kept during the follow-up pass was lost")
	}
	if _, more := h.end("t"); more {
		t.Fatal("nothing kept, yet end handed a job back")
	}
	if !h.begin("t", confirmJob{}) {
		t.Fatal("task not released")
	}
}
