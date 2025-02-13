package helpers

import (
	"fmt"
	"log"
	"os"
)

// FilePathExists is simply a shortcut for using os.Stat to check for a file's or directory's existence.
func FilePathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// WithTempFile creates a temporary file, passes its name to the given function, then ensures that the file is deleted.
//
// If for any reason it is not possible to create the file, a panic is raised since the test code cannot continue.
//
// If deletion of the file fails (assuming it has not already been deleted) then an error is logged, but there is no
// panic.
//
//	helpers.WithTempFile(func(path string) {
//		DoSomethingWithTempFile(path)
//	}) // the file is deleted at the end of this block
func WithTempFile(f func(filePath string)) {
	file, err := os.CreateTemp("", "test")
	if err != nil {
		panic(fmt.Errorf("can't create temp file: %s", err))
	}
	_ = file.Close()
	path := file.Name()
	defer (func() {
		if FilePathExists(path) {
			err := os.Remove(path)
			if err != nil {
				log.Printf("Could not delete temp file %s: %s", path, err)
			}
		}
	})()
	f(file.Name())
}

// WithTempFileData is identical to WithTempFile except that it prepopulates the file with the
// specified data.
func WithTempFileData(data []byte, f func(filePath string)) {
	WithTempFile(func(filePath string) {
		if err := os.WriteFile(filePath, data, 0600); err != nil {
			panic(fmt.Errorf("can't write to temp file: %s", err))
		}
		f(filePath)
	})
}

// WithTempDir creates a temporary directory, calls the function with its path, then removes it.
func WithTempDir(f func(path string)) {
	path, err := os.MkdirTemp("", "test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(path) //nolint:errcheck
	f(path)
}
