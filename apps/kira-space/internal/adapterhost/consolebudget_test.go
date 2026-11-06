package adapterhost

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// A batch that fills the shared console byte budget must still fit the response limit with room
// for per-cell offsets (see DefaultConsoleCap).
func TestConsoleBudgetFitsResponseLimit(t *testing.T) {
	const offsetSlack = 2 << 20
	if pageTooLarge(adapters.DefaultConsoleCap.Bytes + offsetSlack) {
		t.Fatalf("DefaultConsoleCap.Bytes %d leaves less than %d for encoding slack", adapters.DefaultConsoleCap.Bytes, offsetSlack)
	}
}
