package dockerflow

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const (
	mib        = 1 << 20
	editScript = "echo hello > /data/hello; echo ready; exec sleep 300"
)

// editBox is a running container with every kind of setting the editor shows.
type editBox struct {
	id, name  string
	vol, anon string
	net, net2 string
}

func runIDOf(d *flowharness.Docker) string {
	return strings.TrimSuffix(strings.TrimPrefix(d.Prefix(), "kira-flow-"), "-")
}

func editOpts(d *flowharness.Docker, box editBox, extra ...flowharness.ContractOption) []flowharness.ContractOption {
	opts := []flowharness.ContractOption{
		flowharness.Mask("id", "baseHash", "oldId"),
		flowharness.Replace(d.Prefix(), "kira-flow-"),
		flowharness.Replace(runIDOf(d), "run"),
	}
	if box.anon != "" {
		opts = append(opts, flowharness.Replace(box.anon, "anon-volume"))
	}
	return append(opts, extra...)
}

func newEditBox(t *testing.T, d *flowharness.Docker, running bool) editBox {
	t.Helper()
	box := editBox{vol: d.Volume("edit", nil)}
	_, box.net = d.Network("edit")
	_, box.net2 = d.Network("edit2")
	stop := 1
	cfg := container.Config{
		Cmd: []string{"sh", "-c", editScript}, StopTimeout: &stop, Env: []string{"A=1"}, Hostname: "edit-host",
		ExposedPorts: network.PortSet{network.MustParsePort("8080/tcp"): {}},
	}
	host := &container.HostConfig{
		Resources:     container.Resources{Memory: 64 * mib, MemorySwap: 128 * mib},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyOnFailure, MaximumRetryCount: 3},
		Mounts: []mount.Mount{
			{Type: mount.TypeVolume, Source: box.vol, Target: "/data"},
			{Type: mount.TypeVolume, Target: "/anon"},
		},
		PortBindings: network.PortMap{network.MustParsePort("8080/tcp"): {{}}},
		CapAdd:       []string{"NET_ADMIN"},
	}
	box.name = d.Prefix() + "edit"
	box.id = d.RunNamed(box.name, cfg, host, map[string]string{"edit": "1"})
	in, err := d.Cli.ContainerInspect(context.Background(), box.id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, mp := range in.Container.Mounts {
		if mp.Destination == "/anon" {
			box.anon = mp.Name
		}
	}
	if box.anon == "" {
		t.Fatalf("mounts = %+v, want an anonymous volume at /anon", in.Container.Mounts)
	}
	t.Cleanup(func() {
		_, _ = d.Cli.VolumeRemove(context.Background(), box.anon, client.VolumeRemoveOptions{Force: true})
	})
	if _, err := d.Cli.NetworkConnect(context.Background(), box.net, client.NetworkConnectOptions{
		Container: box.id, EndpointConfig: &network.EndpointSettings{Aliases: []string{"svc"}},
	}); err != nil {
		t.Fatal(err)
	}
	if !running {
		if err := stopForEdit(d, box.id); err != nil {
			t.Fatal(err)
		}
	}
	return box
}

func stopForEdit(d *flowharness.Docker, id string) error {
	_, err := d.Cli.ContainerStop(context.Background(), id, client.ContainerStopOptions{})
	return err
}

func netByName(s docker.InPlaceSpec, name string) *docker.EditNetwork {
	for i := range s.Networks {
		if s.Networks[i].Name == name {
			return &s.Networks[i]
		}
	}
	return nil
}

func ipcCode(t *testing.T, err error) *ipcerr.Error {
	t.Helper()
	var ie *ipcerr.Error
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v, want *ipcerr.Error", err)
	}
	return ie
}

func TestContainerEditSpec(t *testing.T) {
	app, d := boot(t)
	box := newEditBox(t, d, true)

	spec, err := app.W.Docker.ContainerEditSpec(docker.IDArgs{ID: box.id})
	if err != nil {
		t.Fatal(err)
	}
	if spec.ID != box.id || spec.State != "running" || len(spec.BaseHash) != 64 || spec.NetworkMode != "bridge" {
		t.Fatalf("spec head = %s %s %q %s, want running bridge with sha256 hash", spec.ID, spec.State, spec.BaseHash, spec.NetworkMode)
	}
	ip, rc := spec.InPlace, spec.Recreate
	if ip.Name != box.name || ip.Resources.Memory != 64*mib || ip.Restart.Name != "on-failure" || ip.Restart.MaxRetries != 3 {
		t.Fatalf("inPlace = %+v, want name, 64 MiB and on-failure:3", ip)
	}
	if ip.Resources.MemorySwap != 128*mib && ip.Resources.MemorySwap != -1 {
		t.Fatalf("swap = %d, want 128 MiB (or -1 without swap accounting)", ip.Resources.MemorySwap)
	}
	if n := netByName(ip, box.net); n == nil || !slices.Equal(n.Aliases, []string{"svc"}) {
		t.Fatalf("networks = %+v, want %s with only alias svc (short ID filtered)", ip.Networks, box.net)
	}
	if n := netByName(ip, "bridge"); n == nil || len(n.Aliases) != 0 {
		t.Fatalf("networks = %+v, want bridge without aliases", ip.Networks)
	}
	if rc.Hostname != "edit-host" || !slices.Contains(rc.Env, "A=1") || rc.Labels["edit"] != "1" || !slices.Contains(rc.CapAdd, "CAP_NET_ADMIN") {
		t.Fatalf("recreate = %+v, want hostname, env, label and cap", rc)
	}
	if len(rc.Ports) != 1 || rc.Ports[0].ContainerPort != 8080 || rc.Ports[0].Proto != "tcp" || rc.Ports[0].HostPort != "" {
		t.Fatalf("ports = %+v, want 8080/tcp with engine-assigned host port", rc.Ports)
	}
	if len(rc.Mounts) != 1 || rc.Mounts[0].Source != box.vol || rc.Mounts[0].Target != "/data" || rc.Mounts[0].Type != "volume" || rc.Mounts[0].Key == "" {
		t.Fatalf("mounts = %+v, want the named volume only", rc.Mounts)
	}
	if len(spec.AnonymousVolumes) != 1 || spec.AnonymousVolumes[0].Name != box.anon || spec.AnonymousVolumes[0].Destination != "/anon" {
		t.Fatalf("anonymous = %+v, want %s at /anon", spec.AnonymousVolumes, box.anon)
	}
	again, err := app.W.Docker.ContainerEditSpec(docker.IDArgs{ID: box.id})
	if err != nil || again.BaseHash != spec.BaseHash {
		t.Fatalf("second spec hash %q (err %v), want stable %q", again.BaseHash, err, spec.BaseHash)
	}
	app.Contract(t, "docker-edit", "DockerService.ContainerEditSpec", spec, editOpts(d, box)...)

	_, err = app.W.Docker.ContainerEditSpec(docker.IDArgs{ID: "kira-flow-no-such"})
	if ie := ipcCode(t, err); ie.Code != "E_NOT_FOUND" {
		t.Fatalf("code = %s, want E_NOT_FOUND", ie.Code)
	}
}

func TestUpdateContainerInPlace(t *testing.T) {
	app, d := boot(t)
	box := newEditBox(t, d, true)
	svc := app.W.Docker

	spec, err := svc.ContainerEditSpec(docker.IDArgs{ID: box.id})
	if err != nil {
		t.Fatal(err)
	}
	want := spec.InPlace
	want.Name = box.name + "-renamed"
	want.Resources.Memory, want.Resources.MemorySwap, want.Resources.MemoryReservation = 96*mib, 192*mib, 32*mib
	want.Resources.NanoCPUs, want.Resources.CPUShares, want.Resources.PidsLimit = 500_000_000, 512, 64
	want.Restart = docker.RestartPolicy{Name: "unless-stopped"}
	want.Networks = slices.Clone(want.Networks)
	for i := range want.Networks {
		if want.Networks[i].Name == box.net {
			want.Networks[i].Aliases = []string{"svc", "api"}
		}
	}
	want.Networks = append(want.Networks, docker.EditNetwork{Name: box.net2, Aliases: []string{}})
	args := docker.UpdateArgs{ID: box.id, BaseHash: spec.BaseHash, Spec: want}

	got, err := svc.UpdateContainer(args)
	if err != nil {
		t.Fatal(err)
	}
	wantSteps := []string{"rename", "resources", "restart"}
	for _, n := range []string{box.net, box.net2} {
		wantSteps = append(wantSteps, "network:"+n)
	}
	if !slices.Equal(got.Applied, wantSteps) {
		t.Fatalf("applied = %v, want %v", got.Applied, wantSteps)
	}

	in, err := d.Cli.ContainerInspect(context.Background(), box.id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c, hc := in.Container, in.Container.HostConfig
	if c.ID != box.id || !c.State.Running || strings.TrimPrefix(c.Name, "/") != want.Name {
		t.Fatalf("container = %s running=%v name=%s, want same ID, running, renamed", c.ID, c.State.Running, c.Name)
	}
	if hc.Memory != 96*mib || hc.MemoryReservation != 32*mib || hc.NanoCPUs != 500_000_000 || hc.CPUShares != 512 || hc.PidsLimit == nil || *hc.PidsLimit != 64 {
		t.Fatalf("host config = %+v, want updated limits", hc.Resources)
	}
	if hc.RestartPolicy.Name != container.RestartPolicyUnlessStopped {
		t.Fatalf("restart = %+v, want unless-stopped", hc.RestartPolicy)
	}
	if a := c.NetworkSettings.Networks[box.net]; a == nil || !slices.Contains(a.Aliases, "svc") || !slices.Contains(a.Aliases, "api") {
		t.Fatalf("%s endpoint = %+v, want aliases svc and api", box.net, a)
	}
	if c.NetworkSettings.Networks[box.net2] == nil {
		t.Fatalf("networks = %v, want %s attached", c.NetworkSettings.Networks, box.net2)
	}
	app.Contract(t, "docker-edit", "args:DockerService.UpdateContainer", args, editOpts(d, box)...)
	app.Contract(t, "docker-edit", "DockerService.UpdateContainer", got, editOpts(d, box, flowharness.Mask("warnings"))...)
}

func TestUpdateContainerRejects(t *testing.T) {
	app, d := boot(t)
	box := newEditBox(t, d, true)
	other := sleeper(d, "taken", nil)
	svc := app.W.Docker

	spec, err := svc.ContainerEditSpec(docker.IDArgs{ID: box.id})
	if err != nil {
		t.Fatal(err)
	}
	try := func(mutate func(*docker.InPlaceSpec), hash string) *ipcerr.Error {
		t.Helper()
		in := spec.InPlace
		in.Networks = slices.Clone(in.Networks)
		mutate(&in)
		_, err := svc.UpdateContainer(docker.UpdateArgs{ID: box.id, BaseHash: hash, Spec: in})
		return ipcCode(t, err)
	}

	stale := try(func(*docker.InPlaceSpec) {}, strings.Repeat("0", 64))
	if stale.Code != "E_CONFLICT" {
		t.Fatalf("stale hash code = %s, want E_CONFLICT", stale.Code)
	}
	app.Contract(t, "docker-edit", "DockerService.UpdateContainer#stale", stale, editOpts(d, box)...)
	ie := try(func(in *docker.InPlaceSpec) { in.Resources.Memory = 0 }, spec.BaseHash)
	if ie.Code != "E_BAD_REQUEST" || !strings.Contains(string(ie.Details), "resources.memory") {
		t.Fatalf("clear memory = %s %s, want E_BAD_REQUEST on resources.memory", ie.Code, ie.Details)
	}
	if ie := try(func(in *docker.InPlaceSpec) { in.Resources.Memory, in.Resources.MemorySwap = 128*mib, 64*mib }, spec.BaseHash); ie.Code != "E_BAD_REQUEST" {
		t.Fatalf("swap below memory code = %s, want E_BAD_REQUEST", ie.Code)
	}
	otherName := strings.TrimPrefix(inspectName(t, d, other), "/")
	if ie := try(func(in *docker.InPlaceSpec) { in.Name = otherName }, spec.BaseHash); ie.Code != "E_CONFLICT" {
		t.Fatalf("name taken code = %s, want E_CONFLICT", ie.Code)
	}

	managed := d.RunWith("managed", container.Config{Cmd: []string{"sleep", "300"}}, nil, map[string]string{"io.kubernetes.pod.name": "p"})
	mspec, err := svc.ContainerEditSpec(docker.IDArgs{ID: managed})
	if err != nil || mspec.Managed != "kubernetes" {
		t.Fatalf("managed spec = %q, err %v, want kubernetes", mspec.Managed, err)
	}
	app.Contract(t, "docker-edit", "DockerService.ContainerEditSpec#managed", mspec, editOpts(d, box)...)
	_, err = svc.UpdateContainer(docker.UpdateArgs{ID: managed, BaseHash: mspec.BaseHash, Spec: mspec.InPlace})
	if ie := ipcCode(t, err); ie.Code != "E_BAD_REQUEST" {
		t.Fatalf("managed code = %s, want E_BAD_REQUEST", ie.Code)
	}
}

func inspectName(t *testing.T, d *flowharness.Docker, id string) string {
	t.Helper()
	in, err := d.Cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return in.Container.Name
}

func TestUpdateStoppedContainer(t *testing.T) {
	app, d := boot(t)
	box := newEditBox(t, d, false)
	svc := app.W.Docker

	spec, err := svc.ContainerEditSpec(docker.IDArgs{ID: box.id})
	if err != nil {
		t.Fatal(err)
	}
	if spec.State == "running" {
		t.Fatalf("state = %s, want stopped", spec.State)
	}
	want := spec.InPlace
	want.Restart = docker.RestartPolicy{Name: "always"}
	want.Resources.Memory, want.Resources.MemorySwap = 80*mib, 160*mib
	got, err := svc.UpdateContainer(docker.UpdateArgs{ID: box.id, BaseHash: spec.BaseHash, Spec: want})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Applied, []string{"resources", "restart"}) {
		t.Fatalf("applied = %v, want resources, restart", got.Applied)
	}
	args := docker.UpdateArgs{ID: box.id, BaseHash: spec.BaseHash, Spec: want}
	app.Contract(t, "docker-edit", "DockerService.ContainerEditSpec#stopped", spec, editOpts(d, box)...)
	app.Contract(t, "docker-edit", "args:DockerService.UpdateContainer#stopped", args, editOpts(d, box)...)
	app.Contract(t, "docker-edit", "DockerService.UpdateContainer#stopped", got, editOpts(d, box, flowharness.Mask("warnings"))...)
	in, err := d.Cli.ContainerInspect(context.Background(), box.id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if in.Container.State.Running || in.Container.HostConfig.RestartPolicy.Name != container.RestartPolicyAlways || in.Container.HostConfig.Memory != 80*mib {
		t.Fatalf("container running=%v restart=%v memory=%d, want stopped, always, 80 MiB",
			in.Container.State.Running, in.Container.HostConfig.RestartPolicy, in.Container.HostConfig.Memory)
	}
}

// recreateArgs builds the edits TestRecreateContainer asserts: env, label, port, cmd and image.
func recreateArgs(spec docker.EditSpec, image string) docker.RecreateArgs {
	rc := spec.Recreate
	rc.Image = image
	rc.Env = append(slices.Clone(rc.Env), "B=2")
	rc.Labels = map[string]string{}
	for k, v := range spec.Recreate.Labels {
		rc.Labels[k] = v
	}
	rc.Labels["edit2"] = "yes"
	rc.Ports = []docker.PortBinding{{ContainerPort: 8081, Proto: "tcp", HostPort: ""}}
	rc.Cmd = []string{"sh", "-c", "echo again; exec sleep 300"}
	return docker.RecreateArgs{ID: spec.ID, BaseHash: spec.BaseHash, InPlace: spec.InPlace, Recreate: rc}
}

func registerRecreated(t *testing.T, d *flowharness.Docker, id string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = d.Cli.ContainerRemove(context.Background(), id, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	})
}

func noTempContainers(t *testing.T, d *flowharness.Docker) {
	t.Helper()
	list, err := d.Cli.ContainerList(context.Background(), client.ContainerListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list.Items {
		for _, n := range c.Names {
			if strings.Contains(n, "-kira-new-") || strings.Contains(n, "-kira-old-") {
				t.Fatalf("temp container %v left behind", c.Names)
			}
		}
	}
}

func TestRecreateContainer(t *testing.T) {
	flowharness.Complete(t)
	app, d := boot(t)
	box := newEditBox(t, d, true)
	svc := app.W.Docker
	mark := app.Events.Mark()

	if _, err := svc.ExecOpen(docker.ExecOpenArgs{WindowKey: "w1", TerminalID: "x1", ContainerID: box.id, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	spec, err := svc.ContainerEditSpec(docker.IDArgs{ID: box.id})
	if err != nil {
		t.Fatal(err)
	}
	tag := d.Tag("edit")
	args := recreateArgs(spec, tag)
	got, err := svc.RecreateContainer(args)
	if err != nil {
		t.Fatal(err)
	}
	registerRecreated(t, d, got.ID)
	if got.ID == box.id || got.OldID != box.id || got.Name != box.name {
		t.Fatalf("result = %+v, want a new ID, old %s, same name", got, box.id)
	}

	in, err := d.Cli.ContainerInspect(context.Background(), got.ID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c := in.Container
	if !c.State.Running || strings.TrimPrefix(c.Name, "/") != box.name || c.Config.Image != tag {
		t.Fatalf("container running=%v name=%s image=%s, want running, %s, %s", c.State.Running, c.Name, c.Config.Image, box.name, tag)
	}
	if !slices.Contains(c.Config.Env, "B=2") || !slices.Contains(c.Config.Env, "A=1") || c.Config.Labels["edit2"] != "yes" || c.Config.Labels["edit"] != "1" {
		t.Fatalf("env %v labels %v, want edits and originals", c.Config.Env, c.Config.Labels)
	}
	if _, ok := c.HostConfig.PortBindings[network.MustParsePort("8081/tcp")]; !ok || len(c.HostConfig.PortBindings) != 1 {
		t.Fatalf("port bindings = %v, want only 8081/tcp", c.HostConfig.PortBindings)
	}
	if a := c.NetworkSettings.Networks[box.net]; a == nil || !slices.Contains(a.Aliases, "svc") {
		t.Fatalf("%s endpoint = %+v, want alias svc kept", box.net, a)
	}
	if c.HostConfig.Memory != 64*mib || c.HostConfig.RestartPolicy.Name != container.RestartPolicyOnFailure || c.Config.Hostname != "edit-host" {
		t.Fatalf("memory %d restart %v hostname %s, want preserved", c.HostConfig.Memory, c.HostConfig.RestartPolicy, c.Config.Hostname)
	}

	if _, err := svc.InspectContainer(docker.IDArgs{ID: box.id}); ipcCode(t, err).Code != "E_NOT_FOUND" {
		t.Fatalf("old container err = %v, want E_NOT_FOUND", err)
	}
	if _, err := d.Cli.VolumeInspect(context.Background(), box.anon, client.VolumeInspectOptions{}); err != nil {
		t.Fatalf("old anonymous volume: %v, want it kept", err)
	}
	testx.WaitUntil(t, wait, func() bool {
		_, exited, _ := execOut(app, mark, "w1", "x1")
		return exited
	})

	if _, err := svc.ExecOpen(docker.ExecOpenArgs{WindowKey: "w1", TerminalID: "x2", ContainerID: got.ID, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	write(t, app, "x2", "cat /data/hello; echo done-$((1+1))\n")
	testx.WaitUntil(t, wait, func() bool {
		out, _, _ := execOut(app, mark, "w1", "x2")
		return strings.Contains(out, "hello") && strings.Contains(out, "done-2")
	})
	noTempContainers(t, d)

	app.Contract(t, "docker-edit", "args:DockerService.RecreateContainer", args, editOpts(d, box)...)
	app.Contract(t, "docker-edit", "DockerService.RecreateContainer", got, editOpts(d, box, flowharness.Mask("warnings"))...)
}

func TestRecreateStoppedContainer(t *testing.T) {
	flowharness.Complete(t)
	app, d := boot(t)
	box := newEditBox(t, d, false)
	svc := app.W.Docker

	spec, err := svc.ContainerEditSpec(docker.IDArgs{ID: box.id})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.RecreateContainer(recreateArgs(spec, spec.Recreate.Image))
	if err != nil {
		t.Fatal(err)
	}
	registerRecreated(t, d, got.ID)
	in, err := d.Cli.ContainerInspect(context.Background(), got.ID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == box.id || in.Container.State.Running || strings.TrimPrefix(in.Container.Name, "/") != box.name {
		t.Fatalf("container %s running=%v name=%s, want new ID, stopped, %s", got.ID, in.Container.State.Running, in.Container.Name, box.name)
	}
	noTempContainers(t, d)
}

func TestRecreateRollback(t *testing.T) {
	flowharness.Complete(t)
	app, d := boot(t)
	box := newEditBox(t, d, true)
	svc := app.W.Docker

	spec, err := svc.ContainerEditSpec(docker.IDArgs{ID: box.id})
	if err != nil {
		t.Fatal(err)
	}
	args := docker.RecreateArgs{ID: box.id, BaseHash: spec.BaseHash, InPlace: spec.InPlace, Recreate: spec.Recreate}
	args.Recreate.Entrypoint = []string{"/nonexistent"}
	_, err = svc.RecreateContainer(args)
	ie := ipcCode(t, err)
	if ie.Code != "E_RECREATE_FAILED" || !strings.Contains(string(ie.Details), `"restored":true`) || !strings.Contains(string(ie.Details), `"step":"start"`) {
		t.Fatalf("err = %s %s, want E_RECREATE_FAILED restored at step start", ie.Code, ie.Details)
	}
	in, err := d.Cli.ContainerInspect(context.Background(), box.id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !in.Container.State.Running || strings.TrimPrefix(in.Container.Name, "/") != box.name {
		t.Fatalf("original running=%v name=%s, want restored and running as %s", in.Container.State.Running, in.Container.Name, box.name)
	}
	noTempContainers(t, d)
	app.Contract(t, "docker-edit", "DockerService.RecreateContainer#rollback", ie, editOpts(d, box, flowharness.Mask("message"))...)
}

func TestRecreateRefusals(t *testing.T) {
	flowharness.Complete(t)
	app, d := boot(t)
	svc := app.W.Docker

	auto := d.RunWith("auto", container.Config{Cmd: []string{"sleep", "300"}}, &container.HostConfig{AutoRemove: true}, nil)
	aspec, err := svc.ContainerEditSpec(docker.IDArgs{ID: auto})
	if err != nil || !aspec.AutoRemove {
		t.Fatalf("auto spec autoRemove=%v err=%v, want true", aspec.AutoRemove, err)
	}
	_, err = svc.RecreateContainer(docker.RecreateArgs{ID: auto, BaseHash: aspec.BaseHash, InPlace: aspec.InPlace, Recreate: aspec.Recreate})
	ie := ipcCode(t, err)
	if ie.Code != "E_BAD_REQUEST" {
		t.Fatalf("auto-remove code = %s, want E_BAD_REQUEST", ie.Code)
	}
	autoBox := editBox{}
	app.Contract(t, "docker-edit", "DockerService.ContainerEditSpec#auto-remove", aspec, editOpts(d, autoBox)...)
	app.Contract(t, "docker-edit", "DockerService.RecreateContainer#auto-remove", ie, editOpts(d, autoBox, flowharness.Mask("message"))...)

	box := newEditBox(t, d, true)
	spec, err := svc.ContainerEditSpec(docker.IDArgs{ID: box.id})
	if err != nil {
		t.Fatal(err)
	}
	args := recreateArgs(spec, spec.Recreate.Image)
	var (
		wg      sync.WaitGroup
		results [2]docker.RecreateResult
		errs    [2]error
	)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = svc.RecreateContainer(args)
		}()
	}
	wg.Wait()
	ok, failed := 0, 0
	for i := range results {
		switch {
		case errs[i] == nil:
			ok++
			registerRecreated(t, d, results[i].ID)
		case ipcCode(t, errs[i]).Code == "E_CONFLICT":
			failed++
		default:
			t.Fatalf("unexpected error: %v", errs[i])
		}
	}
	if ok != 1 || failed != 1 {
		t.Fatalf("outcomes ok=%d conflict=%d, want 1 and 1", ok, failed)
	}
}
