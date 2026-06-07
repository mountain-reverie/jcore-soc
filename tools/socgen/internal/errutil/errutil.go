// Package errutil holds helpers for working with aggregated (errors.Join) errors.
package errutil

// Errors flattens an errors.Join result into its leaf component errors: the parts
// of a joined error (recursively, so a join-of-joins flattens fully), a one-element
// slice for a non-joined error, or nil for nil. Use it to iterate or count the
// errors a best-effort pass produced — robust to the nested joins that arise when
// a function joins sub-results that are themselves joins.
func Errors(err error) []error {
	if err == nil {
		return nil
	}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		var out []error
		for _, e := range j.Unwrap() {
			out = append(out, Errors(e)...)
		}
		return out
	}
	return []error{err}
}
