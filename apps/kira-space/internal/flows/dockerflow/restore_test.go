package dockerflow

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// The engine is shared with every other test, so each assertion looks only at this run's prefixed containers.

type restoreEnv struct {
	app *flowharness.App
	d   *flowharness.Docker
	gen *flowharness.BootGen
}

func bootRestore(t *testing.T) restoreEnv {
	t.Helper()
	d := flowharness.RequireDocker(t)
	gen := &flowharness.BootGen{}
	app := flowharness.New(t, flowharness.WithDockerOptions(
		docker.WithRestoreTiming(200*time.Millisecond, time.Second, 2*time.Second),
		docker.WithBootProbe(gen.Probe()),
	))
	return restoreEnv{app: app, d: d, gen: gen}
}

func (e restoreEnv) state(t *testing.T) docker.RestoreState {
	t.Helper()
	st, err := e.app.W.Docker.RestoreState()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// ours keeps this run's items, keyed by name without the run prefix.
func (e restoreEnv) ours(st docker.RestoreState) map[string]docker.RestoreItem {
	out := map[string]docker.RestoreItem{}
	for _, it := range st.Items {
		if strings.HasPrefix(it.Name, e.d.Prefix()) {
			out[strings.TrimPrefix(it.Name, e.d.Prefix())] = it
		}
	}
	return out
}

func (e restoreEnv) onlyOurs(st docker.RestoreState) docker.RestoreState {
	items := []docker.RestoreItem{}
	for _, it := range e.ours(st) {
		items = append(items, it)
	}
	st.Items = sortedItems(items)
	return st
}

func sortedItems(items []docker.RestoreItem) []docker.RestoreItem {
	for i := range items {
		for j := i + 1; j < len(items); j++ {
			if items[j].ComposeProject < items[i].ComposeProject ||
				(items[j].ComposeProject == items[i].ComposeProject && items[j].Name < items[i].Name) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	return items
}

func hasOpenOurs(e restoreEnv, st docker.RestoreState) bool {
	for _, it := range e.ours(st) {
		if it.Status == "restorable" || it.Status == "waiting" {
			return true
		}
	}
	return false
}

func (e restoreEnv) waitState(t *testing.T, what string, pred func(docker.RestoreState) bool) docker.RestoreState {
	t.Helper()
	var st docker.RestoreState
	waitUntil(t, what, func() bool {
		st = e.state(t)
		return pred(st)
	})
	return st
}

func (e restoreEnv) inSnapshot(id string) bool {
	var n int
	_ = e.app.StudioDB().QueryRow(`SELECT COUNT(*) FROM docker_restore_containers WHERE container_id = ?`, id).Scan(&n)
	return n > 0
}

func (e restoreEnv) waitSnapshot(t *testing.T, ids ...string) {
	t.Helper()
	waitUntil(t, "containers in the restore snapshot", func() bool {
		for _, id := range ids {
			if !e.inSnapshot(id) {
				return false
			}
		}
		return true
	})
}

func (e restoreEnv) kill(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if _, err := e.d.Cli.ContainerKill(context.Background(), id, client.ContainerKillOptions{Signal: "KILL"}); err != nil {
			t.Fatal(err)
		}
		// Kill returns once the signal is sent; a tick that still sees the container running swallows the boot change.
		waitUntil(t, "killed container to stop", func() bool {
			res, err := e.d.Cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
			return err == nil && res.Container.State != nil && !res.Container.State.Running
		})
	}
}

func (e restoreEnv) opts(ids map[string]string, extra ...flowharness.ContractOption) []flowharness.ContractOption {
	opts := []flowharness.ContractOption{
		flowharness.Mask("engineId", "context", "detectedAt", "bootAt"),
		flowharness.Replace(e.d.Prefix(), "kira-flow-"),
		flowharness.Replace(runIDOf(e.d), "run"),
	}
	for name, id := range ids {
		opts = append(opts, flowharness.Replace(id, "id-"+name))
	}
	return append(opts, extra...)
}

func restartPolicy(name container.RestartPolicyMode) *container.HostConfig {
	return &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: name}}
}

func keepAlive(d *flowharness.Docker, name string, policy container.RestartPolicyMode) string {
	stop := 1
	return d.RunWith(name, container.Config{Cmd: []string{"sh", "-c", "exec sleep 300"}, StopTimeout: &stop}, restartPolicy(policy), nil)
}

func startedAt(t *testing.T, e restoreEnv, id string) time.Time {
	t.Helper()
	det, err := e.app.W.Docker.InspectContainer(docker.IDArgs{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339Nano, det.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestRestoreAfterEngineRestart(t *testing.T) {
	e := bootRestore(t)
	d, svc := e.d, e.app.W.Docker
	proj := "shop-" + runIDOf(d)
	compose := func(service string) map[string]string {
		return map[string]string{"com.docker.compose.project": proj, "com.docker.compose.service": service}
	}
	webLabels := compose("web")
	webLabels["com.docker.compose.depends_on"] = "db:service_started:false"

	solo := sleeper(d, "solo", nil)
	keeper := keepAlive(d, "keeper", "always")
	doomed := sleeper(d, "doomed", nil)
	// Names sort against dependency order, so only a depends_on-aware restore starts db first.
	web := sleeper(d, "shop-a-web", webLabels)
	db := sleeper(d, "shop-b-db", compose("db"))
	tc := sleeper(d, "tc", map[string]string{"org.testcontainers": "true"})
	e.waitSnapshot(t, solo, keeper, doomed, web, db)
	if e.inSnapshot(tc) {
		t.Fatal("testcontainers container was snapshotted")
	}
	if st := e.state(t); hasOpenOurs(e, st) || st.State == "pending" && len(e.ours(st)) > 0 {
		t.Fatalf("state before the restart = %+v", st)
	}
	mark := e.app.Events.Mark()

	e.kill(t, solo, web, db)
	if _, err := d.Cli.ContainerRemove(context.Background(), doomed, client.ContainerRemoveOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	e.gen.Bump()
	st := e.state(t)
	if st.State != "pending" {
		t.Fatalf("state = %+v, want pending", st)
	}
	want := map[string]string{"solo": "restorable", "keeper": "back", "doomed": "gone", "shop-a-web": "restorable", "shop-b-db": "restorable"}
	got := e.ours(st)
	if len(got) != len(want) {
		t.Fatalf("items = %+v, want %v", got, want)
	}
	for name, status := range want {
		if got[name].Status != status {
			t.Fatalf("%s = %q, want %q", name, got[name].Status, status)
		}
	}
	ids := map[string]string{"solo": solo, "keeper": keeper, "doomed": doomed, "web": web, "db": db}
	e.app.StudioContract(t, "docker-restore", "DockerService.RestoreState#pending", e.onlyOurs(st), e.opts(ids)...)

	args := docker.RestoreArgs{EngineID: st.EngineID, IDs: []string{solo, web, db}}
	e.app.StudioContract(t, "docker-restore", "args:DockerService.RestoreContainers", args, e.opts(ids)...)
	res, err := svc.RestoreContainers(args)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		if !it.OK {
			t.Fatalf("restore %s: %s", it.Name, it.Error)
		}
	}
	e.app.StudioContract(t, "docker-restore", "DockerService.RestoreContainers", res, e.opts(ids)...)
	if !startedAt(t, e, db).Before(startedAt(t, e, web)) {
		t.Fatal("db started after web; want depends_on order")
	}

	e.waitState(t, "restore to settle", func(s docker.RestoreState) bool { return !hasOpenOurs(e, s) })
	var sawPending, sawSettled bool
	var pendingEv flowharness.Event
	for _, ev := range e.app.Events.Since(mark, docker.ChannelRestore) {
		s, ok := ev.Data.(docker.RestoreState)
		if !ok {
			continue
		}
		if s.State == "pending" && e.ours(s)["solo"].Status == "restorable" && !sawPending {
			sawPending, pendingEv = true, ev
		}
		if sawPending && !hasOpenOurs(e, s) {
			sawSettled = true
		}
	}
	if !sawPending || !sawSettled {
		t.Fatalf("restore events pending=%v settled=%v", sawPending, sawSettled)
	}
	pendingEv.Data = e.onlyOurs(pendingEv.Data.(docker.RestoreState))
	e.app.StudioContractEvent(t, "docker-restore", pendingEv, e.opts(ids)...)
}

func TestRestoreSnapshotSurvivesIdleEngine(t *testing.T) {
	e := bootRestore(t)
	solo := sleeper(e.d, "solo", nil)
	other := sleeper(e.d, "other", nil)
	e.waitSnapshot(t, solo, other)
	e.kill(t, solo, other)
	e.gen.Bump()
	st := e.state(t)
	if got := e.ours(st); st.State != "pending" || got["solo"].Status != "restorable" || got["other"].Status != "restorable" {
		t.Fatalf("state = %+v", st)
	}

	time.Sleep(1500 * time.Millisecond)
	if got := e.ours(e.state(t)); got["solo"].Status != "restorable" || got["other"].Status != "restorable" {
		t.Fatalf("items after idle ticks = %+v, want the frozen snapshot", got)
	}

	e.app.Restart()
	if got := e.ours(e.state(t)); len(got) != 2 {
		t.Fatalf("items after relaunch = %+v, want the pending snapshot from the DB", got)
	}
}

func TestRestoreAtLaunch(t *testing.T) {
	e := bootRestore(t)
	solo := sleeper(e.d, "solo", nil)
	e.waitSnapshot(t, solo)
	e.app.Quit(t)
	e.kill(t, solo)
	e.gen.Bump()
	e.app.Restart()
	st := e.waitState(t, "pending at launch", func(s docker.RestoreState) bool { return s.State == "pending" })
	if got := e.ours(st)["solo"]; got.Status != "restorable" {
		t.Fatalf("solo = %+v, want restorable", got)
	}
}

func TestRestoreIgnoresUserStop(t *testing.T) {
	e := bootRestore(t)
	solo := sleeper(e.d, "solo", nil)
	e.waitSnapshot(t, solo)
	e.kill(t, solo)
	waitUntil(t, "stopped container to leave the snapshot after grace", func() bool { return !e.inSnapshot(solo) })
	e.gen.Bump()
	if got := e.ours(e.state(t)); len(got) != 0 {
		t.Fatalf("items = %+v, want none for a deliberate stop", got)
	}
}

func TestRestoreShutdownCasualty(t *testing.T) {
	e := bootRestore(t)
	solo := sleeper(e.d, "solo", nil)
	e.waitSnapshot(t, solo)
	e.kill(t, solo)
	e.gen.Bump()
	if got := e.ours(e.state(t))["solo"]; got.Status != "restorable" {
		t.Fatalf("solo = %+v, want restorable inside grace", got)
	}
}

func TestRestoreWaitingPolicy(t *testing.T) {
	e := bootRestore(t)
	keeper := keepAlive(e.d, "keeper", "always")
	e.waitSnapshot(t, keeper)
	if _, err := e.d.Cli.ContainerStop(context.Background(), keeper, client.ContainerStopOptions{}); err != nil {
		t.Fatal(err)
	}
	e.gen.Bump()
	if got := e.ours(e.state(t))["keeper"]; got.Status != "waiting" {
		t.Fatalf("keeper = %+v, want waiting", got)
	}
	e.waitState(t, "keeper to turn restorable", func(s docker.RestoreState) bool {
		return e.ours(s)["keeper"].Status == "restorable"
	})
}

func TestRestoreDismissAndConflict(t *testing.T) {
	e := bootRestore(t)
	svc := e.app.W.Docker
	solo := sleeper(e.d, "solo", nil)
	keeper := keepAlive(e.d, "keeper", "no")
	e.waitSnapshot(t, solo, keeper)
	e.kill(t, solo)
	e.gen.Bump()
	st := e.state(t)
	first := e.ours(st)
	if st.State != "pending" || first["solo"].Status != "restorable" {
		t.Fatalf("state = %+v", st)
	}

	e.gen.Bump()
	if got := e.ours(e.state(t)); len(got) != len(first) || got["solo"].Status != "restorable" {
		t.Fatalf("items after a second restart = %+v, want %+v", got, first)
	}

	_, err := svc.RestoreContainers(docker.RestoreArgs{EngineID: "other-engine", IDs: []string{solo}})
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeConflict) {
		t.Fatalf("wrong engine err = %v, want E_CONFLICT", err)
	}
	if err := svc.RestoreDismiss(docker.RestoreDismissArgs{EngineID: st.EngineID}); err != nil {
		t.Fatal(err)
	}
	if got := e.ours(e.state(t)); len(got) != 0 {
		t.Fatalf("items after dismiss = %+v, want none", got)
	}
	if !e.inSnapshot(keeper) || e.inSnapshot(solo) {
		t.Fatal("dismiss must rebuild the snapshot from the running set")
	}
	err = svc.RestoreDismiss(docker.RestoreDismissArgs{EngineID: st.EngineID})
	if !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeConflict) {
		t.Fatalf("dismiss without pending err = %v, want E_CONFLICT", err)
	}
}

// TestRestoreRealDaemonRestart restarts the sandbox dockerd, so it kills the engine every other
// test shares: opt-in, never in CI.
func TestRestoreRealDaemonRestart(t *testing.T) {
	if os.Getenv("KIRA_FLOW_DOCKER_RESTART") != "1" {
		t.Skip("set KIRA_FLOW_DOCKER_RESTART=1 to restart the local dockerd")
	}
	d := flowharness.RequireDocker(t)
	app := flowharness.New(t, flowharness.WithDockerOptions(docker.WithRestoreTiming(500*time.Millisecond, 2*time.Second, 5*time.Second)))
	e := restoreEnv{app: app, d: d}
	solo := sleeper(d, "solo", nil)
	keeper := keepAlive(d, "keeper", "always")
	e.waitSnapshot(t, solo, keeper)

	if out, err := exec.Command("pkill", "-TERM", "-x", "dockerd").CombinedOutput(); err != nil {
		t.Fatalf("pkill dockerd: %v %s", err, out)
	}
	waitFor(t, 90*time.Second, "dockerd to exit", func() bool {
		return exec.Command("pgrep", "-x", "dockerd").Run() != nil
	})
	cmd := exec.Command("sh", "-c", "setsid nohup dockerd > /tmp/dockerd.log 2>&1 < /dev/null &")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("start dockerd: %v %s", err, out)
	}
	waitFor(t, 90*time.Second, "dockerd to answer", func() bool {
		_, err := d.Cli.Ping(context.Background(), client.PingOptions{})
		return err == nil
	})

	var st docker.RestoreState
	waitFor(t, 60*time.Second, "pending after the daemon restart", func() bool {
		st = e.state(t)
		return st.State == "pending" && e.ours(st)["solo"].Status == "restorable"
	})
	args := docker.RestoreArgs{EngineID: st.EngineID, IDs: []string{solo}}
	res, err := app.W.Docker.RestoreContainers(args)
	if err != nil || len(res.Items) != 1 || !res.Items[0].OK {
		t.Fatalf("restore = %+v, %v", res, err)
	}
	waitFor(t, 30*time.Second, "restore to settle", func() bool { return !hasOpenOurs(e, e.state(t)) })
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
