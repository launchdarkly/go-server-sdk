package helpers

import (
	"fmt"
	"io/ioutil"
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
//     helpers.WithTempFile(func(path string) {
//         DoSomethingWithTempFile(path)
//     }) // the file is deleted at the end of this block
func WithTempFile(f func(string)) {
	file, err := ioutil.TempFile("", "test")
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
