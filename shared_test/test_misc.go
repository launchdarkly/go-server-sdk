// Package shared_test contains types and functions used by SDK unit tests in multiple packages.
//
// Application code should not use this package. In a future version, it will be moved to internal.
package shared_test

import (
	"io/ioutil"
	"log"
	"os"
)

// WithTempFile creates a temporary file, passes its name to the given function, then ensures that the file is deleted.
func WithTempFile(f func(filename string)) {
	file, err := ioutil.TempFile("", "test")
	if err != nil {
		log.Fatalf("Can't create temp file: %s", err)
	}
	_ = file.Close()
	defer (func() {
		_ = os.Remove(file.Name())
	})()
	f(file.Name())
}
