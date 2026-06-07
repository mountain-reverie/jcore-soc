package design

import (
	"errors"
	"fmt"
)

// LoadError wraps an IO/decode failure with the file path.
type LoadError struct {
	Path string
	Err  error
}

func (e *LoadError) Error() string { return fmt.Sprintf("%s: %v", e.Path, e.Err) }
func (e *LoadError) Unwrap() error { return e.Err }

// SpecError is a malformed-spec error tied to a YAML line.
type SpecError struct {
	Line int
	Msg  string
	Err  error // optional underlying decode error
}

func (e *SpecError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("line %d: %s: %v", e.Line, e.Msg, e.Err)
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}
func (e *SpecError) Unwrap() error { return e.Err }

var (
	ErrIncludeCycle = errors.New("include cycle")
	ErrIncludeDepth = errors.New("include depth exceeded")
	ErrIncludeRead  = errors.New("include read failed")
)

// IncludeError reports an !include resolution failure.
type IncludeError struct {
	Kind error // ErrIncludeCycle | ErrIncludeDepth | ErrIncludeRead
	Path string
	Err  error
}

func (e *IncludeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%v: %s: %v", e.Kind, e.Path, e.Err)
	}
	return fmt.Sprintf("%v: %s", e.Kind, e.Path)
}
func (e *IncludeError) Unwrap() error { return e.Kind }

var (
	ErrEntityNotFound     = errors.New("entity not found")
	ErrConfigNotFound     = errors.New("configuration not found")
	ErrArchNotFound       = errors.New("configuration architecture not found")
	ErrGenericNotOnEntity = errors.New("generic not on entity")
	ErrPortNotOnEntity    = errors.New("port not on entity")
	ErrUnknownClass       = errors.New("unknown class")
)

// ValidateError reports a design-vs-interface validation failure.
type ValidateError struct {
	Kind   error
	Ctx    string // e.g. `device "uart0"` / `top-entity "cpus"`
	Name   string // the offending entity/config/generic/port/class name
	Entity string // related entity, where applicable
}

func (e *ValidateError) Error() string {
	switch {
	case errors.Is(e.Kind, ErrEntityNotFound):
		return fmt.Sprintf("%s: entity %q not found", e.Ctx, e.Name)
	case errors.Is(e.Kind, ErrConfigNotFound):
		return fmt.Sprintf("%s: configuration %q not found", e.Ctx, e.Name)
	case errors.Is(e.Kind, ErrArchNotFound):
		return fmt.Sprintf("%s: configuration %q architecture not found for entity %q", e.Ctx, e.Name, e.Entity)
	case errors.Is(e.Kind, ErrGenericNotOnEntity):
		return fmt.Sprintf("%s: generic %q not on entity %q", e.Ctx, e.Name, e.Entity)
	case errors.Is(e.Kind, ErrPortNotOnEntity):
		return fmt.Sprintf("%s: port %q not on entity %q", e.Ctx, e.Name, e.Entity)
	case errors.Is(e.Kind, ErrUnknownClass):
		return fmt.Sprintf("%s: unknown class %q", e.Ctx, e.Name)
	default:
		return fmt.Sprintf("%s: %v %q", e.Ctx, e.Kind, e.Name)
	}
}
func (e *ValidateError) Unwrap() error { return e.Kind }

// PinFileError reports a .pins file read/parse failure.
type PinFileError struct {
	Path string
	Part string // set for the "part not found" case
	Err  error
}

func (e *PinFileError) Error() string {
	if e.Part != "" {
		return fmt.Sprintf("pin-list %s: part %q not found", e.Path, e.Part)
	}
	return fmt.Sprintf("read pins %s: %v", e.Path, e.Err)
}
func (e *PinFileError) Unwrap() error { return e.Err }
