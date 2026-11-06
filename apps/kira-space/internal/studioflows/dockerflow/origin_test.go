package dockerflow

import (
	"sort"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/docker"
)

func TestContainerOrigin(t *testing.T) {
	app, d := boot(t)
	svc := app.W.Docker

	type row struct {
		Name           string `json:"name"`
		Origin         string `json:"origin"`
		OriginName     string `json:"originName"`
		ComposeProject string `json:"composeProject"`
		ComposeService string `json:"composeService"`
	}
	cases := []struct {
		name   string
		labels map[string]string
		want   row
	}{
		{"plain", nil, row{}},
		{"compose", map[string]string{"com.docker.compose.project": "shop", "com.docker.compose.service": "web"},
			row{Origin: "compose", OriginName: "shop", ComposeProject: "shop", ComposeService: "web"}},
		{"devc", map[string]string{"devcontainer.local_folder": "/work/kira", "com.docker.compose.project": "kira_devcontainer", "com.docker.compose.service": "app"},
			row{Origin: "devcontainer", OriginName: "kira", ComposeProject: "kira_devcontainer", ComposeService: "app"}},
		{"tc", map[string]string{"org.testcontainers": "true", "org.testcontainers.sessionId": "sess1"},
			row{Origin: "testcontainers", OriginName: "sess1"}},
		{"kind", map[string]string{"io.x-k8s.kind.cluster": "dev"}, row{Origin: "kind", OriginName: "dev"}},
		{"k8s", map[string]string{"io.kubernetes.pod.name": "api-1", "io.kubernetes.pod.namespace": "prod"},
			row{Origin: "kubernetes", OriginName: "prod/api-1"}},
		{"swarm", map[string]string{"com.docker.swarm.service.name": "stack_web"}, row{Origin: "swarm", OriginName: "stack_web"}},
	}
	ids := map[string]string{}
	want := map[string]row{}
	for _, c := range cases {
		id := sleeper(d, c.name, c.labels)
		ids[c.name] = id
		c.want.Name = d.Prefix() + c.name
		want[id] = c.want
	}
	stop := 1
	runID := strings.Split(d.Prefix(), "-")[2]
	bxName := "buildx_buildkit_kf" + runID
	bx := d.RunNamed(bxName, container.Config{Cmd: []string{"sleep", "300"}, StopTimeout: &stop}, nil, nil)
	want[bx] = row{Name: bxName, Origin: "buildx", OriginName: "kf" + runID}

	all, err := svc.Containers(docker.ListArgs{All: true})
	if err != nil {
		t.Fatal(err)
	}
	var got []row
	for id, w := range want {
		c := findContainer(all, id)
		if c == nil {
			t.Fatalf("container %s missing", w.Name)
		}
		r := row{c.Name, c.Origin, c.OriginName, c.ComposeProject, c.ComposeService}
		if r != w {
			t.Fatalf("list row = %+v, want %+v", r, w)
		}
		got = append(got, r)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })

	det, err := svc.InspectContainer(docker.IDArgs{ID: ids["devc"]})
	if err != nil {
		t.Fatal(err)
	}
	if det.Container.Origin != "devcontainer" || det.Container.OriginName != "kira" {
		t.Fatalf("inspect origin = %q %q, want devcontainer kira", det.Container.Origin, det.Container.OriginName)
	}
	app.Contract(t, "docker-disk", "DockerService.Containers#origins", got, flowharness.Replace(d.Prefix(), "kira-flow-"), flowharness.Replace(runID, "run"))
}
