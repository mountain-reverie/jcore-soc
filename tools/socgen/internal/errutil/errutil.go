// Package errutil holds helpers for working with aggregated (errors.Join) errors.
package errutil

// Errors flattens an errors.Join result into its component errors: the parts of a
// joined error, a one-element slice for a non-joined error, or nil for nil. Use it
// to iterate or count the errors a best-effort pass produced.
func Errors(err error) []error {
	if err == nil {
		return nil
	}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		return j.Unwrap()
	}
	return []error{err}
}
