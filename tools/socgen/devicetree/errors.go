package devicetree

import (
	"errors"
	"fmt"
)

// Sentinels for DTError.Kind.
var (
	ErrCPUFreq = errors.New("cannot determine cpu frequency (CFG_CLK_CPU_PERIOD_NS)")
	ErrPitTrap = errors.New("cannot determine unused pit trap")
	ErrIPI     = errors.New("cannot determine ipi info")
)

// DTError is a device-tree generation error.
type DTError struct {
	Kind   error
	Detail string
}

func (e *DTError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("device tree: %v: %s", e.Kind, e.Detail)
	}
	return fmt.Sprintf("device tree: %v", e.Kind)
}

func (e *DTError) Unwrap() error { return e.Kind }
