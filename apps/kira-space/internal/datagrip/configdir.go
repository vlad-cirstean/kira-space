package datagrip

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// productPrefixByBuildCode maps F2's created-in build-code prefix (before the first '-') to the
// product's own config-directory folder-name prefix (D5.2a).
var productPrefixByBuildCode = map[string]string{
	"DB": "DataGrip", "IU": "IntelliJIdea", "IC": "IntelliJIdea", "PY": "PyCharm", "PC": "PyCharm",
	"WS": "WebStorm", "PS": "PhpStorm", "GO": "GoLand", "RM": "RubyMine", "CL": "CLion",
	"RD": "Rider", "DS": "DataSpell",
}

// ConfigCandidate is one ranked IDE config directory to try a credential lookup against (D5).
type ConfigCandidate struct {
	Dir     string
	Product string // the directory's own base name, e.g. "DataGrip2026.1"
}

// jetBrainsConfigRoot is F6's config-directory table, minus the `<version>` product folder itself
// — Windows has no case here (D1: out of scope).
func jetBrainsConfigRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "JetBrains"), nil
	}
	return filepath.Join(home, ".config", "JetBrains"), nil
}

// DiscoverConfigDirs enumerates and ranks the JetBrains product config directories under
// root, given createdIn (dataSources.local.xml's own build-code hint, "" when unknown). A
// directory counts as a candidate only when it looks like a real IDE config dir — contains
// c.kdbx or options/security.xml — so an unrelated folder under the same JetBrains/ parent (a
// plugin cache, a stale product with neither file) is never tried.
func DiscoverConfigDirs(root, createdIn string) ([]ConfigCandidate, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var candidates []ConfigCandidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if !looksLikeIDEConfigDir(dir) {
			continue
		}
		candidates = append(candidates, ConfigCandidate{Dir: dir, Product: e.Name()})
	}

	wantedProduct := productPrefixByBuildCode[productPrefixFromCreatedIn(createdIn)]
	sort.SliceStable(candidates, func(i, j int) bool {
		ri, rj := candidateRank(candidates[i].Product, wantedProduct), candidateRank(candidates[j].Product, wantedProduct)
		if ri != rj {
			return ri < rj
		}
		// D5.2: "within a group, descending by folder name" — descending by name is descending
		// by version for JetBrains' own <Product><YYYY>.<N> naming scheme.
		return candidates[i].Product > candidates[j].Product
	})
	return candidates, nil
}

// candidateRank is D5.2's three-group order: (a) the product named by created-in, (b) DataGrip
// itself (the row's own likeliest source even with no hint), (c) everything else.
func candidateRank(product, wantedProduct string) int {
	switch {
	case wantedProduct != "" && strings.HasPrefix(product, wantedProduct):
		return 0
	case strings.HasPrefix(product, "DataGrip"):
		return 1
	default:
		return 2
	}
}

func looksLikeIDEConfigDir(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "c.kdbx")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "options", "security.xml")); err == nil {
		return true
	}
	return false
}

// SecurityConfig is security.xml's PasswordSafe component — Provider is "" when the file is
// missing or unparseable, which is treated as "try the keychain" rather than an error. keepassDb
// (the file-store's own relocation option) is intentionally not carried here any more — this
// package no longer reads a file-based store at all, only
// Provider itself, to tell a KEEPASS-configured project apart from one this app can help.
type SecurityConfig struct {
	Provider string
}

type xmlSecurityDoc struct {
	Components []struct {
		Name    string `xml:"name,attr"`
		Options []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"option"`
	} `xml:"component"`
}

// ReadSecurityXML reads <configDir>/options/security.xml (F5/F6's own storage convention — never
// treated as load-bearing per §1.10.4: a missing or unparseable file returns the zero value, not
// an error).
func ReadSecurityXML(configDir string) SecurityConfig {
	data, err := os.ReadFile(filepath.Join(configDir, "options", "security.xml"))
	if err != nil {
		return SecurityConfig{}
	}
	var doc xmlSecurityDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return SecurityConfig{}
	}
	var cfg SecurityConfig
	for _, c := range doc.Components {
		if c.Name != "PasswordSafe" {
			continue
		}
		for _, o := range c.Options {
			if o.Name == "PROVIDER" {
				cfg.Provider = o.Value
			}
		}
	}
	return cfg
}

// The file-based PasswordSafe store (KEEPASS) was removed per explicit user correction, so
// "keychain" is the only backend this package ever tries. A KEEPASS-configured project is handled
// separately by its callers (apply.go's lookupPassword, scan.go's outlookFor), which check
// cfg.Provider directly so they can report ReasonCredentialStoreUnsupported /
// OutlookStoreUnsupported rather than conflating it with "DataGrip never saved a password"
// (MEMORY_ONLY/DO_NOT_STORE, which is what an empty return here means). provider is
// SecurityConfig.Provider ("" for absent/unparseable, treated the same as "KEYCHAIN").
func LookupOrder(provider string) []string {
	switch provider {
	case "KEEPASS", "MEMORY_ONLY", "DO_NOT_STORE":
		return nil
	default: // "KEYCHAIN", "", or anything unrecognised.
		return []string{"keychain"}
	}
}
