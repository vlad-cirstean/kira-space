package jsengine

import (
	"fmt"
	"time"
)

// Limit defaults, ceilings and fixed caps.
const (
	DefaultTimeout     = 2 * time.Second
	MaxTimeout         = 10 * time.Second
	DefaultMemoryBytes = 256 << 20
	MaxMemoryBytes     = 1 << 30
	DefaultInputBytes  = 4 << 20
	MaxInputBytes      = 32 << 20
	DefaultOutputBytes = 1 << 20
	MaxOutputBytes     = 8 << 20
	// ReadBackValueBytes caps one read-back value (a saved variable) and ReadBackMaxKeys the keys
	// read back across all read-back globals; both are independent of OutputBytes.
	ReadBackValueBytes = 1 << 20
	ReadBackMaxKeys    = 32
	MaxSourceBytes     = 64 << 10
	MaxJSONDepth       = 256
	MaxHostCalls       = 10000
	MaxConcurrent      = 4

	maxCallStack   = 10000
	maxGoStack     = 64 << 20
	killGrace      = time.Second
	helloTimeout   = 10 * time.Second
	memoryPerInput = 24 // goja heap bytes per input JSON byte, with headroom (about 22x measured)
	exitMemory     = 75 // worker hard-ceiling exit code
)

// Limits bound one run. A zero field takes its default; a field above its ceiling is a programming
// error (Exec returns a plain error, never clamps).
type Limits struct {
	Timeout     time.Duration `json:"timeout"`     // wall clock for the whole run, spawn included
	MemoryBytes int           `json:"memoryBytes"` // worker heap budget over its post-input baseline
	InputBytes  int           `json:"inputBytes"`  // sum of binding JSON; also per host call and kira.parseJSON text
	OutputBytes int           `json:"outputBytes"` // encoded result
}

func (l Limits) normalize() (Limits, error) {
	if l.Timeout == 0 {
		l.Timeout = DefaultTimeout
	}
	if l.MemoryBytes == 0 {
		l.MemoryBytes = DefaultMemoryBytes
	}
	if l.InputBytes == 0 {
		l.InputBytes = DefaultInputBytes
	}
	if l.OutputBytes == 0 {
		l.OutputBytes = DefaultOutputBytes
	}
	switch {
	case l.Timeout < 0 || l.Timeout > MaxTimeout:
		return l, fmt.Errorf("jsengine: Timeout %v outside (0, %v]", l.Timeout, MaxTimeout)
	case l.MemoryBytes < 0 || l.MemoryBytes > MaxMemoryBytes:
		return l, fmt.Errorf("jsengine: MemoryBytes %d outside (0, %d]", l.MemoryBytes, MaxMemoryBytes)
	case l.InputBytes < 0 || l.InputBytes > MaxInputBytes:
		return l, fmt.Errorf("jsengine: InputBytes %d outside (0, %d]", l.InputBytes, MaxInputBytes)
	case l.OutputBytes < 0 || l.OutputBytes > MaxOutputBytes:
		return l, fmt.Errorf("jsengine: OutputBytes %d outside (0, %d]", l.OutputBytes, MaxOutputBytes)
	}
	return l, nil
}

// callCap is the per-call allocation cap the guards enforce.
func (l Limits) callCap() int { return l.MemoryBytes / 4 }

// hardCeiling is the extra worker memory over the baseline at which the worker exits at once.
func (l Limits) hardCeiling() int { return l.MemoryBytes/4*5 + 64<<20 }
