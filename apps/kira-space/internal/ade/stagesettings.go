package ade

import (
	"strconv"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// stageValues are the step's values for agent: the block under agent's id, keeping the keys the
// provider still declares with a valid value (an old snapshot may carry a removed key). Another
// provider's block is ignored: a workflow can be shared across providers.
func stageValues(settings map[string]map[string]string, agent harness.Provider) harness.StageValues {
	block := settings[agent.ID()]
	if len(block) == 0 {
		return nil
	}
	var out harness.StageValues
	for _, st := range agent.StageSettings() {
		if v, ok := st.Parse(block[st.Key]); ok {
			if out == nil {
				out = harness.StageValues{}
			}
			out[st.Key] = v
		}
	}
	return out
}

// stepSettingsOf are the settings of a step of the task's workflow; nil when it has none.
func stepSettingsOf(tc *taskCtx, stepID string) map[string]map[string]string {
	if tc.wf == nil {
		return nil
	}
	for _, s := range tc.wf.Steps {
		if s.ID == stepID {
			return s.Settings
		}
	}
	return nil
}

// stepAgentOf is the agent id of a step of the task's workflow; "" means the Agent setting.
func stepAgentOf(tc *taskCtx, stepID string) string {
	if tc.wf == nil {
		return ""
	}
	for _, s := range tc.wf.Steps {
		if s.ID == stepID {
			return s.Agent
		}
	}
	return ""
}

// stageCatalog lists every stage setting the registered providers take, with its copy rendered.
func stageCatalog() []adewire.StageSettingSpec {
	out := []adewire.StageSettingSpec{}
	for _, set := range providers.StageCatalog() {
		for _, st := range set.Settings {
			out = append(out, adewire.StageSettingSpec{
				Provider: set.ID, ProviderLabel: set.Label, Key: st.Key, Kind: string(st.Kind), Min: st.Min, Max: st.Max,
				Label: i18n.T(st.LabelKey, nil),
				Hint:  i18n.T(st.HintKey, map[string]string{"min": strconv.Itoa(st.Min), "max": strconv.Itoa(st.Max)}),
			})
		}
	}
	return out
}
