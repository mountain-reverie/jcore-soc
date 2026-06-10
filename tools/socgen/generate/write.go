package generate

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write writes each file's content into dir, creating dir (and parents) if
// needed. It is faithful to the Clojure generate-design: dir must be a directory
// (or creatable); existing files are overwritten in place (no recursive delete).
func Write(files []File, dir string) error {
	info, err := os.Stat(dir)
	switch {
	case err == nil && !info.IsDir():
		return &GenerateError{Kind: ErrEmit, Name: dir, Detail: "output path exists and is not a directory"}
	case os.IsNotExist(err):
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return &GenerateError{Kind: ErrEmit, Name: dir, Detail: mkErr.Error()}
		}
	case err != nil:
		return &GenerateError{Kind: ErrEmit, Name: dir, Detail: err.Error()}
	}
	for _, f := range files {
		path := filepath.Join(dir, f.Name)
		fmt.Println("Writing", f.Name) // faithful to Clojure `println "Writing" name`
		if err := os.WriteFile(path, []byte(f.Content), 0o644); err != nil {
			return &GenerateError{Kind: ErrEmit, Name: f.Name, Detail: err.Error()}
		}
	}
	return nil
}
