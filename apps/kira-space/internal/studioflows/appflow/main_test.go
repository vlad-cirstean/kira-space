package appflow

import (
	"os"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
)

func TestMain(m *testing.M) { os.Exit(flowharness.Main(m)) }
