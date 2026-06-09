package emit

import (
	"errors"
	"fmt"
)

// ErrUnboundEntity is returned when a device instance has no entity bound during
// elaborate (an upstream resolution error already exists); emit skips the
// instantiation.
var ErrUnboundEntity = errors.New("entity not bound")

// ErrDiffPair is a differential pin pair missing one of its pos/neg legs.
var ErrDiffPair = errors.New("emit: incomplete differential pin pair")

// EmitError reports a problem encountered while emitting VHDL for an instance.
type EmitError struct {
	Kind   error  // ErrUnboundEntity (the category sentinel)
	Inst   string // the instance label/name
	Detail string
}

func (e *EmitError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("emit %q: %v: %s", e.Inst, e.Kind, e.Detail)
	}
	return fmt.Sprintf("emit %q: %v", e.Inst, e.Kind)
}

func (e *EmitError) Unwrap() error { return e.Kind }
