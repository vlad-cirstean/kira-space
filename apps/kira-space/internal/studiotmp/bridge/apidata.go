package bridge

import (
	"sort"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/appcore"
)

// ApiDataChange names one renderer cache a mutation made stale (P112). Kind is one of the consts
// below; ItemID is set only for savedRequest, Scope/OwnerID only for variables.
type ApiDataChange struct {
	Kind    string              `json:"kind"`
	ItemID  string              `json:"itemId,omitempty"`
	Scope   model.VariableScope `json:"scope,omitempty"`
	OwnerID string              `json:"ownerId,omitempty"`
}

const (
	ApiDataTree         = "tree"         // collections + items list (names, method/url summary, order)
	ApiDataSavedRequest = "savedRequest" // one item's saved HTTP or gRPC body
	ApiDataVariables    = "variables"    // one owner's variable rows
	ApiDataEnvironments = "environments" // environment list, including is_active
	ApiDataConfig       = "config"       // the config files' status and the secret names
)

// ApiDataChanged is ChannelApiDataChanged's payload: every scope one mutation touched, in one
// event, so the renderer invalidates them in one batch.
type ApiDataChanged struct {
	Changes []ApiDataChange `json:"changes"`
}

func emitApiData(e appcore.Emitter, changes ...ApiDataChange) {
	e.Emit(ChannelApiDataChanged, ApiDataChanged{Changes: changes})
}

// EmitApiChanges pushes what a config file load or write made stale, as one event.
func EmitApiChanges(e appcore.Emitter, ch apifiles.Changes) {
	var out []ApiDataChange
	if ch.Tree {
		out = append(out, treeChange())
	}
	if ch.Environments {
		out = append(out, environmentsChange())
	}
	if ch.Config {
		out = append(out, configChange())
	}
	ids := make([]string, 0, len(ch.Requests))
	for id := range ch.Requests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		out = append(out, savedRequestChange(id))
	}
	keys := make([]apifiles.OwnerKey, 0, len(ch.Variables))
	for k := range ch.Variables {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Scope != keys[j].Scope {
			return keys[i].Scope < keys[j].Scope
		}
		return keys[i].ID < keys[j].ID
	})
	for _, k := range keys {
		out = append(out, variablesChange(model.VariableScope(k.Scope), k.ID))
	}
	if len(out) > 0 {
		emitApiData(e, out...)
	}
}

func treeChange() ApiDataChange { return ApiDataChange{Kind: ApiDataTree} }

func savedRequestChange(itemID string) ApiDataChange {
	return ApiDataChange{Kind: ApiDataSavedRequest, ItemID: itemID}
}

func variablesChange(scope model.VariableScope, ownerID string) ApiDataChange {
	return ApiDataChange{Kind: ApiDataVariables, Scope: scope, OwnerID: ownerID}
}

func environmentsChange() ApiDataChange { return ApiDataChange{Kind: ApiDataEnvironments} }

func configChange() ApiDataChange { return ApiDataChange{Kind: ApiDataConfig} }
