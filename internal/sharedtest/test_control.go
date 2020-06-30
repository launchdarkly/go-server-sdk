package sharedtest

import "os"

// ShouldSkipDatabaseTests returns true if the environment variable LD_SKIP_DATABASE_TESTS is non-empty.
func ShouldSkipDatabaseTests() bool {
	return os.Getenv("LD_SKIP_DATABASE_TESTS") != ""
}
