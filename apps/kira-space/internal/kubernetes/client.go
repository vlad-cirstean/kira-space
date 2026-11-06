package kubernetes

import (
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/sync/singleflight"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	clientQPS   = 50
	clientBurst = 100
)

// clusterClient is the per-cluster client set, rebuilt when the kubeconfig entry changes.
type clusterClient struct {
	sig  string
	rest rest.Interface
	disc discovery.CachedDiscoveryInterfaceWithContext
	core corev1client.CoreV1Interface
}

type clientCache struct {
	sf singleflight.Group
}

// signature identifies the kubeconfig parts a client depends on.
func signature(cfg *clientcmdapi.Config, ctx *clientcmdapi.Context) string {
	b, err := json.Marshal(struct {
		Context  *clientcmdapi.Context
		Cluster  *clientcmdapi.Cluster
		AuthInfo *clientcmdapi.AuthInfo
	}{ctx, cfg.Clusters[ctx.Cluster], cfg.AuthInfos[ctx.AuthInfo]})
	if err != nil {
		return fmt.Sprintf("%p", ctx)
	}
	return string(b)
}

// pinExecPlugins points relative exec plugin commands at the resolved PATH and hands the plugin that
// PATH. Go resolves a bare command with the process PATH, which a Finder-launched app lacks.
func pinExecPlugins(cfg *clientcmdapi.Config, authInfo, path string) {
	ai := cfg.AuthInfos[authInfo]
	if ai == nil || ai.Exec == nil || path == "" {
		return
	}
	ex := ai.Exec
	if !strings.ContainsRune(ex.Command, '/') {
		if abs := resolveCommand(ex.Command, path); abs != "" {
			ex.Command = abs
		}
	}
	for i := range ex.Env {
		if ex.Env[i].Name == "PATH" {
			ex.Env[i].Value = path
			return
		}
	}
	ex.Env = append(ex.Env, clientcmdapi.ExecEnvVar{Name: "PATH", Value: path})
}

func buildClient(ld loadedConfig, ctxName, path, sig string) (*clusterClient, error) {
	cfg := ld.cfg.DeepCopy()
	ctx := cfg.Contexts[ctxName]
	pinExecPlugins(cfg, ctx.AuthInfo, path)
	var access clientcmd.ConfigAccess
	if ld.rules != nil {
		access = ld.rules
	}
	rc, err := clientcmd.NewNonInteractiveClientConfig(*cfg, ctxName, &clientcmd.ConfigOverrides{}, access).ClientConfig()
	if err != nil {
		return nil, err
	}
	rc.QPS, rc.Burst, rc.Timeout = clientQPS, clientBurst, 0
	rc.WarningHandler = rest.NoWarnings{}
	dc, err := discovery.NewDiscoveryClientForConfig(rc)
	if err != nil {
		return nil, err
	}
	core, err := corev1client.NewForConfig(rc)
	if err != nil {
		return nil, err
	}
	return &clusterClient{sig: sig, rest: dc.RESTClient(), disc: memory.NewMemCacheClientWithContext(dc), core: core}, nil
}
