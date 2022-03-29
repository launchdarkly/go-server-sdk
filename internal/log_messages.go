package internal

import (
	"fmt"
	"os"
)

// This file contains helper functions for generating various kinds of standardized log warnings/errors.
// In some cases, these need to be written directly to os.Stderr instead of using our ldlog.Loggers API
// because they are for conditions where we don't have access to any configured SDK components.

// LogErrorNilPointerMethod prints a message to os.Stderr to indicate that the application tried to call
// a method on a nil pointer receiver.
func LogErrorNilPointerMethod(typeName string) {
	fmt.Fprintf(os.Stderr, "[LaunchDarkly] ERROR: tried to call a method on a nil pointer of type *%s", typeName)
}
