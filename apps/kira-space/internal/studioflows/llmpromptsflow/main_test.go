package llmpromptsflow_test

import (
	"os"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
)

func TestMain(m *testing.M) { os.Exit(studioflowharness.Main(m)) }
