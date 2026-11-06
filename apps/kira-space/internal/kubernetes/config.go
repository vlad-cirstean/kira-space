package kubernetes

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type loadedConfig struct {
	info  ConfigInfo
	cfg   *clientcmdapi.Config
	rules *clientcmd.ClientConfigLoadingRules
}

// kubeconfigPaths is KUBECONFIG's path list, else ~/.kube/config.
func kubeconfigPaths(env Env) []string {
	if env.Kubeconfig != "" {
		var out []string
		for _, p := range filepath.SplitList(env.Kubeconfig) {
			if p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home, ".kube", "config")}
}

// loadConfig reads the merged kubeconfig. It never fails: the outcome is in info.State.
func loadConfig(env Env) loadedConfig {
	paths := kubeconfigPaths(env)
	ld := loadedConfig{info: ConfigInfo{State: ConfigOK, Paths: paths}, cfg: clientcmdapi.NewConfig()}
	if paths == nil {
		ld.info.Paths = []string{}
	}
	existing := slices.DeleteFunc(slices.Clone(paths), func(p string) bool {
		_, err := os.Stat(p)
		return errors.Is(err, os.ErrNotExist)
	})
	if len(existing) == 0 {
		ld.info.State = ConfigMissing
		return ld
	}
	ld.rules = &clientcmd.ClientConfigLoadingRules{Precedence: existing}
	cfg, err := ld.rules.Load()
	if err != nil {
		ld.info.State = ConfigInvalid
		ld.info.Message = strings.TrimSpace(err.Error())
		return ld
	}
	ld.cfg = cfg
	return ld
}
