package helpers

import (
	"io"
	"log"
)

// WithCloser executes a function and ensures that the given object's Close() method is always called afterward.
//
// This is simply a way to get more specific control over an object's lifetime than using defer. A test function
// may wish to ensure that an object is closed before some subsequent actions are taken, rather than at the end
// of the entire test.
//
// If closing the object fails, an error is logged.
func WithCloser(closeableObject io.Closer, action func()) {
	defer func() {
		err := closeableObject.Close()
		if err != nil {
			log.Printf("failed to close %T: %s", closeableObject, err)
		}
	}()
	action()
}
