package mysql

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/mysqlfamily"
)

func init() {
	adapters.Register("mysql", func(deps adapters.Deps) (adapters.Adapter, error) {
		profile := mysqlfamily.Profile{Kind: "mysql", ServerLabel: "MySQL", ApplyEngineOptions: applyEngineOptions}
		return mysqlfamily.New(deps, profile, caps), nil
	})
}
