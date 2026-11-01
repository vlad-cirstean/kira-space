package scriptruns

import (
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// maskOutcome hides secret values in every free-text field of an outcome before it is stored or pushed.
func maskOutcome(o runoutcome.Outcome, m *secrets.Masker) runoutcome.Outcome {
	o.Reason = m.Mask(o.Reason)
	o.LastError = m.Mask(o.LastError)
	o.OutputTail = m.Mask(o.OutputTail)
	o.Summary = m.Mask(o.Summary)
	return o
}
