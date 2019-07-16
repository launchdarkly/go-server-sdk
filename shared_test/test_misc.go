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
