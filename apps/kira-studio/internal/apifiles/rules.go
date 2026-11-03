package apifiles

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/palette"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

//go:embed schema/collection.schema.json
var collectionSchema []byte

//go:embed schema/environment.schema.json
var environmentSchema []byte

const (
	ruleItemKind    = "An item has exactly one of items (a folder), http and grpc."
	ruleItemUnique  = "Item ids are unique in the file."
	ruleVarUnique   = "Variable names are unique in the file."
	ruleVarKind     = "A variable has a value or a secret, not both."
	ruleStemCollect = "The file name without .collection.yaml is the collection id: lowercase letters, digits and -, starting with a letter or digit, up to 64 characters. A file with another name is listed but not used."
	ruleStemEnv     = "The file name without .environment.yaml is the environment id: lowercase letters, digits and -, starting with a letter or digit, up to 64 characters. A file with another name is listed but not used."
)

func problem(path, key string, vars map[string]string) *configfs.Problem {
	return &configfs.Problem{Path: path, Message: i18n.T(key, vars)}
}

func itemKindRule() configfs.Rule[CollectionFile] {
	return configfs.Rule[CollectionFile]{Doc: ruleItemKind, Check: func(f CollectionFile, _ configfs.Locator) *configfs.Problem {
		var walk func(items []Item, path string) *configfs.Problem
		walk = func(items []Item, path string) *configfs.Problem {
			for i, it := range items {
				at := path + "/" + strconv.Itoa(i)
				n := 0
				if it.Items != nil {
					n++
				}
				if it.HTTP != nil {
					n++
				}
				if it.GRPC != nil {
					n++
				}
				if n != 1 {
					return problem(at, "backend.studio.apiFiles.itemKind.body", nil)
				}
				if it.Items != nil {
					if p := walk(*it.Items, at+"/items"); p != nil {
						return p
					}
				}
			}
			return nil
		}
		return walk(f.Items, "/items")
	}}
}

func itemUniqueRule() configfs.Rule[CollectionFile] {
	return configfs.Rule[CollectionFile]{Doc: ruleItemUnique, Check: func(f CollectionFile, _ configfs.Locator) *configfs.Problem {
		seen := map[string]bool{}
		var walk func(items []Item, path string) *configfs.Problem
		walk = func(items []Item, path string) *configfs.Problem {
			for i, it := range items {
				at := path + "/" + strconv.Itoa(i)
				if seen[it.ID] {
					return problem(at+"/id", "backend.studio.apiFiles.itemDuplicate.body", map[string]string{"id": it.ID})
				}
				seen[it.ID] = true
				if it.Items != nil {
					if p := walk(*it.Items, at+"/items"); p != nil {
						return p
					}
				}
			}
			return nil
		}
		return walk(f.Items, "/items")
	}}
}

func varUniqueRule[T any](vars func(T) []Variable) configfs.Rule[T] {
	return configfs.Rule[T]{Doc: ruleVarUnique, Check: func(f T, _ configfs.Locator) *configfs.Problem {
		seen := map[string]bool{}
		for i, v := range vars(f) {
			if seen[v.Name] {
				return problem(fmt.Sprintf("/variables/%d/name", i), "backend.studio.apiFiles.variableDuplicate.body", map[string]string{"name": v.Name})
			}
			seen[v.Name] = true
		}
		return nil
	}}
}

func varKindRule[T any](vars func(T) []Variable) configfs.Rule[T] {
	return configfs.Rule[T]{Doc: ruleVarKind, Check: func(f T, _ configfs.Locator) *configfs.Problem {
		for i, v := range vars(f) {
			if v.Value != "" && v.Secret != "" {
				return problem(fmt.Sprintf("/variables/%d/secret", i), "backend.studio.apiFiles.variableBoth.body", map[string]string{"name": v.Name})
			}
		}
		return nil
	}}
}

func collectionVars(f CollectionFile) []Variable   { return f.Variables }
func environmentVars(f EnvironmentFile) []Variable { return f.Variables }

// withStem refuses a file whose name is not a valid id, before the content is read.
func withStem[T any](ext string, decode configfs.DecodeFunc[T]) configfs.DecodeFunc[T] {
	return func(file string, src []byte) (T, *configfs.Problem) {
		if _, ok := StemOf(file, ext); !ok {
			var zero T
			return zero, &configfs.Problem{Message: i18n.T("backend.studio.apiFiles.fileName.body", nil)}
		}
		return decode(file, src)
	}
}

// Decoders holds the two file decoders and their AGENTS.md models.
type decoders struct {
	collection    configfs.DecodeFunc[CollectionFile]
	environment   configfs.DecodeFunc[EnvironmentFile]
	collectionDoc configfs.DocModel
	environmentDoc configfs.DocModel
}

func newDecoders() (decoders, error) {
	var d decoders
	collection, cdoc, err := configfs.YAMLSchema[CollectionFile](collectionSchema,
		itemKindRule(), itemUniqueRule(), varUniqueRule(collectionVars), varKindRule(collectionVars))
	if err != nil {
		return d, err
	}
	envSchema, err := assembleEnvironmentSchema()
	if err != nil {
		return d, err
	}
	environment, edoc, err := configfs.YAMLSchema[EnvironmentFile](envSchema,
		varUniqueRule(environmentVars), varKindRule(environmentVars))
	if err != nil {
		return d, err
	}
	cdoc.Rules = append(cdoc.Rules, ruleStemCollect)
	edoc.Rules = append(edoc.Rules, ruleStemEnv)
	d.collection, d.environment = withStem(CollectionExt, collection), withStem(EnvironmentExt, environment)
	d.collectionDoc, d.environmentDoc = cdoc, edoc
	return d, nil
}

// assembleEnvironmentSchema writes the palette into the schema, so the schema and the code cannot drift.
func assembleEnvironmentSchema() ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(environmentSchema, &root); err != nil {
		return nil, fmt.Errorf("apifiles: schema: %w", err)
	}
	names := palette.Names()
	enum := make([]any, len(names))
	for i, n := range names {
		enum[i] = n
	}
	root["properties"].(map[string]any)["color"].(map[string]any)["enum"] = enum
	return json.Marshal(root)
}
