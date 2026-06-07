package board

import (
	"errors"
	"fmt"
)

var (
	ErrMake      = errors.New("make vhdl_list failed")
	ErrReadList  = errors.New("read file list failed")
	ErrEmptyList = errors.New("empty file list")
	ErrReadFile  = errors.New("read source failed")
)

// FileListError reports a failure obtaining or reading the board's VHDL file list.
type FileListError struct {
	Kind   error  // ErrMake | ErrReadList | ErrEmptyList | ErrReadFile
	Target string // board/make target or file path
	Err    error
}

func (e *FileListError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%v (%s): %v", e.Kind, e.Target, e.Err)
	}
	return fmt.Sprintf("%v (%s)", e.Kind, e.Target)
}

// Unwrap exposes both the category sentinel and the underlying cause so errors.Is
// reaches the Kind AND the wrapped error (e.g. fs.ErrNotExist).
func (e *FileListError) Unwrap() []error {
	if e.Err != nil {
		return []error{e.Kind, e.Err}
	}
	return []error{e.Kind}
}
