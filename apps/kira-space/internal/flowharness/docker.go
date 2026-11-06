package studioflowharness

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/ids"
)

const (
	// EnvDocker is "require" to turn a missing engine into a failure instead of a skip.
	EnvDocker = "KIRA_FLOW_DOCKER"
	// LabelKey marks every resource a flow test creates, for cleanup and the crash sweep.
	LabelKey = "kira.flowtest"
	// Image is the one image flow tests run; always a mirror.gcr.io name (Docker Hub blobs are
	// blocked in the sandbox).
	Image = "mirror.gcr.io/library/alpine:3.20"

	sweepAge = time.Hour
)

var (
	runID = ids.New(ids.FlowRun)

	// dockerHost is the engine endpoint resolved once in Main, before any test swaps HOME.
	dockerHost string

	pullOnce sync.Once
	pullErr  error
)

// resolveDockerHost finds the engine endpoint the way a user's shell would: DOCKER_HOST, else the
// current docker context, else the default and the usual Colima/Desktop sockets. "" when none exists.
func resolveDockerHost() string {
	if h := os.Getenv("DOCKER_HOST"); h != "" {
		return h
	}
	if out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output(); err == nil {
		if h := strings.TrimSpace(string(out)); h != "" && h != "<no value>" {
			return h
		}
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		"/var/run/docker.sock",
		filepath.Join(home, ".colima", "default", "docker.sock"),
		filepath.Join(home, ".docker", "run", "docker.sock"),
		filepath.Join(home, ".rd", "docker.sock"),
	} {
		if _, err := os.Stat(p); err == nil {
			return "unix://" + p
		}
	}
	return ""
}

// useDockerHost points the test's DOCKER_HOST at the engine resolved in Main, so a temp HOME never
// hides it.
func useDockerHost(t testing.TB) {
	if dockerHost != "" {
		t.Setenv("DOCKER_HOST", dockerHost)
	}
}

// Docker is a client to the real engine with labelled, auto-removed resources.
type Docker struct {
	t    testing.TB
	Cli  *client.Client
	Host string
}

// RequireDocker returns a client to the engine, or skips (fails under KIRA_FLOW_DOCKER=require)
// when none answers. Pulls Image once per test binary.
func RequireDocker(t testing.TB) *Docker {
	t.Helper()
	useDockerHost(t)
	fail := func(format string, args ...any) {
		t.Helper()
		msg := fmt.Sprintf("docker: no engine at %q (%s); start dockerd or colima, or unset %s", dockerHost, fmt.Sprintf(format, args...), EnvDocker)
		if os.Getenv(EnvDocker) == "require" {
			t.Fatal(msg)
		}
		t.Skip(msg)
	}
	if dockerHost == "" {
		fail("no socket found")
	}
	cli, err := newClient(dockerHost)
	if err != nil {
		fail("%v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx, client.PingOptions{}); err != nil {
		fail("%v", err)
	}
	pullOnce.Do(func() { pullErr = pullImage(cli) })
	if pullErr != nil {
		t.Fatalf("docker: pull %s: %v", Image, pullErr)
	}
	return &Docker{t: t, Cli: cli, Host: dockerHost}
}

func newClient(host string) (*client.Client, error) {
	if _, err := url.Parse(host); err != nil {
		return nil, err
	}
	return client.New(client.WithHost(host), client.WithAPIVersionNegotiation())
}

func pullImage(cli *client.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ii, inspectErr := cli.ImageInspect(ctx, Image)
	if inspectErr == nil && len(ii.RepoDigests) > 0 {
		return nil
	}
	// docker load drops RepoDigests; re-pulling restores them (layers are cached, so only the manifest moves).
	pull, err := cli.ImagePull(ctx, Image, client.ImagePullOptions{})
	if err == nil {
		err = pull.Wait(ctx)
	}
	if err != nil && inspectErr == nil {
		return nil
	}
	return err
}

func labelled(extra map[string]string) map[string]string {
	l := map[string]string{LabelKey: runID}
	for k, v := range extra {
		l[k] = v
	}
	return l
}

// Run creates and starts a container from Image running cmd, labelled for cleanup, and returns its
// id. Extra labels are merged in; the container is force-removed with its volumes at cleanup.
func (d *Docker) Run(name string, cmd []string, labels map[string]string) string {
	d.t.Helper()
	return d.RunWith(name, container.Config{Cmd: cmd}, nil, labels)
}

// RunWith is Run with a caller-built container config and host config.
func (d *Docker) RunWith(name string, cfg container.Config, host *container.HostConfig, labels map[string]string) string {
	d.t.Helper()
	return d.RunNamed(d.Prefix()+name, cfg, host, labels)
}

// Prefix is the run-scoped name prefix RunWith, Volume and Network put before a name.
func (d *Docker) Prefix() string { return fmt.Sprintf("kira-flow-%s-", runID) }

// RunNamed is RunWith with the exact container name, for names a feature infers meaning from.
func (d *Docker) RunNamed(fullName string, cfg container.Config, host *container.HostConfig, labels map[string]string) string {
	d.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg.Image = Image
	cfg.Labels = labelled(labels)
	created, err := d.Cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: fullName, Config: &cfg, HostConfig: host})
	if err != nil {
		d.t.Fatalf("docker: create %s: %v", fullName, err)
	}
	id := created.ID
	d.t.Cleanup(func() {
		_, _ = d.Cli.ContainerRemove(context.Background(), id, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	})
	if _, err := d.Cli.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		d.t.Fatalf("docker: start %s: %v", fullName, err)
	}
	return id
}

// Volume creates a labelled named volume, removed at cleanup, and returns its name.
func (d *Docker) Volume(name string, labels map[string]string) string {
	d.t.Helper()
	full := fmt.Sprintf("kira-flow-%s-%s", runID, name)
	if _, err := d.Cli.VolumeCreate(context.Background(), client.VolumeCreateOptions{Name: full, Labels: labelled(labels)}); err != nil {
		d.t.Fatalf("docker: volume %s: %v", name, err)
	}
	d.t.Cleanup(func() {
		_, _ = d.Cli.VolumeRemove(context.Background(), full, client.VolumeRemoveOptions{Force: true})
	})
	return full
}

// Network creates a labelled network, removed at cleanup, and returns its id and name.
func (d *Docker) Network(name string) (id, fullName string) {
	d.t.Helper()
	fullName = fmt.Sprintf("kira-flow-%s-%s", runID, name)
	res, err := d.Cli.NetworkCreate(context.Background(), fullName, client.NetworkCreateOptions{Labels: labelled(nil)})
	if err != nil {
		d.t.Fatalf("docker: network %s: %v", name, err)
	}
	d.t.Cleanup(func() { _, _ = d.Cli.NetworkRemove(context.Background(), res.ID, client.NetworkRemoveOptions{}) })
	return res.ID, fullName
}

// Tag adds a run-scoped tag to Image, removed at cleanup, and returns the new reference.
func (d *Docker) Tag(name string) string {
	d.t.Helper()
	ref := fmt.Sprintf("kira-flowtest-%s/%s:latest", runID, name)
	if _, err := d.Cli.ImageTag(context.Background(), client.ImageTagOptions{Source: Image, Target: ref}); err != nil {
		d.t.Fatalf("docker: tag %s: %v", ref, err)
	}
	d.t.Cleanup(func() { _, _ = d.Cli.ImageRemove(context.Background(), ref, client.ImageRemoveOptions{}) })
	return ref
}

// sweep removes flow-test resources older than sweepAge, left by a crashed run.
func sweep(host string) {
	cli, err := newClient(host)
	if err != nil {
		return
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx, client.PingOptions{}); err != nil {
		return
	}
	filter := client.Filters{}.Add("label", LabelKey)
	cutoff := time.Now().Add(-sweepAge)
	if res, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: filter}); err == nil {
		for _, c := range res.Items {
			if time.Unix(c.Created, 0).Before(cutoff) {
				_, _ = cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
			}
		}
	}
	if res, err := cli.NetworkList(ctx, client.NetworkListOptions{Filters: filter}); err == nil {
		for _, n := range res.Items {
			if n.Created.Before(cutoff) {
				_, _ = cli.NetworkRemove(ctx, n.ID, client.NetworkRemoveOptions{})
			}
		}
	}
	if res, err := cli.VolumeList(ctx, client.VolumeListOptions{Filters: filter}); err == nil {
		for _, v := range res.Items {
			if created, err := time.Parse(time.RFC3339, v.CreatedAt); err == nil && created.Before(cutoff) {
				_, _ = cli.VolumeRemove(ctx, v.Name, client.VolumeRemoveOptions{Force: true})
			}
		}
	}
}

// BootGen simulates daemon restarts for the restore tracker: a real restart would kill the engine
// every other test shares. Bump gives the probe a new boot id and boot time.
type BootGen struct {
	gen atomic.Int64
	at  atomic.Int64
}

// Bump makes the next probe report a new daemon life.
func (b *BootGen) Bump() {
	b.at.Store(time.Now().UnixMilli())
	b.gen.Add(1)
}

// Probe wraps the production probe, appending the generation to the boot id.
func (b *BootGen) Probe() docker.BootProbe {
	return func(ctx context.Context, cli *client.Client) (string, int64, error) {
		id, at, err := docker.BridgeBootProbe(ctx, cli)
		if err != nil || b.gen.Load() == 0 {
			return id, at, err
		}
		return fmt.Sprintf("%s#%d", id, b.gen.Load()), b.at.Load(), nil
	}
}
