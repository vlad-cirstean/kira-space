package dockerflow

import (
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func TestContainerActions(t *testing.T) {
	app, d := boot(t)
	svc := app.W.Docker

	plain := sleeper(d, "plain", nil)
	proj := "kiraflow-" + t.Name()
	web := sleeper(d, "web", map[string]string{"com.docker.compose.project": proj, "com.docker.compose.service": "web"})
	db := sleeper(d, "db", map[string]string{"com.docker.compose.project": proj, "com.docker.compose.service": "db"})

	all, err := svc.Containers(docker.ListArgs{All: true})
	if err != nil {
		t.Fatal(err)
	}
	for id, service := range map[string]string{web: "web", db: "db"} {
		c := findContainer(all, id)
		if c == nil || c.ComposeProject != proj || c.ComposeService != service {
			t.Fatalf("container %s = %+v, want project %q service %q", id, c, proj, service)
		}
	}
	if c := findContainer(all, plain); c == nil || c.ComposeProject != "" || c.State != "running" {
		t.Fatalf("plain container = %+v, want running without project", c)
	}

	if err := svc.Stop(docker.IDArgs{ID: plain}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "stopped container to drop out of an all:false list", func() bool {
		running, _ := svc.Containers(docker.ListArgs{All: false})
		return findContainer(running, plain) == nil
	})
	stopped, _ := svc.Containers(docker.ListArgs{All: true})
	if c := findContainer(stopped, plain); c == nil || c.State != "exited" {
		t.Fatalf("stopped container = %+v, want exited", c)
	}

	if err := svc.Start(docker.IDArgs{ID: plain}); err != nil {
		t.Fatal(err)
	}
	before, err := svc.InspectContainer(docker.IDArgs{ID: plain})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(docker.IDArgs{ID: plain}); err != nil {
		t.Fatal(err)
	}
	after, err := svc.InspectContainer(docker.IDArgs{ID: plain})
	if err != nil {
		t.Fatal(err)
	}
	if before.StartedAt == "" || after.StartedAt == before.StartedAt {
		t.Fatalf("StartedAt %q -> %q, want a change after Restart", before.StartedAt, after.StartedAt)
	}

	vol := d.Volume("insp", nil)
	insp := d.RunWith("insp", container.Config{Cmd: []string{"sleep", "300"}, Env: []string{"FLOW_VAR=kira"}},
		&container.HostConfig{Binds: []string{vol + ":/data"}}, nil)
	det, err := svc.InspectContainer(docker.IDArgs{ID: insp})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(det.Env, "FLOW_VAR=kira") {
		t.Fatalf("env = %v, want FLOW_VAR=kira", det.Env)
	}
	if len(det.Mounts) != 1 || det.Mounts[0].Name != vol || det.Mounts[0].Destination != "/data" || det.Mounts[0].Type != "volume" {
		t.Fatalf("mounts = %+v, want volume %s at /data", det.Mounts, vol)
	}
	if len(det.NetworkAttachments) == 0 || det.NetworkAttachments[0].Network != "bridge" {
		t.Fatalf("networks = %+v, want bridge", det.NetworkAttachments)
	}
	if det.Labels[flowharness.LabelKey] == "" || det.Raw == "" {
		t.Fatalf("labels/raw missing: %v", det.Labels)
	}

	for name, call := range map[string]func() error{
		"Start":   func() error { return svc.Start(docker.IDArgs{ID: "no-such-container"}) },
		"Inspect": func() error { _, err := svc.InspectContainer(docker.IDArgs{ID: "no-such-container"}); return err },
	} {
		if err := call(); err == nil || testx.AsIpcErr(t, err).Code != "E_NOT_FOUND" {
			t.Fatalf("%s unknown id = %v, want E_NOT_FOUND", name, err)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestImagesVolumesNetworks(t *testing.T) {
	app, d := boot(t)
	svc := app.W.Docker

	tag := d.Tag("img")
	vol := d.Volume("vol", nil)
	netID, netName := d.Network("net")
	d.RunWith("netbox", container.Config{Cmd: []string{"sleep", "300"}},
		&container.HostConfig{NetworkMode: container.NetworkMode(netName), Binds: []string{vol + ":/v"}}, nil)

	images, err := svc.Images()
	if err != nil {
		t.Fatal(err)
	}
	var img *docker.Image
	for i := range images {
		if contains(images[i].Tags, tag) {
			img = &images[i]
		}
	}
	if img == nil || img.Size <= 0 || img.Dangling {
		t.Fatalf("image %s = %+v, want listed with a size", tag, img)
	}

	vols, err := svc.Volumes()
	if err != nil {
		t.Fatal(err)
	}
	var v *docker.Volume
	for i := range vols {
		if vols[i].Name == vol {
			v = &vols[i]
		}
	}
	if v == nil || v.Driver != "local" || v.Labels[flowharness.LabelKey] == "" || len(v.UsedBy) != 1 {
		t.Fatalf("volume %s = %+v, want local, labelled, used by the container", vol, v)
	}

	nets, err := svc.Networks()
	if err != nil {
		t.Fatal(err)
	}
	var n *docker.Network
	for i := range nets {
		if nets[i].ID == netID {
			n = &nets[i]
		}
	}
	if n == nil || n.Name != netName || n.Builtin || n.Containers != 1 || len(n.UsedBy) != 1 || len(n.Subnets) == 0 {
		t.Fatalf("network %s = %+v, want user network with the one container", netName, n)
	}

	for kind, c := range map[string]struct{ id, want string }{
		"image":   {tag, tag},
		"volume":  {vol, vol},
		"network": {netID, netName},
	} {
		res, err := svc.Inspect(docker.InspectArgs{Kind: kind, ID: c.id})
		if err != nil {
			t.Fatalf("inspect %s: %v", kind, err)
		}
		if !strings.Contains(res.Raw, c.want) {
			t.Fatalf("inspect %s raw lacks %q", kind, c.want)
		}
	}
	if _, err := svc.Inspect(docker.InspectArgs{Kind: "secret", ID: "x"}); err == nil || testx.AsIpcErr(t, err).Code != "E_CONTRACT" {
		t.Fatalf("unknown kind = %v, want E_CONTRACT", err)
	}
}
