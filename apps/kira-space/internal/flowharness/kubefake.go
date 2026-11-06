package flowharness

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// KubeGoodToken is the bearer token the fake API server accepts.
const KubeGoodToken = "good-token"

// KubeRequest is one request the fake API server saw.
type KubeRequest struct {
	Method, Path, Query string
}

// KubeFake is a fake Kubernetes API server. Responses are built from real apimachinery
// types, so shapes match a real server by construction. It answers GET only; anything else is a
// 405 that tests can find in Requests.
type KubeFake struct {
	URL string

	down string

	srv              *httptest.Server
	forbidNamespaces bool

	mu          sync.Mutex
	requests    []KubeRequest
	lines       map[string][]string
	followers   map[string][]chan string
	followEnded int
	resources   map[string]*kubeResource
}

// KubeFakeOption tweaks NewKubeFake.
type KubeFakeOption func(*KubeFake)

// ForbidNamespaces makes the namespace list answer 403.
func ForbidNamespaces() KubeFakeOption { return func(f *KubeFake) { f.forbidNamespaces = true } }

type kubeObj struct {
	obj runtime.Object
	// cells are the default table cells; the Name column is always first.
	cells []any
}

type kubeResource struct {
	group, version, resource, kind string
	namespaced                     bool
	columns                        []metav1.TableColumnDefinition
	objs                           []kubeObj
}

var kubeCreated = metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

func col(name, typ string, priority int32) metav1.TableColumnDefinition {
	return metav1.TableColumnDefinition{Name: name, Type: typ, Priority: priority}
}

var (
	nameAge   = []metav1.TableColumnDefinition{col("Name", "string", 0), col("Age", "string", 0)}
	podColumn = []metav1.TableColumnDefinition{
		col("Name", "string", 0), col("Ready", "string", 0), col("Status", "string", 0),
		col("Restarts", "integer", 0), col("Age", "string", 0), col("IP", "string", 1),
	}
)

func meta1(ns, name, uid string, lbls map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: name, Namespace: ns, UID: types.UID("uid-" + uid), Labels: lbls, CreationTimestamp: kubeCreated,
		ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply}},
	}
}

func fakePod(ns, name string, lbls map[string]string, containers []string, inits ...string) kubeObj {
	p := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: meta1(ns, name, "pod-"+ns+"-"+name, lbls),
		Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{
			{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: kubeCreated},
			{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, Reason: "Scheduled", Message: "assigned to node-1", LastTransitionTime: kubeCreated},
		}},
	}
	p.OwnerReferences = []metav1.OwnerReference{{Kind: "ReplicaSet", Name: name + "-rs", APIVersion: "apps/v1", UID: "uid-rs"}}
	for _, c := range inits {
		p.Spec.InitContainers = append(p.Spec.InitContainers, corev1.Container{Name: c})
	}
	for _, c := range containers {
		p.Spec.Containers = append(p.Spec.Containers, corev1.Container{Name: c, Image: "example/" + c + ":1"})
	}
	return kubeObj{p, []any{name, "1/1", "Running", 0, "5m", "10.0.0.1"}}
}

func newKubeResources() map[string]*kubeResource {
	web := map[string]string{"app": "web"}
	secret := &corev1.Secret{
		TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: meta1("default", "db-creds", "secret-db-creds", nil),
		Data:       map[string][]byte{"password": []byte("hunter2-secret"), "user": []byte("admin")},
		Type:       corev1.SecretTypeOpaque,
	}
	secret.Annotations = map[string]string{"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"password":"aHVudGVyMi1zZWNyZXQ="}}`, "owner": "team-a"}
	svc := &corev1.Service{
		TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
		ObjectMeta: meta1("default", "web-svc", "svc-web", web),
	}
	node := &corev1.Node{TypeMeta: metav1.TypeMeta{Kind: "Node", APIVersion: "v1"}, ObjectMeta: meta1("", "node-1", "node-1", nil)}
	dep := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
		ObjectMeta: meta1("default", "web", "dep-web", web),
	}
	widget := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1", "kind": "Widget",
		"metadata": map[string]any{"name": "w1", "namespace": "default", "uid": "uid-widget-w1", "creationTimestamp": "2026-01-01T00:00:00Z"},
	}}
	ns := func(name string) kubeObj {
		return kubeObj{&corev1.Namespace{TypeMeta: metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"}, ObjectMeta: meta1("", name, "ns-"+name, nil)}, []any{name, "5m"}}
	}
	return map[string]*kubeResource{
		"/v1/pods": {version: "v1", resource: "pods", kind: "Pod", namespaced: true, columns: podColumn, objs: []kubeObj{
			fakePod("default", "web-1", web, []string{"app", "sidecar"}, "init"),
			fakePod("default", "web-2", web, []string{"app"}),
			fakePod("default", "db-0", map[string]string{"app": "db"}, []string{"db"}),
			fakePod("kube-system", "coredns-1", map[string]string{"k8s-app": "kube-dns"}, []string{"coredns"}),
		}},
		"/v1/services": {version: "v1", resource: "services", kind: "Service", namespaced: true, columns: nameAge, objs: []kubeObj{{svc, []any{"web-svc", "5m"}}}},
		"/v1/secrets":  {version: "v1", resource: "secrets", kind: "Secret", namespaced: true, columns: nameAge, objs: []kubeObj{{secret, []any{"db-creds", "5m"}}}},
		"/v1/nodes":    {version: "v1", resource: "nodes", kind: "Node", columns: nameAge, objs: []kubeObj{{node, []any{"node-1", "5m"}}}},
		"/v1/namespaces": {version: "v1", resource: "namespaces", kind: "Namespace", columns: nameAge, objs: []kubeObj{
			ns("default"), ns("kube-system"), ns("apps"),
		}},
		"/v1/events":             {version: "v1", resource: "events", kind: "Event", namespaced: true, columns: nameAge},
		"apps/v1/deployments":    {group: "apps", version: "v1", resource: "deployments", kind: "Deployment", namespaced: true, columns: nameAge, objs: []kubeObj{{dep, []any{"web", "5m"}}}},
		"example.com/v1/widgets": {group: "example.com", version: "v1", resource: "widgets", kind: "Widget", namespaced: true, columns: nameAge, objs: []kubeObj{{widget, []any{"w1", "5m"}}}},
	}
}

// NewKubeFake starts the fake API server; it stops at test cleanup.
func NewKubeFake(t testing.TB, opts ...KubeFakeOption) *KubeFake {
	t.Helper()
	f := &KubeFake{
		lines:     map[string][]string{"web-1": {"line one", "line two", "line three"}},
		followers: map[string][]chan string{}, resources: newKubeResources(),
	}
	for _, o := range opts {
		o(f)
	}
	// client-go sends credentials only over TLS, so the fake serves HTTPS with its self-signed cert.
	f.srv = httptest.NewUnstartedServer(http.HandlerFunc(f.serve))
	f.srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	f.srv.StartTLS()
	f.URL = f.srv.URL
	t.Cleanup(f.srv.Close)
	return f
}

// DownURL is the closed-port server URL the "down" context points at; set by WriteKubeconfig.
func (f *KubeFake) DownURL() string { return f.down }

// Requests returns the requests seen so far whose path contains sub ("" for all).
func (f *KubeFake) Requests(sub string) []KubeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []KubeRequest
	for _, r := range f.requests {
		if strings.Contains(r.Path, sub) {
			out = append(out, r)
		}
	}
	return out
}

// AppendLog adds a line to a pod's log and sends it to every follower.
func (f *KubeFake) AppendLog(pod, line string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines[pod] = append(f.lines[pod], line)
	for _, ch := range f.followers[pod] {
		select {
		case ch <- line:
		default:
		}
	}
}

// FollowEnded is how many follow log requests have ended.
func (f *KubeFake) FollowEnded() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.followEnded
}

// Followers is how many follow log requests are open now.
func (f *KubeFake) Followers() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.followers {
		n += len(c)
	}
	return n
}

func (f *KubeFake) status(w http.ResponseWriter, code int, reason metav1.StatusReason, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}, Status: metav1.StatusFailure,
		Message: msg, Reason: reason, Code: int32(code),
	})
}

func (f *KubeFake) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *KubeFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, KubeRequest{r.Method, r.URL.Path, r.URL.RawQuery})
	f.mu.Unlock()
	if r.Method != http.MethodGet {
		f.status(w, http.StatusMethodNotAllowed, metav1.StatusReasonMethodNotAllowed, "read only fake")
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+KubeGoodToken {
		f.status(w, http.StatusUnauthorized, metav1.StatusReasonUnauthorized, "Unauthorized")
		return
	}
	segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.URL.Path == "/version":
		f.json(w, map[string]string{"major": "1", "minor": "31", "gitVersion": "v1.31.0-fake"})
	case r.URL.Path == "/api":
		f.json(w, metav1.APIVersions{TypeMeta: metav1.TypeMeta{Kind: "APIVersions"}, Versions: []string{"v1"}})
	case r.URL.Path == "/apis":
		f.json(w, f.groups())
	case len(segs) == 2 && segs[0] == "api":
		f.json(w, f.resourceList("", segs[1]))
	case len(segs) == 3 && segs[0] == "apis":
		if segs[1] == "broken.example.com" {
			f.status(w, http.StatusServiceUnavailable, metav1.StatusReasonServiceUnavailable, "group down")
			return
		}
		f.json(w, f.resourceList(segs[1], segs[2]))
	case segs[0] == "api" && len(segs) > 2:
		f.object(w, r, "", segs[1], segs[2:])
	case segs[0] == "apis" && len(segs) > 3:
		f.object(w, r, segs[1], segs[2], segs[3:])
	default:
		f.status(w, http.StatusNotFound, metav1.StatusReasonNotFound, "no route "+r.URL.Path)
	}
}

func (f *KubeFake) groups() metav1.APIGroupList {
	g := func(name string) metav1.APIGroup {
		v := metav1.GroupVersionForDiscovery{GroupVersion: name + "/v1", Version: "v1"}
		return metav1.APIGroup{Name: name, Versions: []metav1.GroupVersionForDiscovery{v}, PreferredVersion: v}
	}
	return metav1.APIGroupList{TypeMeta: metav1.TypeMeta{Kind: "APIGroupList"}, Groups: []metav1.APIGroup{g("apps"), g("example.com"), g("broken.example.com")}}
}

func (f *KubeFake) resourceList(group, version string) metav1.APIResourceList {
	gv := version
	if group != "" {
		gv = group + "/" + version
	}
	out := metav1.APIResourceList{TypeMeta: metav1.TypeMeta{Kind: "APIResourceList"}, GroupVersion: gv}
	for _, res := range f.resources {
		if res.group != group || res.version != version {
			continue
		}
		verbs := metav1.Verbs{"get", "list", "watch"}
		out.APIResources = append(out.APIResources, metav1.APIResource{
			Name: res.resource, SingularName: strings.ToLower(res.kind), Namespaced: res.namespaced, Kind: res.kind, Verbs: verbs,
		})
		if res.resource == "pods" {
			out.APIResources = append(out.APIResources,
				metav1.APIResource{Name: "pods/log", Namespaced: true, Kind: "Pod", Verbs: metav1.Verbs{"get"}},
				metav1.APIResource{Name: "pods/status", Namespaced: true, Kind: "Pod", Verbs: metav1.Verbs{"get"}})
		}
	}
	return out
}

// object routes a resource path: [resource], [resource name], [namespaces ns resource], [namespaces ns resource name ...].
func (f *KubeFake) object(w http.ResponseWriter, r *http.Request, group, version string, rest []string) {
	ns := ""
	if len(rest) >= 3 && rest[0] == "namespaces" {
		ns, rest = rest[1], rest[2:]
	}
	res := f.resources[group+"/"+version+"/"+rest[0]]
	if res == nil {
		f.status(w, http.StatusNotFound, metav1.StatusReasonNotFound, "no resource "+rest[0])
		return
	}
	switch len(rest) {
	case 1:
		f.list(w, r, res, ns)
	case 2:
		f.get(w, res, ns, rest[1])
	case 3:
		if res.resource == "pods" && rest[2] == "log" {
			f.log(w, r, ns, rest[1])
			return
		}
		fallthrough
	default:
		f.status(w, http.StatusNotFound, metav1.StatusReasonNotFound, "no route")
	}
}

func accessor(o runtime.Object) metav1.Object {
	a, err := meta.Accessor(o)
	if err != nil {
		panic(err)
	}
	return a
}

func (f *KubeFake) get(w http.ResponseWriter, res *kubeResource, ns, name string) {
	for _, o := range res.objs {
		a := accessor(o.obj)
		if a.GetName() == name && (ns == "" || a.GetNamespace() == ns) {
			f.json(w, o.obj)
			return
		}
	}
	f.status(w, http.StatusNotFound, metav1.StatusReasonNotFound, fmt.Sprintf("%s %q not found", res.resource, name))
}

func (f *KubeFake) list(w http.ResponseWriter, r *http.Request, res *kubeResource, ns string) {
	q := r.URL.Query()
	if res.resource == "namespaces" && f.forbidNamespaces && !strings.Contains(r.Header.Get("Accept"), "as=Table") {
		f.status(w, http.StatusForbidden, metav1.StatusReasonForbidden, "namespaces is forbidden")
		return
	}
	sel := labels.Everything()
	if s := q.Get("labelSelector"); s != "" {
		var err error
		if sel, err = labels.Parse(s); err != nil {
			f.status(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
			return
		}
	}
	var items []kubeObj
	for _, o := range res.objs {
		a := accessor(o.obj)
		if ns != "" && a.GetNamespace() != ns {
			continue
		}
		if sel.Matches(labels.Set(a.GetLabels())) {
			items = append(items, o)
		}
	}
	if res.resource == "events" {
		f.events(w, q.Get("fieldSelector"), ns)
		return
	}
	if res.resource == "namespaces" && !strings.Contains(r.Header.Get("Accept"), "as=Table") {
		list := corev1.NamespaceList{TypeMeta: metav1.TypeMeta{Kind: "NamespaceList", APIVersion: "v1"}}
		for _, o := range items {
			list.Items = append(list.Items, *o.obj.(*corev1.Namespace))
		}
		f.json(w, list)
		return
	}
	start, limit := 0, len(items)
	if c := q.Get("continue"); c != "" {
		start, _ = strconv.Atoi(c)
	}
	if l, _ := strconv.Atoi(q.Get("limit")); l > 0 {
		limit = l
	}
	end := min(start+limit, len(items))
	table := metav1.Table{TypeMeta: metav1.TypeMeta{Kind: "Table", APIVersion: "meta.k8s.io/v1"}, ColumnDefinitions: res.columns}
	if end < len(items) {
		table.Continue = strconv.Itoa(end)
	}
	for _, o := range items[start:end] {
		a := accessor(o.obj)
		pom, _ := json.Marshal(metav1.PartialObjectMetadata{
			TypeMeta:   metav1.TypeMeta{Kind: "PartialObjectMetadata", APIVersion: "meta.k8s.io/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: a.GetName(), Namespace: a.GetNamespace(), UID: a.GetUID()},
		})
		table.Rows = append(table.Rows, metav1.TableRow{Cells: o.cells, Object: runtime.RawExtension{Raw: pom}})
	}
	f.json(w, table)
}

func (f *KubeFake) events(w http.ResponseWriter, fieldSelector, ns string) {
	uid := strings.TrimPrefix(fieldSelector, "involvedObject.uid=")
	list := corev1.EventList{TypeMeta: metav1.TypeMeta{Kind: "EventList", APIVersion: "v1"}}
	ev := func(name, reason, msg string, sec int) corev1.Event {
		ts := metav1.NewTime(kubeCreated.Add(time.Duration(sec) * time.Second))
		return corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: name, Namespace: "default", CreationTimestamp: ts},
			InvolvedObject: corev1.ObjectReference{UID: "uid-pod-default-web-1"}, Reason: reason, Message: msg, Type: "Normal",
			Count: 1, FirstTimestamp: ts, LastTimestamp: ts, Source: corev1.EventSource{Component: "kubelet"},
		}
	}
	if uid == "uid-pod-default-web-1" && (ns == "" || ns == "default") {
		list.Items = []corev1.Event{ev("e1", "Pulled", "image pulled", 1), ev("e2", "Started", "container started", 2)}
	}
	f.json(w, list)
}

func (f *KubeFake) log(w http.ResponseWriter, r *http.Request, ns, pod string) {
	res := f.resources["/v1/pods"]
	var p *corev1.Pod
	for _, o := range res.objs {
		if a := accessor(o.obj); a.GetName() == pod && a.GetNamespace() == ns {
			p = o.obj.(*corev1.Pod)
		}
	}
	if p == nil {
		f.status(w, http.StatusNotFound, metav1.StatusReasonNotFound, fmt.Sprintf("pods %q not found", pod))
		return
	}
	q := r.URL.Query()
	container := q.Get("container")
	if container == "" {
		container = p.Spec.Containers[0].Name
	}
	valid := false
	for _, c := range append(append([]corev1.Container{}, p.Spec.InitContainers...), p.Spec.Containers...) {
		valid = valid || c.Name == container
	}
	if !valid {
		f.status(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, fmt.Sprintf("container %s is not valid for pod %s", container, pod))
		return
	}
	f.mu.Lock()
	lines := append([]string(nil), f.lines[pod]...)
	f.mu.Unlock()
	if q.Get("previous") == "true" {
		if pod != "web-1" || container != "app" {
			f.status(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, "previous terminated container "+container+" in pod "+pod+" not found")
			return
		}
		lines = []string{"prev one", "prev two"}
	}
	if n, err := strconv.Atoi(q.Get("tailLines")); err == nil && n < len(lines) {
		lines = lines[len(lines)-n:]
	}
	stamp := q.Get("timestamps") == "true"
	render := func(i int, l string) string {
		if stamp {
			return fmt.Sprintf("2026-01-01T00:00:%02d.000000000Z %s\n", i%60, l)
		}
		return l + "\n"
	}
	w.Header().Set("Content-Type", "text/plain")
	var body strings.Builder
	for i, l := range lines {
		body.WriteString(render(i, l))
	}
	out := body.String()
	if lim, err := strconv.Atoi(q.Get("limitBytes")); err == nil && lim < len(out) {
		out = out[:lim]
	}
	_, _ = w.Write([]byte(out))
	if q.Get("follow") != "true" {
		return
	}
	fl, _ := w.(http.Flusher)
	fl.Flush()
	ch := make(chan string, 16)
	f.mu.Lock()
	f.followers[pod] = append(f.followers[pod], ch)
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		cur := f.followers[pod]
		for i, c := range cur {
			if c == ch {
				f.followers[pod] = append(cur[:i:i], cur[i+1:]...)
				break
			}
		}
		f.followEnded++
		f.mu.Unlock()
	}()
	n := len(lines)
	for {
		select {
		case l := <-ch:
			n++
			_, _ = w.Write([]byte(render(n, l)))
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// KubeconfigOpts tunes WriteKubeconfig.
type KubeconfigOpts struct {
	// BinDir receives the fake exec credential plugin kira-fake-cred.
	BinDir string
}

// WriteKubeconfig writes a kubeconfig with four contexts: kind-dev (current, token ok), plugin-ctx
// (exec plugin kira-fake-cred), bad-token (401) and down (closed port).
func (f *KubeFake) WriteKubeconfig(t testing.TB, path string, o KubeconfigOpts) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "https://" + l.Addr().String()
	f.down = closed
	_ = l.Close()
	plugin := filepath.Join(o.BinDir, "kira-fake-cred")
	script := fmt.Sprintf("#!/bin/sh\n[ -e \"$(dirname \"$0\")/kira-fake-cred.fail\" ] && exit 1\nprintf '%%s' '{\"apiVersion\":\"client.authentication.k8s.io/v1\",\"kind\":\"ExecCredential\",\"status\":{\"token\":\"%s\"}}'\n", KubeGoodToken)
	if err := os.WriteFile(plugin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	caData := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.srv.Certificate().Raw}))
	cfg := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: kind-dev
clusters:
- name: kind-dev
  cluster: {server: %[1]s, certificate-authority-data: %[4]s}
- name: down
  cluster: {server: %[2]s, insecure-skip-tls-verify: true}
contexts:
- name: kind-dev
  context: {cluster: kind-dev, user: dev-user, namespace: default}
- name: plugin-ctx
  context: {cluster: kind-dev, user: plugin-user}
- name: bad-token
  context: {cluster: kind-dev, user: bad-user}
- name: down
  context: {cluster: down, user: dev-user}
users:
- name: dev-user
  user: {token: %[3]s}
- name: bad-user
  user: {token: wrong-token}
- name: plugin-user
  user:
    exec: {apiVersion: client.authentication.k8s.io/v1, command: kira-fake-cred, interactiveMode: Never}
`, f.URL, closed, KubeGoodToken, caData)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

// B64 is a helper for tests that look for a Secret value in its encoded form.
func B64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
