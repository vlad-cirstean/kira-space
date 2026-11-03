package kubernetes

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

const podsKey = "v1/pods"

var categoryOrder = []string{"workloads", "network", "config", "storage", "access", "cluster", "custom"}

var categoryOf = map[string]string{
	"pods": "workloads", "deployments": "workloads", "replicasets": "workloads", "statefulsets": "workloads",
	"daemonsets": "workloads", "jobs": "workloads", "cronjobs": "workloads",
	"services": "network", "endpointslices": "network", "ingresses": "network", "ingressclasses": "network",
	"networkpolicies": "network",
	"configmaps":      "config", "secrets": "config", "resourcequotas": "config", "limitranges": "config",
	"horizontalpodautoscalers": "config", "poddisruptionbudgets": "config",
	"persistentvolumeclaims": "storage", "persistentvolumes": "storage", "storageclasses": "storage",
	"serviceaccounts": "access", "roles": "access", "rolebindings": "access", "clusterroles": "access",
	"clusterrolebindings": "access",
	"namespaces":          "cluster", "nodes": "cluster", "events": "cluster", "priorityclasses": "cluster",
	"runtimeclasses": "cluster", "customresourcedefinitions": "cluster",
}

func builtinGroup(g string) bool {
	return g == "" || !strings.Contains(g, ".") || strings.HasSuffix(g, ".k8s.io")
}

func categorize(group, resource string) string {
	if builtinGroup(group) {
		if c, ok := categoryOf[resource]; ok {
			return c
		}
	}
	return "custom"
}

func kindKey(group, version, resource string) string {
	if group == "" {
		return version + "/" + resource
	}
	return group + "/" + version + "/" + resource
}

// Namespaces lists namespace names. A forbidden list is not an error: the UI falls back to the
// context's own namespace.
func (s *Service) Namespaces(ctx context.Context, args ClusterArgs) (NamespacesResult, error) {
	var res NamespacesResult
	err := s.record(ctx, args.ClusterID, args.Background, NamespacesCommand, func(ctx context.Context, note *OpNote) error {
		var err error
		if res, err = s.namespaces(ctx, args); err == nil {
			note.SetRows(len(res.Namespaces))
		}
		return err
	})
	if err != nil {
		return NamespacesResult{}, err
	}
	return res, nil
}

func (s *Service) namespaces(ctx context.Context, args ClusterArgs) (NamespacesResult, error) {
	cs, err := s.client(args.ClusterID)
	if err != nil {
		return NamespacesResult{}, mapErr(err)
	}
	fallback := "default"
	if _, _, c, err := s.target(args.ClusterID); err == nil && c.Namespace != "" {
		fallback = c.Namespace
	}
	res := NamespacesResult{Namespaces: []string{}, Fallback: fallback}
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	opts := metav1.ListOptions{Limit: 500}
	for {
		list, err := cs.core.Namespaces().List(cctx, opts)
		if err != nil {
			if apierrors.IsForbidden(err) {
				return NamespacesResult{Namespaces: []string{}, Restricted: true, Fallback: fallback}, nil
			}
			return NamespacesResult{}, mapErr(err)
		}
		for _, n := range list.Items {
			res.Namespaces = append(res.Namespaces, n.Name)
		}
		if list.Continue == "" || len(res.Namespaces) >= s.listMax {
			break
		}
		opts.Continue = list.Continue
	}
	sort.Strings(res.Namespaces)
	return res, nil
}

// Kinds lists the listable resource types through discovery.
func (s *Service) Kinds(ctx context.Context, args ClusterArgs) (KindsResult, error) {
	var res KindsResult
	err := s.record(ctx, args.ClusterID, args.Background, KindsCommand, func(ctx context.Context, note *OpNote) error {
		cs, err := s.client(args.ClusterID)
		if err != nil {
			return mapErr(err)
		}
		if res, err = s.kindsOf(ctx, cs); err != nil {
			return err
		}
		note.SetRows(len(res.Kinds))
		return nil
	})
	if err != nil {
		return KindsResult{}, err
	}
	return res, nil
}

func (s *Service) kindsOf(ctx context.Context, cs *clusterClient) (KindsResult, error) {
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	lists, err := cs.disc.ServerPreferredResourcesWithContext(cctx)
	res := KindsResult{Kinds: []Kind{}}
	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) || len(lists) == 0 {
			return KindsResult{}, mapErr(err)
		}
		res.Partial = true
	}
	for _, l := range lists {
		gv, perr := schema.ParseGroupVersion(l.GroupVersion)
		if perr != nil {
			continue
		}
		for _, r := range l.APIResources {
			if strings.Contains(r.Name, "/") || !slices.Contains(r.Verbs, "list") || !slices.Contains(r.Verbs, "get") {
				continue
			}
			key := kindKey(gv.Group, gv.Version, r.Name)
			res.Kinds = append(res.Kinds, Kind{
				Key: key, Group: gv.Group, Version: gv.Version, Resource: r.Name, Kind: r.Kind,
				Namespaced: r.Namespaced, Category: categorize(gv.Group, r.Name), Logs: key == podsKey,
			})
		}
	}
	sort.SliceStable(res.Kinds, func(i, j int) bool {
		a, b := res.Kinds[i], res.Kinds[j]
		if ca, cb := slices.Index(categoryOrder, a.Category), slices.Index(categoryOrder, b.Category); ca != cb {
			return ca < cb
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Key < b.Key
	})
	return res, nil
}

// findKind resolves a kind key against discovery.
func (s *Service) findKind(ctx context.Context, cs *clusterClient, key string) (Kind, error) {
	if key == "" {
		return Kind{}, ipcerr.Required("kind")
	}
	res, err := s.kindsOf(ctx, cs)
	if err != nil {
		return Kind{}, err
	}
	for _, k := range res.Kinds {
		if k.Key == key {
			return k, nil
		}
	}
	return Kind{}, ipcerr.T(ipcerr.CodeNotFound, "backend.studio.kubernetes.notFound.error", map[string]string{"message": key})
}
