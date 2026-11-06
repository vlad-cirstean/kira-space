package apifiles

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/postman"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// ErrSecretStoreOff marks a secret value that cannot be sealed because the secret store is unavailable.
var ErrSecretStoreOff = errors.New("apifiles: the secret store is not available")

// importVariables converts imported variables to file variables. A secret becomes a reference named
// after the owner stem and the variable, and its value goes to put when put is set: a collection
// import passes nil, so its secrets arrive unset. A repeated name keeps the first, as sends always did.
func importVariables(stem string, vars []postman.Variable, put func(name, value string)) []Variable {
	var out []Variable
	seen := map[string]bool{}
	secretNames := map[string]bool{}
	for _, v := range vars {
		if v.Name == "" || seen[v.Name] {
			continue
		}
		seen[v.Name] = true
		if !v.Secret {
			out = append(out, Variable{Name: v.Name, Value: v.Value, Description: v.Description})
			continue
		}
		name := freeName(SecretName(stem, v.Name), func(c string) bool { return secretNames[c] })
		secretNames[name] = true
		if put != nil && v.Value != "" {
			put(name, v.Value)
		}
		out = append(out, Variable{Name: v.Name, Secret: "${secret:" + name + "}", Description: v.Description})
	}
	return out
}

type treeNode struct {
	item Item
	kids []int
}

// itemsOf nests the flat Postman item list, giving each item a key unique in the file.
func itemsOf(tree *postman.Tree, stem string, s *Store) []Item {
	nodes := make([]treeNode, len(tree.Items))
	taken := map[string]bool{}
	var roots []int
	for i, src := range tree.Items {
		key := freeName(Slug(src.Name, "item"), func(c string) bool { return taken[c] })
		taken[key] = true
		it := Item{ID: key, Name: src.Name}
		if src.Kind == postman.KindFolder {
			it.Items = &[]Item{}
		} else {
			req := src.Request
			if req.Method == "" {
				req = model.SavedRequest{Method: defaultMethod, BodyMode: bodyModeNone, CodeLanguage: langJSON}
			}
			h := HTTPOf(req)
			it.HTTP = &h
		}
		if js, err := encodeJSON(src.Origin); err == nil && len(src.Origin) > 0 {
			s.setOrigin(stem, key, js)
		}
		nodes[i].item = it
		if src.Parent == postman.RootParent {
			roots = append(roots, i)
		} else {
			nodes[src.Parent].kids = append(nodes[src.Parent].kids, i)
		}
	}
	var build func(idx []int) []Item
	build = func(idx []int) []Item {
		out := make([]Item, 0, len(idx))
		for _, i := range idx {
			n := nodes[i]
			if n.item.Items != nil {
				kids := build(n.kids)
				n.item.Items = &kids
			}
			out = append(out, n.item)
		}
		return out
	}
	return build(roots)
}

func (s *Store) dropOrigins(stem string) {
	for k := range s.origins {
		if strings.HasPrefix(k, stem+"\x00") {
			delete(s.origins, k)
		}
	}
}

// ImportCollection writes a parsed Postman collection as a collection file and returns its stem.
// The Postman origins of the collection and its items are kept in the rows, for a faithful export.
func (s *Store) ImportCollection(tree *postman.Tree) (string, error) {
	s.mu.Lock()
	s.pend = &Changes{}
	stem, err := s.importCollectionLocked(tree)
	ch := *s.pend
	s.mu.Unlock()
	s.emit(ch)
	return stem, err
}

func (s *Store) importCollectionLocked(tree *postman.Tree) (string, error) {
	stem := s.freeStem(CollectionExt, Slug(tree.Name, "collection"))
	file := CollectionFile{Name: tree.Name, Variables: importVariables(stem, tree.Variables, nil), Items: itemsOf(tree, stem, s)}
	if js, err := encodeJSON(tree.Origin); err == nil {
		s.setOrigin(stem, "", js)
	}
	defer s.dropOrigins(stem)
	text, err := configfs.RenderYAML(file, "")
	if err != nil {
		return "", err
	}
	name := stem + CollectionExt
	if _, p := s.dec.collection(name, text); p != nil {
		return "", &InvalidError{Problem: p}
	}
	if _, err := s.collections.Save(name, "", text); err != nil {
		return "", err
	}
	return stem, nil
}

func (s *Store) freeStem(ext, base string) string {
	return freeName(base, func(c string) bool { return fileExists(s.o.Dir, c+ext) })
}

// ImportEnvironment writes a parsed Postman environment as an environment file and returns its stem.
// Secret values are sealed first, so a store that cannot seal leaves nothing written.
func (s *Store) ImportEnvironment(env *postman.Environment) (string, error) {
	s.mu.Lock()
	s.pend = &Changes{}
	stem, err := s.importEnvironmentLocked(env)
	ch := *s.pend
	s.mu.Unlock()
	s.emit(ch)
	return stem, err
}

func (s *Store) importEnvironmentLocked(env *postman.Environment) (string, error) {
	stem := s.freeStem(EnvironmentExt, Slug(env.Name, "environment"))
	puts := map[string]string{}
	vars := importVariables(stem, env.Variables, func(name, value string) { puts[name] = value })
	if len(puts) > 0 && !s.o.Secrets.Status().Available {
		return "", ErrSecretStoreOff
	}
	var written []string
	cleanup := func() {
		for _, n := range written {
			_ = s.o.Secrets.Delete(n)
		}
	}
	for name, value := range puts {
		if err := s.o.Secrets.Put(name, value); err != nil {
			cleanup()
			return "", fmt.Errorf("apifiles: seal secret: %w", err)
		}
		written = append(written, name)
	}
	text, err := configfs.RenderYAML(EnvironmentFile{Name: env.Name, Variables: vars}, "")
	if err == nil {
		name := stem + EnvironmentExt
		if _, p := s.dec.environment(name, text); p != nil {
			err = &InvalidError{Problem: p}
		} else {
			_, err = s.environments.Save(name, "", text)
		}
	}
	if err != nil {
		cleanup()
		return "", err
	}
	return stem, nil
}
