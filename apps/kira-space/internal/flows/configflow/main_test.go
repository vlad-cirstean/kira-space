package configflow_test

import (
	"os"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

func TestMain(m *testing.M) { os.Exit(flowharness.Main(m)) }
