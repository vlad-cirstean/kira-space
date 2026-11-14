package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"path"
	"sort"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const (
	actionTimeout = 60 * time.Second

	labelComposeProject = "com.docker.compose.project"
	labelComposeService = "com.docker.compose.service"

	labelDevcontainer    = "devcontainer.local_folder"
	labelTestcontainers  = "org.testcontainers"
	labelTestcontSession = "org.testcontainers.sessionId"
	labelKindCluster     = "io.x-k8s.kind.cluster"
	labelPodName         = "io.kubernetes.pod.name"
	labelPodNamespace    = "io.kubernetes.pod.namespace"
	labelSwarmService    = "com.docker.swarm.service.name"

	buildxPrefix = "buildx_buildkit_"
)

// originOf infers what created a container from its labels and name; first match wins. "" when
// nothing is recognised.
func originOf(labels map[string]string, name string) (origin, originName string) {
	switch {
	case labels[labelDevcontainer] != "":
		return "devcontainer", path.Base(strings.ReplaceAll(labels[labelDevcontainer], "\\", "/"))
	case labels[labelTestcontainers] == "true":
		return "testcontainers", labels[labelTestcontSession]
	case labels[labelKindCluster] != "":
		return "kind", labels[labelKindCluster]
	case labels[labelPodName] != "":
		return "kubernetes", labels[labelPodNamespace] + "/" + labels[labelPodName]
	case labels[labelSwarmService] != "":
		return "swarm", labels[labelSwarmService]
	case strings.HasPrefix(name, buildxPrefix):
		return "buildx", strings.TrimPrefix(name, buildxPrefix)
	case labels[labelComposeProject] != "":
		return "compose", labels[labelComposeProject]
	}
	return "", ""
}

// Port is one published or exposed container port.
type Port struct {
	IP          string `json:"ip"`
	PrivatePort int    `json:"privatePort"`
	PublicPort  int    `json:"publicPort"`
	Type        string `json:"type"`
}

// Container is one row of the container list.
type Container struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Image          string   `json:"image"`
	ImageID        string   `json:"imageId"`
	State          string   `json:"state"`
	Status         string   `json:"status"`
	Created        int64    `json:"created"`
	Ports          []Port   `json:"ports"`
	ComposeProject string   `json:"composeProject"`
	ComposeService string   `json:"composeService"`
	Origin         string   `json:"origin"`
	OriginName     string   `json:"originName"`
	Networks       []string `json:"networks"`
}

// Image is one row of the image list.
type Image struct {
	ID         string   `json:"id"`
	Tags       []string `json:"tags"`
	Size       int64    `json:"size"`
	Created    int64    `json:"created"`
	Containers int      `json:"containers"`
	Dangling   bool     `json:"dangling"`
}

// Volume is one row of the volume list.
type Volume struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	Scope      string            `json:"scope"`
	Created    string            `json:"created"`
	Labels     map[string]string `json:"labels"`
	UsedBy     []string          `json:"usedBy"`
}

// Network is one row of the network list.
type Network struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Driver     string   `json:"driver"`
	Scope      string   `json:"scope"`
	Internal   bool     `json:"internal"`
	Subnets    []string `json:"subnets"`
	Containers int      `json:"containers"`
	Builtin    bool     `json:"builtin"`
	UsedBy     []string `json:"usedBy"`
}

// Mount is one container mount.
type Mount struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	RW          bool   `json:"rw"`
}

// NetworkAttachment is a container's endpoint on one network.
type NetworkAttachment struct {
	Network    string   `json:"network"`
	IPAddress  string   `json:"ipAddress"`
	Gateway    string   `json:"gateway"`
	MacAddress string   `json:"macAddress"`
	Aliases    []string `json:"aliases"`
}

// ContainerDetail is the inspect view of one container.
type ContainerDetail struct {
	Container          Container           `json:"container"`
	Command            []string            `json:"command"`
	Entrypoint         []string            `json:"entrypoint"`
	Env                []string            `json:"env"`
	WorkingDir         string              `json:"workingDir"`
	User               string              `json:"user"`
	RestartPolicy      string              `json:"restartPolicy"`
	Health             string              `json:"health"`
	StartedAt          string              `json:"startedAt"`
	FinishedAt         string              `json:"finishedAt"`
	ExitCode           int                 `json:"exitCode"`
	TTY                bool                `json:"tty"`
	Mounts             []Mount             `json:"mounts"`
	Labels             map[string]string   `json:"labels"`
	NetworkAttachments []NetworkAttachment `json:"networkAttachments"`
	Raw                string              `json:"raw"`
}

// InspectResult is the raw inspect JSON of an image, volume or network.
type InspectResult struct {
	Raw string `json:"raw"`
}

func addrString(a netip.Addr) string {
	if !a.IsValid() {
		return ""
	}
	return a.String()
}

func trimName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func containerFromSummary(s container.Summary) Container {
	c := Container{
		ID: s.ID, Name: trimName(s.Names), Image: s.Image, ImageID: s.ImageID, State: string(s.State),
		Status: s.Status, Created: s.Created, Ports: make([]Port, 0, len(s.Ports)),
		ComposeProject: s.Labels[labelComposeProject], ComposeService: s.Labels[labelComposeService],
		Networks: []string{},
	}
	c.Origin, c.OriginName = originOf(s.Labels, c.Name)
	for _, p := range s.Ports {
		c.Ports = append(c.Ports, Port{IP: addrString(p.IP), PrivatePort: int(p.PrivatePort), PublicPort: int(p.PublicPort), Type: p.Type})
	}
	if s.NetworkSettings != nil {
		c.Networks = sortedKeys(s.NetworkSettings.Networks)
	}
	return c
}

// call runs fn with the call timeout and maps its error onto an ipcerr.
func (m *Manager) call(ctx context.Context, timeout time.Duration, fn func(ctx context.Context, cli *client.Client) error) error {
	cli, ep, err := m.client()
	if err != nil {
		return m.mapErr(ep, err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return m.mapErr(ep, fn(ctx, cli))
}

func isConnErr(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded)
}

func (m *Manager) mapErr(ep Endpoint, err error) error {
	if err == nil {
		return nil
	}
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie
	}
	switch {
	case cerrdefs.IsNotFound(err):
		return ipcerr.T(ipcerr.CodeNotFound, "backend.core.docker.notFound.body", nil)
	case cerrdefs.IsInvalidArgument(err), cerrdefs.IsPermissionDenied(err):
		return ipcerr.BadRequest(err.Error())
	case cerrdefs.IsConflict(err):
		return ipcerr.New(string(ipcerr.CodeConflict), err.Error())
	}
	if !ep.Remote || isConnErr(err) {
		if reason := m.classify(ep, err); reason != reasonError {
			e := ipcerr.New(string(ipcerr.CodeUnavailable), err.Error())
			e.Details, _ = json.Marshal(map[string]string{"reason": reason})
			return e
		}
	}
	return ipcerr.Internal(err.Error())
}

func (m *Manager) containers(ctx context.Context, all bool) ([]Container, error) {
	var out []Container
	err := m.call(ctx, callTimeout, func(ctx context.Context, cli *client.Client) error {
		res, err := cli.ContainerList(ctx, client.ContainerListOptions{All: all})
		if err != nil {
			return err
		}
		out = make([]Container, 0, len(res.Items))
		for _, s := range res.Items {
			out = append(out, containerFromSummary(s))
		}
		return nil
	})
	return out, err
}

func (m *Manager) listAllContainers(ctx context.Context, cli *client.Client) ([]container.Summary, error) {
	res, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	return res.Items, err
}

func (m *Manager) images(ctx context.Context) ([]Image, error) {
	var out []Image
	err := m.call(ctx, callTimeout, func(ctx context.Context, cli *client.Client) error {
		res, err := cli.ImageList(ctx, client.ImageListOptions{})
		if err != nil {
			return err
		}
		cs, err := m.listAllContainers(ctx, cli)
		if err != nil {
			return err
		}
		inUse := map[string]int{}
		for _, c := range cs {
			inUse[c.ImageID]++
		}
		out = make([]Image, 0, len(res.Items))
		for _, s := range res.Items {
			tags := make([]string, 0, len(s.RepoTags))
			for _, t := range s.RepoTags {
				if t != "<none>:<none>" {
					tags = append(tags, t)
				}
			}
			out = append(out, Image{ID: s.ID, Tags: tags, Size: s.Size, Created: s.Created, Containers: inUse[s.ID], Dangling: len(tags) == 0})
		}
		return nil
	})
	return out, err
}

func (m *Manager) volumes(ctx context.Context) ([]Volume, error) {
	var out []Volume
	err := m.call(ctx, callTimeout, func(ctx context.Context, cli *client.Client) error {
		res, err := cli.VolumeList(ctx, client.VolumeListOptions{})
		if err != nil {
			return err
		}
		cs, err := m.listAllContainers(ctx, cli)
		if err != nil {
			return err
		}
		usedBy := map[string][]string{}
		for _, c := range cs {
			for _, mp := range c.Mounts {
				if mp.Name != "" {
					usedBy[mp.Name] = append(usedBy[mp.Name], trimName(c.Names))
				}
			}
		}
		out = make([]Volume, 0, len(res.Items))
		for _, v := range res.Items {
			used := usedBy[v.Name]
			if used == nil {
				used = []string{}
			}
			out = append(out, Volume{Name: v.Name, Driver: v.Driver, Mountpoint: v.Mountpoint, Scope: v.Scope, Created: v.CreatedAt, Labels: v.Labels, UsedBy: used})
		}
		return nil
	})
	return out, err
}

func (m *Manager) networks(ctx context.Context) ([]Network, error) {
	var out []Network
	err := m.call(ctx, callTimeout, func(ctx context.Context, cli *client.Client) error {
		res, err := cli.NetworkList(ctx, client.NetworkListOptions{})
		if err != nil {
			return err
		}
		cs, err := m.listAllContainers(ctx, cli)
		if err != nil {
			return err
		}
		usedBy := map[string][]string{}
		for _, c := range cs {
			if c.NetworkSettings == nil {
				continue
			}
			for name := range c.NetworkSettings.Networks {
				usedBy[name] = append(usedBy[name], trimName(c.Names))
			}
		}
		out = make([]Network, 0, len(res.Items))
		for _, n := range res.Items {
			subnets := []string{}
			for _, cfg := range n.IPAM.Config {
				if cfg.Subnet.IsValid() {
					subnets = append(subnets, cfg.Subnet.String())
				}
			}
			used := usedBy[n.Name]
			if used == nil {
				used = []string{}
			}
			out = append(out, Network{
				ID: n.ID, Name: n.Name, Driver: n.Driver, Scope: n.Scope, Internal: n.Internal, Subnets: subnets,
				Containers: len(used), Builtin: n.Name == "bridge" || n.Name == "host" || n.Name == "none", UsedBy: used,
			})
		}
		return nil
	})
	return out, err
}

func indentJSON(raw []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

func (m *Manager) inspectContainer(ctx context.Context, id string) (ContainerDetail, error) {
	var out ContainerDetail
	err := m.call(ctx, callTimeout, func(ctx context.Context, cli *client.Client) error {
		res, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
		if err != nil {
			return err
		}
		out = detailFromInspect(res.Container, res.Raw)
		return nil
	})
	return out, err
}

func detailFromInspect(r container.InspectResponse, raw []byte) ContainerDetail {
	d := ContainerDetail{
		Command: []string{}, Entrypoint: []string{}, Env: []string{}, Mounts: []Mount{}, Labels: map[string]string{},
		NetworkAttachments: []NetworkAttachment{}, Raw: indentJSON(raw),
	}
	c := Container{ID: r.ID, Name: strings.TrimPrefix(r.Name, "/"), ImageID: r.Image, Ports: []Port{}, Networks: []string{}}
	if t, err := time.Parse(time.RFC3339Nano, r.Created); err == nil {
		c.Created = t.Unix()
	}
	if r.State != nil {
		c.State, c.Status = string(r.State.Status), string(r.State.Status)
		d.StartedAt, d.FinishedAt, d.ExitCode = r.State.StartedAt, r.State.FinishedAt, r.State.ExitCode
		if r.State.Health != nil {
			d.Health = string(r.State.Health.Status)
		}
	}
	if r.Config != nil {
		cfg := r.Config
		c.Image = cfg.Image
		c.ComposeProject, c.ComposeService = cfg.Labels[labelComposeProject], cfg.Labels[labelComposeService]
		c.Origin, c.OriginName = originOf(cfg.Labels, c.Name)
		d.Command, d.Entrypoint, d.Env = nonNil(cfg.Cmd), nonNil(cfg.Entrypoint), nonNil(cfg.Env)
		d.WorkingDir, d.User, d.TTY = cfg.WorkingDir, cfg.User, cfg.Tty
		if cfg.Labels != nil {
			d.Labels = cfg.Labels
		}
	}
	if r.HostConfig != nil {
		d.RestartPolicy = string(r.HostConfig.RestartPolicy.Name)
		if r.HostConfig.RestartPolicy.MaximumRetryCount > 0 {
			d.RestartPolicy += fmt.Sprintf(":%d", r.HostConfig.RestartPolicy.MaximumRetryCount)
		}
	}
	for _, mp := range r.Mounts {
		d.Mounts = append(d.Mounts, Mount{Type: string(mp.Type), Name: mp.Name, Source: mp.Source, Destination: mp.Destination, Mode: mp.Mode, RW: mp.RW})
	}
	if ns := r.NetworkSettings; ns != nil {
		for port, bindings := range ns.Ports {
			for _, b := range bindings {
				var pub int
				_, _ = fmt.Sscanf(b.HostPort, "%d", &pub)
				c.Ports = append(c.Ports, Port{IP: addrString(b.HostIP), PrivatePort: int(port.Num()), PublicPort: pub, Type: string(port.Proto())})
			}
		}
		sort.Slice(c.Ports, func(i, j int) bool {
			if c.Ports[i].PrivatePort != c.Ports[j].PrivatePort {
				return c.Ports[i].PrivatePort < c.Ports[j].PrivatePort
			}
			return c.Ports[i].PublicPort < c.Ports[j].PublicPort
		})
		c.Networks = sortedKeys(ns.Networks)
		for _, name := range c.Networks {
			e := ns.Networks[name]
			if e == nil {
				continue
			}
			d.NetworkAttachments = append(d.NetworkAttachments, NetworkAttachment{
				Network: name, IPAddress: addrString(e.IPAddress), Gateway: addrString(e.Gateway),
				MacAddress: e.MacAddress.String(), Aliases: nonNil(e.Aliases),
			})
		}
	}
	d.Container = c
	return d
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (m *Manager) inspect(ctx context.Context, kind, id string) (InspectResult, error) {
	var out InspectResult
	err := m.call(ctx, callTimeout, func(ctx context.Context, cli *client.Client) error {
		switch kind {
		case "image":
			var buf bytes.Buffer
			if _, err := cli.ImageInspect(ctx, id, client.ImageInspectWithRawResponse(&buf)); err != nil {
				return err
			}
			out.Raw = indentJSON(buf.Bytes())
		case "volume":
			res, err := cli.VolumeInspect(ctx, id, client.VolumeInspectOptions{})
			if err != nil {
				return err
			}
			out.Raw = indentJSON(res.Raw)
		case "network":
			res, err := cli.NetworkInspect(ctx, id, client.NetworkInspectOptions{})
			if err != nil {
				return err
			}
			out.Raw = indentJSON(res.Raw)
		default:
			return ipcerr.Contract("kind must be image, volume or network")
		}
		return nil
	})
	return out, err
}

func (m *Manager) start(ctx context.Context, id string) error {
	return m.call(ctx, actionTimeout, func(ctx context.Context, cli *client.Client) error {
		_, err := cli.ContainerStart(ctx, id, client.ContainerStartOptions{})
		return err
	})
}

func (m *Manager) stop(ctx context.Context, id string) error {
	return m.call(ctx, actionTimeout, func(ctx context.Context, cli *client.Client) error {
		_, err := cli.ContainerStop(ctx, id, client.ContainerStopOptions{})
		return err
	})
}

func (m *Manager) restart(ctx context.Context, id string) error {
	if _, held := m.restarting.LoadOrStore(id, struct{}{}); held {
		return ipcerr.T(ipcerr.CodeConflict, "backend.core.docker.restartInProgress.body", nil)
	}
	defer m.restarting.Delete(id)
	return m.call(ctx, actionTimeout, func(ctx context.Context, cli *client.Client) error {
		_, err := cli.ContainerRestart(ctx, id, client.ContainerRestartOptions{})
		return err
	})
}
