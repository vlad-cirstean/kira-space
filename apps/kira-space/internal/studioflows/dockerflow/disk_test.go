package dockerflow

import (
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const writerScript = "mkdir -p /data; dd if=/dev/zero of=/data/f bs=1024 count=1024 2>/dev/null; dd if=/dev/zero of=/rw bs=1024 count=512 2>/dev/null; echo ready; exec sleep 300"

// writer runs a container that writes 1 MiB into vol (when set) and 512 KiB into its own layer.
func writer(d *flowharness.Docker, name, vol string) string {
	stop := 1
	var host *container.HostConfig
	if vol != "" {
		host = &container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: vol, Target: "/data"}}}
	}
	return d.RunWith(name, container.Config{Cmd: []string{"sh", "-c", writerScript}, StopTimeout: &stop}, host, nil)
}

func TestDiskUsage(t *testing.T) {
	flowharness.Complete(t)
	app, d := boot(t)
	vol := d.Volume("disk", nil)
	writer(d, "disk", vol)

	var got docker.DiskUsage
	testx.WaitUntil(t, 30*time.Second, func() bool {
		var err error
		if got, err = app.W.Docker.DiskUsage(); err != nil {
			t.Fatal(err)
		}
		for _, v := range got.VolumeSizes {
			if v.Name == vol && v.Size >= 1<<20 {
				return got.Containers.Size >= 512<<10
			}
		}
		return false
	})
	if _, err := kiratime.ParseISO(got.TakenAt); err != nil {
		t.Fatalf("takenAt %q: %v", got.TakenAt, err)
	}
	if got.DurationMs < 0 || got.Containers.Count < 1 || got.Images.Count < 1 || got.Containers.Size < 512<<10 {
		t.Fatalf("usage = %+v, want containers >= 1 with >= 512 KiB and images >= 1", got)
	}
	if sum := got.Images.Size + got.Containers.Size + got.Volumes.Size + got.BuildCache.Size; got.Total != sum {
		t.Fatalf("total = %d, want %d", got.Total, sum)
	}
	if !sort.SliceIsSorted(got.VolumeSizes, func(i, j int) bool { return got.VolumeSizes[i].Size > got.VolumeSizes[j].Size }) {
		t.Fatalf("volumeSizes not sorted by size desc: %+v", got.VolumeSizes)
	}
	var mine []docker.VolumeDisk
	for _, v := range got.VolumeSizes {
		if v.Name == vol {
			if v.RefCount < 1 {
				t.Fatalf("volume %+v, want refCount >= 1", v)
			}
			mine = append(mine, v)
		}
	}
	got.VolumeSizes = mine
	app.Contract(t, "docker-disk", "DockerService.DiskUsage", got,
		flowharness.Mask("count", "active", "size", "reclaimable", "total", "refCount", "durationMs"),
		flowharness.Replace(vol, "kira-flow-disk"))
}

func TestContainerSize(t *testing.T) {
	app, d := boot(t)
	id := writer(d, "size", "")

	var got docker.ContainerSize
	testx.WaitUntil(t, wait, func() bool {
		var err error
		if got, err = app.W.Docker.ContainerSize(docker.IDArgs{ID: id}); err != nil {
			t.Fatal(err)
		}
		return got.SizeRw >= 512<<10
	})
	if got.SizeRootFs < got.SizeRw {
		t.Fatalf("size = %+v, want sizeRootFs >= sizeRw", got)
	}
	if _, err := kiratime.ParseISO(got.TakenAt); err != nil {
		t.Fatalf("takenAt %q: %v", got.TakenAt, err)
	}
	app.Contract(t, "docker-disk", "DockerService.ContainerSize", got, flowharness.Mask("sizeRw", "sizeRootFs", "durationMs"))

	for id, code := range map[string]string{"": "E_CONTRACT", "kira-flow-no-such-container": "E_NOT_FOUND"} {
		_, err := app.W.Docker.ContainerSize(docker.IDArgs{ID: id})
		var ie *ipcerr.Error
		if !errors.As(err, &ie) || ie.Code != code {
			t.Fatalf("ContainerSize(%q) err = %v, want %s", id, err, code)
		}
	}
}
