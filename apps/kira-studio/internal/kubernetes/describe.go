package kubernetes

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

const (
	maxEvents       = 200
	lastAppliedAnno = "kubectl.kubernetes.io/last-applied-configuration"
	secretsKey      = "v1/secrets"
)

// Describe returns one object's overview, conditions, events and manifest. Secret values are
// replaced by their byte counts before anything is rendered.
func (s *Service) Describe(ctx context.Context, args DescribeArgs) (Description, error) {
	if args.Name == "" {
		return Description{}, ipcerr.Required("name")
	}
	var d Description
	err := s.record(ctx, args.ClusterID, args.Background, func(n string) string { return DescribeCommand(n, args) }, func(ctx context.Context, note *OpNote) error {
		var err error
		if d, err = s.describe(ctx, args); err == nil {
			note.SetRows(1)
		}
		return err
	})
	if err != nil {
		return Description{}, err
	}
	return d, nil
}

func (s *Service) describe(ctx context.Context, args DescribeArgs) (Description, error) {
	cs, err := s.client(args.ClusterID)
	if err != nil {
		return Description{}, mapErr(err)
	}
	kind, err := s.findKind(ctx, cs, args.Kind)
	if err != nil {
		return Description{}, mapErr(err)
	}
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	raw, err := cs.rest.Get().AbsPath(resourcePath(kind, args.Namespace, args.Name)).SetHeader("Accept", jsonAccept).Do(cctx).Raw()
	if err != nil {
		return Description{}, mapErr(err)
	}
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(raw); err != nil {
		return Description{}, ipcerr.Internal(err.Error())
	}
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	if kind.Key == secretsKey {
		redactSecret(obj)
	}
	manifest, err := yaml.Marshal(obj.Object)
	if err != nil {
		return Description{}, ipcerr.Internal(err.Error())
	}
	d := Description{
		Kind: obj.GetKind(), APIVersion: obj.GetAPIVersion(), Name: obj.GetName(), Namespace: obj.GetNamespace(),
		UID: string(obj.GetUID()), Created: obj.GetCreationTimestamp().UTC().Format(time.RFC3339),
		Labels: orEmpty(obj.GetLabels()), Annotations: orEmpty(obj.GetAnnotations()),
		Owners: owners(obj), Conditions: conditions(obj), Containers: containers(obj),
		Manifest: string(manifest),
	}
	d.Events = s.eventsFor(cctx, cs, kind, d)
	return d, nil
}

func orEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func redactSecret(obj *unstructured.Unstructured) {
	for _, field := range []string{"data", "stringData"} {
		m, found, _ := unstructured.NestedMap(obj.Object, field)
		if !found {
			continue
		}
		for k, v := range m {
			str, _ := v.(string)
			n := len(str)
			if field == "data" {
				if dec, err := base64.StdEncoding.DecodeString(str); err == nil {
					n = len(dec)
				}
			}
			m[k] = fmt.Sprintf("<%d bytes>", n)
		}
		_ = unstructured.SetNestedMap(obj.Object, m, field)
	}
	annos := obj.GetAnnotations()
	if _, ok := annos[lastAppliedAnno]; ok {
		delete(annos, lastAppliedAnno)
		if len(annos) == 0 {
			unstructured.RemoveNestedField(obj.Object, "metadata", "annotations")
		} else {
			obj.SetAnnotations(annos)
		}
	}
}

func owners(obj *unstructured.Unstructured) []Owner {
	refs := obj.GetOwnerReferences()
	out := make([]Owner, 0, len(refs))
	for _, o := range refs {
		out = append(out, Owner{Kind: o.Kind, Name: o.Name})
	}
	return out
}

func conditions(obj *unstructured.Unstructured) []Condition {
	out := []Condition{}
	items, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		str := func(k string) string { v, _ := m[k].(string); return v }
		out = append(out, Condition{
			Type: str("type"), Status: str("status"), Reason: str("reason"), Message: str("message"),
			LastTransition: str("lastTransitionTime"),
		})
	}
	return out
}

func containers(obj *unstructured.Unstructured) []ContainerRef {
	out := []ContainerRef{}
	if obj.GetKind() != "Pod" {
		return out
	}
	for _, f := range []struct {
		field string
		init  bool
	}{{"initContainers", true}, {"containers", false}, {"ephemeralContainers", false}} {
		items, _, _ := unstructured.NestedSlice(obj.Object, "spec", f.field)
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				if name, _ := m["name"].(string); name != "" {
					out = append(out, ContainerRef{Name: name, Init: f.init})
				}
			}
		}
	}
	return out
}

// eventsFor lists events whose involved object has the described uid. A failure leaves the list empty.
func (s *Service) eventsFor(ctx context.Context, cs *clusterClient, kind Kind, d Description) []Event {
	out := []Event{}
	if d.UID == "" {
		return out
	}
	ns := ""
	if kind.Namespaced {
		ns = d.Namespace
	}
	list, err := cs.core.Events(ns).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.uid=" + d.UID, Limit: 500})
	if err != nil {
		slog.Debug("list events", "scope", "kubernetes", "err", err)
		return out
	}
	items := list.Items
	sort.SliceStable(items, func(i, j int) bool { return eventTime(items[i]).After(eventTime(items[j])) })
	if len(items) > maxEvents {
		items = items[:maxEvents]
	}
	for _, e := range items {
		src := e.Source.Component
		if src == "" {
			src = e.ReportingController
		}
		out = append(out, Event{
			Type: e.Type, Reason: e.Reason, Message: e.Message, Count: e.Count,
			First: rfc3339(e.FirstTimestamp.Time, e.EventTime.Time), Last: rfc3339(eventTime(e)), Source: src,
		})
	}
	return out
}

func eventTime(e corev1.Event) time.Time {
	for _, t := range []time.Time{e.LastTimestamp.Time, e.EventTime.Time, e.FirstTimestamp.Time, e.CreationTimestamp.Time} {
		if !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

func rfc3339(ts ...time.Time) string {
	for _, t := range ts {
		if !t.IsZero() {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return ""
}
