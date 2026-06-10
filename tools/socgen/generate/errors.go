package generate

import (
	"errors"
	"fmt"
)

// Sentinels for GenerateError.Kind.
var (
	// ErrUnknownPlugin is a plugin name not in the known set
	// (device_tree, board.h, aic1).
	ErrUnknownPlugin = errors.New("unknown plugin")
	// ErrDuplicatePlugin is a plugin name listed more than once.
	ErrDuplicatePlugin = errors.New("duplicate plugin")
	// ErrEmit wraps a failure from an underlying emitter.
	ErrEmit = errors.New("emit failed")
	// ErrEmptyContent is a core file whose emitter returned empty content
	// without an error (faithful to the Clojure "Failed to generate" warning).
	ErrEmptyContent = errors.New("empty content")
)

// GenerateError reports a problem encountered while orchestrating generation.
type GenerateError struct {
	Kind   error  // category sentinel
	Name   string // the plugin or file name involved
	Detail string
}

func (e *GenerateError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("generate %q: %v: %s", e.Name, e.Kind, e.Detail)
	}
	return fmt.Sprintf("generate %q: %v", e.Name, e.Kind)
}

func (e *GenerateError) Unwrap() error { return e.Kind }
