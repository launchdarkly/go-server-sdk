package sharedtest

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// RewriteFileInPlace rewrites an existing file the slow way: it truncates the file, waits for the
// pause, and then writes the new content, so a reader can find the file empty in between.
// Applications and editors rewrite data files in place, and the SDK must tolerate reading one
// while it is being written, so tests rewrite files this way rather than replacing them.
func RewriteFileInPlace(t testing.TB, path string, content string, pause time.Duration) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0) //nolint:gosec // G304: test file path
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, f.Sync())
	time.Sleep(pause)
	_, err = f.WriteString(content)
	require.NoError(t, err)
}
