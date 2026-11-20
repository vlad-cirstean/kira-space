package dockerflow

import (
	"context"
	"testing"

	"github.com/moby/moby/client"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/sessionrestore"
	"github.com/kirathecat/kira-studio/internal/docker"
)

// The aggregator reads Docker from the live tracker and restores the containers the user picked.
func TestSessionRestoreDocker(t *testing.T) {
	e := bootRestore(t)
	ctx := context.Background()
	solo := sleeper(e.d, "solo", nil)
	e.waitSnapshot(t, solo)
	e.kill(t, solo)
	e.gen.Bump()
	e.waitState(t, "pending restore", func(st docker.RestoreState) bool {
		return st.State == "pending" && e.ours(st)["solo"].Status == "restorable"
	})

	st, err := e.app.W.SessionRestore.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Docker.State != "pending" || e.ours(st.Docker)["solo"].Status != "restorable" {
		t.Fatalf("docker state = %+v", st.Docker)
	}
	st.Docker = e.onlyOurs(st.Docker)
	e.app.Contract(t, "session-restore-docker", "SessionRestoreService.State#docker", st, e.opts(map[string]string{"solo": solo})...)

	res, err := e.app.W.SessionRestore.Restore(ctx, sessionrestore.Args{Docker: &sessionrestore.DockerChoice{EngineID: st.Docker.EngineID, IDs: []string{solo}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Docker == nil || len(res.Docker.Items) != 1 || !res.Docker.Items[0].OK {
		t.Fatalf("restore = %+v", res.Docker)
	}
	waitUntil(t, "container running again", func() bool {
		r, err := e.d.Cli.ContainerInspect(ctx, solo, client.ContainerInspectOptions{})
		return err == nil && r.Container.State != nil && r.Container.State.Running
	})
}
