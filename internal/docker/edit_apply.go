package docker

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

const recreateTimeout = 3 * time.Minute

// lockEdit serialises edits of one container; the engine has no lock of its own.
func (m *Manager) lockEdit(id string) (func(), error) {
	if _, held := m.editing.LoadOrStore(id, struct{}{}); held {
		return nil, ipcerr.T(ipcerr.CodeConflict, "backend.core.docker.edit.editInProgress.body", nil)
	}
	return func() { m.editing.Delete(id) }, nil
}

// stepErr maps err and tags it with the failed step and what already landed.
func (m *Manager) stepErr(ep Endpoint, step string, err error, extra map[string]any) error {
	mapped := m.mapErr(ep, err)
	var ie *ipcerr.Error
	if !errors.As(mapped, &ie) {
		return mapped
	}
	out := ipcerr.New(ie.Code, step+": "+ie.Message)
	d := map[string]any{"step": step}
	maps.Copy(d, extra)
	out.Details = mustJSON(d)
	return out
}

func (m *Manager) loadForEdit(ctx context.Context, cli *client.Client, id, baseHash string) (container.InspectResponse, EditSpec, error) {
	res, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, EditSpec{}, err
	}
	cur := specFromInspect(res.Container)
	if cur.BaseHash != baseHash {
		return container.InspectResponse{}, EditSpec{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.docker.edit.changedSinceLoad.body", nil)
	}
	if err := cur.readOnlyErr(); err != nil {
		return container.InspectResponse{}, EditSpec{}, err
	}
	return res.Container, cur, nil
}

// inPlaceRun is one UpdateContainer call: steps stop at the first failure and nothing is rolled
// back, as each step is valid alone.
type inPlaceRun struct {
	m    *Manager
	cli  *client.Client
	ep   Endpoint
	ctx  context.Context
	r    container.InspectResponse
	cur  EditSpec
	want InPlaceSpec
	res  UpdateResult
	// restored is set when a failed alias change tried to reattach the old endpoint.
	restored *bool
}

func (x *inPlaceRun) fail(step string, err error) error {
	d := map[string]any{"applied": x.res.Applied}
	if x.restored != nil {
		d["restored"] = *x.restored
	}
	return x.m.stepErr(x.ep, step, err, d)
}

func (m *Manager) updateContainer(args UpdateArgs) (UpdateResult, error) {
	release, err := m.lockEdit(args.ID)
	if err != nil {
		return UpdateResult{}, err
	}
	defer release()
	cli, ep, err := m.client()
	if err != nil {
		return UpdateResult{}, m.mapErr(ep, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	r, cur, err := m.loadForEdit(ctx, cli, args.ID, args.BaseHash)
	if err != nil {
		return UpdateResult{}, m.mapErr(ep, err)
	}
	if err := validateInPlace(cur, args.Spec); err != nil {
		return UpdateResult{}, err
	}
	x := &inPlaceRun{m: m, cli: cli, ep: ep, ctx: ctx, r: r, cur: cur, want: args.Spec, res: UpdateResult{Applied: []string{}, Warnings: []string{}}}
	for _, step := range []func() error{x.rename, x.limits, x.networks} {
		if err := step(); err != nil {
			return UpdateResult{}, err
		}
	}
	return x.res, nil
}

func (x *inPlaceRun) rename() error {
	if x.want.Name == x.cur.InPlace.Name {
		return nil
	}
	if _, err := x.cli.ContainerRename(x.ctx, x.r.ID, client.ContainerRenameOptions{NewName: x.want.Name}); err != nil {
		return x.fail("rename", err)
	}
	x.res.Applied = append(x.res.Applied, "rename")
	return nil
}

// limits sends resources and restart policy in one ContainerUpdate.
func (x *inPlaceRun) limits() error {
	rs, resChanged := resourceDelta(x.cur.InPlace.Resources, x.want.Resources)
	restartChanged := x.want.Restart != x.cur.InPlace.Restart
	if !resChanged && !restartChanged {
		return nil
	}
	opts := client.ContainerUpdateOptions{}
	step := "restart"
	if resChanged {
		opts.Resources, step = &rs, "resources"
	}
	if restartChanged {
		opts.RestartPolicy = &container.RestartPolicy{Name: container.RestartPolicyMode(x.want.Restart.Name), MaximumRetryCount: x.want.Restart.MaxRetries}
	}
	u, err := x.cli.ContainerUpdate(x.ctx, x.r.ID, opts)
	if err != nil {
		return x.fail(step, err)
	}
	x.res.Warnings = append(x.res.Warnings, u.Warnings...)
	if resChanged {
		x.res.Applied = append(x.res.Applied, "resources")
	}
	if restartChanged {
		x.res.Applied = append(x.res.Applied, "restart")
	}
	return nil
}

func (x *inPlaceRun) networks() error {
	curNets, wantNets := netIndex(x.cur.InPlace.Networks), netIndex(x.want.Networks)
	for _, n := range slices.Sorted(maps.Keys(unionKeys(curNets, wantNets))) {
		c, hadIt := curNets[n]
		w, wantIt := wantNets[n]
		changed, err := x.network(n, hadIt, wantIt, !slices.Equal(c.Aliases, w.Aliases), w.Aliases)
		if err != nil {
			return x.fail("network:"+n, err)
		}
		if changed {
			x.res.Applied = append(x.res.Applied, "network:"+n)
		}
	}
	return nil
}

// network detaches, attaches or (for an alias change, as the engine has no endpoint update)
// re-attaches one network, keeping its static addresses.
func (x *inPlaceRun) network(n string, had, want, aliasesDiffer bool, aliases []string) (bool, error) {
	if had && want && !aliasesDiffer {
		return false, nil
	}
	if had {
		if _, err := x.cli.NetworkDisconnect(x.ctx, n, client.NetworkDisconnectOptions{Container: x.r.ID}); err != nil {
			return false, err
		}
	}
	if !want {
		return true, nil
	}
	e := &network.EndpointSettings{Aliases: aliases}
	o := x.r.NetworkSettings.Networks[n]
	if o != nil && had {
		e.IPAMConfig, e.DriverOpts = o.IPAMConfig.Copy(), o.DriverOpts
	}
	_, err := x.cli.NetworkConnect(x.ctx, n, client.NetworkConnectOptions{Container: x.r.ID, EndpointConfig: e})
	if err != nil && had {
		x.reattach(n, o)
	}
	return true, err
}

// reattach puts the container back on n with its original endpoint after a failed alias change.
// It runs on a fresh context so a timed-out edit still restores.
func (x *inPlaceRun) reattach(n string, orig *network.EndpointSettings) {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	e := &network.EndpointSettings{Aliases: netIndex(x.cur.InPlace.Networks)[n].Aliases}
	if orig != nil {
		e.IPAMConfig, e.DriverOpts = orig.IPAMConfig.Copy(), orig.DriverOpts
	}
	_, err := x.cli.NetworkConnect(ctx, n, client.NetworkConnectOptions{Container: x.r.ID, EndpointConfig: e})
	ok := err == nil
	x.restored = &ok
}

func netIndex(nets []EditNetwork) map[string]EditNetwork {
	out := make(map[string]EditNetwork, len(nets))
	for _, n := range nets {
		out[n.Name] = n
	}
	return out
}

func unionKeys[V any](a, b map[string]V) map[string]struct{} {
	out := map[string]struct{}{}
	for k := range a {
		out[k] = struct{}{}
	}
	for k := range b {
		out[k] = struct{}{}
	}
	return out
}

// resourceDelta builds the update body: only changed fields, as the engine reads zero as "unchanged".
func resourceDelta(cur, want Resources) (container.Resources, bool) {
	var rs container.Resources
	changed := false
	set := func(differs bool, apply func()) {
		if differs {
			apply()
			changed = true
		}
	}
	set(want.CPUShares != cur.CPUShares, func() { rs.CPUShares = want.CPUShares })
	set(want.NanoCPUs != cur.NanoCPUs, func() { rs.NanoCPUs = want.NanoCPUs })
	set(want.CPUQuota != cur.CPUQuota, func() { rs.CPUQuota = want.CPUQuota })
	set(want.CPUPeriod != cur.CPUPeriod, func() { rs.CPUPeriod = want.CPUPeriod })
	set(want.CpusetCpus != cur.CpusetCpus, func() { rs.CpusetCpus = want.CpusetCpus })
	set(want.CpusetMems != cur.CpusetMems, func() { rs.CpusetMems = want.CpusetMems })
	set(want.MemoryReservation != cur.MemoryReservation, func() { rs.MemoryReservation = want.MemoryReservation })
	set(want.BlkioWeight != cur.BlkioWeight, func() { rs.BlkioWeight = uint16(want.BlkioWeight) })
	set(want.Memory != cur.Memory || want.MemorySwap != cur.MemorySwap, func() {
		rs.Memory, rs.MemorySwap = want.Memory, want.MemorySwap
		if want.Memory == cur.Memory {
			rs.Memory = 0
		}
		if want.MemorySwap == 0 {
			rs.MemorySwap = 0
		}
	})
	set(want.PidsLimit != cur.PidsLimit, func() {
		v := want.PidsLimit
		if v == 0 {
			v = -1
		}
		rs.PidsLimit = &v
	})
	return rs, changed
}

// imageDefaults is the slice of an image config a recreate subtracts when the image changes.
type imageDefaults struct {
	Env, Cmd, Entrypoint []string
	Labels               map[string]string
	Volumes, Exposed     map[string]struct{}
	WorkingDir, User     string
	Healthcheck          *container.HealthConfig
}

type recreateBuild struct {
	Config  *container.Config
	Host    *container.HostConfig
	Primary string
	Net     *network.NetworkingConfig
	Extra   map[string]*network.EndpointSettings
}

func portOf(p PortBinding) (network.Port, bool) {
	return network.PortFrom(uint16(p.ContainerPort), network.IPProtocol(p.Proto))
}

func normalizeLinks(links []string) []string {
	if links == nil {
		return nil
	}
	out := make([]string, 0, len(links))
	for _, l := range links {
		target, alias, ok := strings.Cut(l, ":")
		if !ok {
			out = append(out, l)
			continue
		}
		alias = alias[strings.LastIndex(alias, "/")+1:]
		out = append(out, strings.TrimPrefix(target, "/")+":"+alias)
	}
	return out
}

func sameRow(a, b EditMount) bool {
	return a.Type == b.Type && a.Source == b.Source && a.Target == b.Target && a.ReadOnly == b.ReadOnly
}

// buildRecreate derives the create payload from the inspected container plus the edited specs.
// Everything not editable is carried over from r.
func buildRecreate(r container.InspectResponse, old *imageDefaults, in InPlaceSpec, rc RecreateSpec) recreateBuild {
	cfg := *r.Config
	hc := *r.HostConfig
	userHostname := cfg.Hostname != shortID(r.ID)

	cfg.Image, cfg.User = rc.Image, rc.User
	cfg.Cmd, cfg.Entrypoint, cfg.Env = slices.Clone(rc.Cmd), slices.Clone(rc.Entrypoint), slices.Clone(rc.Env)
	cfg.Labels = maps.Clone(rc.Labels)
	cfg.Hostname = rc.Hostname
	if rc.Hostname == "" || !userHostname {
		cfg.Domainname = ""
	}
	cfg.Volumes = maps.Clone(r.Config.Volumes)
	cfg.ExposedPorts = maps.Clone(r.Config.ExposedPorts)
	if old != nil && rc.Image != r.Config.Image {
		subtractDefaults(&cfg, old)
	}
	if cfg.ExposedPorts == nil {
		cfg.ExposedPorts = network.PortSet{}
	}

	hc.PortBindings = network.PortMap{}
	for _, p := range rc.Ports {
		port, ok := portOf(p)
		if !ok {
			continue
		}
		b := network.PortBinding{HostPort: p.HostPort}
		if p.HostIP != "" {
			b.HostIP, _ = netip.ParseAddr(p.HostIP)
		}
		hc.PortBindings[port] = append(hc.PortBindings[port], b)
		cfg.ExposedPorts[port] = struct{}{}
	}
	hc.CapAdd, hc.CapDrop = slices.Clone(rc.CapAdd), slices.Clone(rc.CapDrop)

	res := &hc.Resources
	res.CPUShares, res.NanoCPUs, res.CPUQuota, res.CPUPeriod = in.Resources.CPUShares, in.Resources.NanoCPUs, in.Resources.CPUQuota, in.Resources.CPUPeriod
	res.CpusetCpus, res.CpusetMems = in.Resources.CpusetCpus, in.Resources.CpusetMems
	res.Memory, res.MemorySwap, res.MemoryReservation = in.Resources.Memory, in.Resources.MemorySwap, in.Resources.MemoryReservation
	res.BlkioWeight = uint16(in.Resources.BlkioWeight)
	res.PidsLimit = nil
	if in.Resources.PidsLimit > 0 {
		v := in.Resources.PidsLimit
		res.PidsLimit = &v
	}
	hc.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(in.Restart.Name), MaximumRetryCount: in.Restart.MaxRetries}
	hc.ContainerIDFile = ""
	hc.Links = normalizeLinks(r.HostConfig.Links)
	hc.Binds, hc.Mounts = mountsFor(r.HostConfig, rc.Mounts)

	b := recreateBuild{Config: &cfg, Host: &hc, Extra: map[string]*network.EndpointSettings{}}
	if isNetworkless(networkMode(r.HostConfig)) {
		return b
	}
	endpoint := func(n EditNetwork) *network.EndpointSettings {
		e := &network.EndpointSettings{Aliases: slices.Clone(n.Aliases)}
		if o := r.NetworkSettings.Networks[n.Name]; o != nil {
			e.IPAMConfig, e.Links, e.DriverOpts = o.IPAMConfig.Copy(), slices.Clone(o.Links), maps.Clone(o.DriverOpts)
		}
		return e
	}
	nets := netIndex(in.Networks)
	b.Primary = networkMode(r.HostConfig)
	if _, ok := nets[b.Primary]; !ok {
		b.Primary = "none"
		if len(in.Networks) > 0 {
			b.Primary = in.Networks[0].Name
		}
	}
	if b.Primary != networkMode(r.HostConfig) {
		hc.NetworkMode = container.NetworkMode(b.Primary)
	}
	if n, ok := nets[b.Primary]; ok {
		if e := endpoint(n); b.Primary != "bridge" || e.IPAMConfig != nil || len(e.Aliases) > 0 || len(e.Links) > 0 {
			b.Net = &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{b.Primary: e}}
		}
	}
	for _, n := range in.Networks {
		if n.Name != b.Primary {
			b.Extra[n.Name] = endpoint(n)
		}
	}
	return b
}

// mountsFor keeps unchanged rows verbatim, rebuilds changed or new ones and drops deleted ones.
// Mounts the editor does not show (tmpfs, anonymous volumes, other types) always stay.
func mountsFor(hc *container.HostConfig, rows []EditMount) ([]string, []mount.Mount) {
	orig := map[string]EditMount{}
	for _, row := range editRows(hc) {
		orig[row.Key] = row
	}
	binds := []string{}
	mounts := []mount.Mount{}
	for _, mt := range hc.Mounts {
		if !((mt.Type == mount.TypeBind || mt.Type == mount.TypeVolume) && mt.Source != "") {
			mounts = append(mounts, mt)
		}
	}
	for _, row := range rows {
		kind, idx, _ := strings.Cut(row.Key, ":")
		n, _ := strconv.Atoi(idx)
		if o, ok := orig[row.Key]; ok && sameRow(o, row) {
			if kind == "bind" {
				binds = append(binds, hc.Binds[n])
			} else {
				mounts = append(mounts, hc.Mounts[n])
			}
			continue
		}
		mounts = append(mounts, mount.Mount{Type: mount.Type(row.Type), Source: row.Source, Target: row.Target, ReadOnly: row.ReadOnly})
	}
	return binds, mounts
}

// subtractDefaults drops values that only came from the old image so the new image's defaults apply.
func subtractDefaults(cfg *container.Config, old *imageDefaults) {
	cfg.Env = slices.DeleteFunc(cfg.Env, func(e string) bool { return slices.Contains(old.Env, e) })
	for k, v := range cfg.Labels {
		if ov, ok := old.Labels[k]; ok && ov == v {
			delete(cfg.Labels, k)
		}
	}
	if slices.Equal(cfg.Cmd, old.Cmd) {
		cfg.Cmd = nil
	}
	if slices.Equal(cfg.Entrypoint, old.Entrypoint) {
		cfg.Entrypoint = nil
	}
	for v := range cfg.Volumes {
		if _, ok := old.Volumes[v]; ok {
			delete(cfg.Volumes, v)
		}
	}
	for p := range cfg.ExposedPorts {
		if _, ok := old.Exposed[p.String()]; ok {
			delete(cfg.ExposedPorts, p)
		}
	}
	if cfg.WorkingDir == old.WorkingDir {
		cfg.WorkingDir = ""
	}
	if cfg.User == old.User {
		cfg.User = ""
	}
	if reflect.DeepEqual(cfg.Healthcheck, old.Healthcheck) {
		cfg.Healthcheck = nil
	}
}

type leftover struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// recreateRun is one RecreateContainer call past the create step.
type recreateRun struct {
	m                *Manager
	cli              *client.Client
	ep               Endpoint
	oldID, oldName   string
	oldTmp           string
	newID, newTmp    string
	wasRunning       bool
	renamedOldToTemp bool
}

// rollback undoes steps 3 to 7: drop the new container, restore the old name, restart the old one.
// It runs on a fresh context so a timed-out recreate still restores.
func (x *recreateRun) rollback(step string, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	var left []leftover
	oldNow := x.oldName
	if x.renamedOldToTemp {
		oldNow = x.oldTmp
	}
	if _, err := x.cli.ContainerRemove(ctx, x.newID, client.ContainerRemoveOptions{Force: true}); err != nil && !cerrdefs.IsNotFound(err) {
		left = append(left, leftover{x.newID, x.newTmp})
	}
	if x.renamedOldToTemp {
		if _, err := x.cli.ContainerRename(ctx, x.oldID, client.ContainerRenameOptions{NewName: x.oldName}); err != nil {
			left = append(left, leftover{x.oldID, oldNow})
		} else {
			oldNow = x.oldName
		}
	}
	if x.wasRunning {
		if _, err := x.cli.ContainerStart(ctx, x.oldID, client.ContainerStartOptions{}); err != nil {
			left = append(left, leftover{x.oldID, oldNow})
		}
	}
	d := map[string]any{"restored": len(left) == 0}
	if len(left) > 0 {
		d["containers"] = left
	}
	e := x.m.stepErr(x.ep, step, cause, d)
	var ie *ipcerr.Error
	if errors.As(e, &ie) {
		ie.Code = string(ipcerr.CodeRecreateFailed)
	}
	return e
}

// swap runs steps 3 to 7: extra networks, stop old, name swap, start new.
func (x *recreateRun) swap(ctx context.Context, b recreateBuild, finalName string) (string, error) {
	for _, n := range sortedKeys(b.Extra) {
		if _, err := x.cli.NetworkConnect(ctx, n, client.NetworkConnectOptions{Container: x.newID, EndpointConfig: b.Extra[n]}); err != nil {
			return "network:" + n, err
		}
	}
	if x.wasRunning {
		if _, err := x.cli.ContainerStop(ctx, x.oldID, client.ContainerStopOptions{}); err != nil {
			return "stop", err
		}
	}
	if _, err := x.cli.ContainerRename(ctx, x.oldID, client.ContainerRenameOptions{NewName: x.oldTmp}); err != nil {
		return "rename-old", err
	}
	x.renamedOldToTemp = true
	if _, err := x.cli.ContainerRename(ctx, x.newID, client.ContainerRenameOptions{NewName: finalName}); err != nil {
		return "rename-new", err
	}
	if x.wasRunning {
		if _, err := x.cli.ContainerStart(ctx, x.newID, client.ContainerStartOptions{}); err != nil {
			return "start", err
		}
	}
	return "", nil
}

// prepareRecreate runs step 1: lock-protected checks and the create payload.
func (m *Manager) prepareRecreate(ctx context.Context, cli *client.Client, args RecreateArgs) (container.InspectResponse, EditSpec, recreateBuild, error) {
	r, cur, err := m.loadForEdit(ctx, cli, args.ID, args.BaseHash)
	if err != nil {
		return r, cur, recreateBuild{}, err
	}
	rc := args.Recreate
	if err := validateRecreate(cur, args.InPlace, rc); err != nil {
		return r, cur, recreateBuild{}, err
	}
	if _, err := cli.ImageInspect(ctx, rc.Image); err != nil {
		if cerrdefs.IsNotFound(err) {
			return r, cur, recreateBuild{}, ipcerr.T(ipcerr.CodeNotFound, "backend.core.docker.edit.imageMissing.body", map[string]string{"image": rc.Image})
		}
		return r, cur, recreateBuild{}, err
	}
	var old *imageDefaults
	if rc.Image != r.Config.Image {
		if oi, err := cli.ImageInspect(ctx, r.Image); err == nil && oi.Config != nil {
			c := oi.Config
			old = &imageDefaults{
				Env: c.Env, Cmd: c.Cmd, Entrypoint: c.Entrypoint, Labels: c.Labels, Volumes: c.Volumes, Exposed: c.ExposedPorts,
				WorkingDir: c.WorkingDir, User: c.User, Healthcheck: c.Healthcheck,
			}
		}
	}
	return r, cur, buildRecreate(r, old, args.InPlace, rc), nil
}

func (m *Manager) recreateContainer(args RecreateArgs) (RecreateResult, error) {
	release, err := m.lockEdit(args.ID)
	if err != nil {
		return RecreateResult{}, err
	}
	defer release()
	cli, ep, err := m.client()
	if err != nil {
		return RecreateResult{}, m.mapErr(ep, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), recreateTimeout)
	defer cancel()
	r, cur, b, err := m.prepareRecreate(ctx, cli, args)
	if err != nil {
		return RecreateResult{}, m.mapErr(ep, err)
	}

	x := &recreateRun{
		m: m, cli: cli, ep: ep, oldID: r.ID, oldName: cur.InPlace.Name, wasRunning: r.State != nil && r.State.Running,
		oldTmp: cur.InPlace.Name + "-kira-old-" + ids.New(ids.DockerTemp), newTmp: cur.InPlace.Name + "-kira-new-" + ids.New(ids.DockerTemp),
	}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: x.newTmp, Config: b.Config, HostConfig: b.Host, NetworkingConfig: b.Net})
	if err != nil {
		return RecreateResult{}, m.stepErr(ep, "create", err, nil)
	}
	x.newID = created.ID
	warnings := append([]string{}, created.Warnings...)
	if step, err := x.swap(ctx, b, args.InPlace.Name); err != nil {
		return RecreateResult{}, x.rollback(step, err)
	}
	rctx, rcancel := context.WithTimeout(context.Background(), actionTimeout)
	defer rcancel()
	if _, err := cli.ContainerRemove(rctx, x.oldID, client.ContainerRemoveOptions{}); err != nil {
		warnings = append(warnings, fmt.Sprintf("old container kept as %s: %v", x.oldTmp, err))
	}
	return RecreateResult{ID: x.newID, Name: args.InPlace.Name, OldID: x.oldID, Warnings: warnings}, nil
}
