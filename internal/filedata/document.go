// Package filedata contains the file reading, parsing, and merging logic shared by the
// components that load flag and segment data from local files.
package filedata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"

	"gopkg.in/ghodss/yaml.v1"
)

// Document is the parsed form of a single data file. A document may contain full flag
// definitions, simplified flag-key-to-value entries, and segment definitions.
type Document struct {
	Flags      *map[string]ldmodel.FeatureFlag
	FlagValues *map[string]ldvalue.Value
	Segments   *map[string]ldmodel.Segment
}

// ReadError indicates that one of the source files could not be read or parsed. It
// distinguishes a per-file failure from a failure to merge the files' contents.
type ReadError struct {
	Err  error
	Path string
}

func (e *ReadError) Error() string {
	return fmt.Sprintf("%s [%s]", e.Err, e.Path)
}

func (e *ReadError) Unwrap() error {
	return e.Err
}

// ReadFile reads and parses a single data file, which may be in JSON or YAML format.
func ReadFile(path string) (Document, error) {
	rawData, err := os.ReadFile(path) //nolint:gosec // G304: ok to read file into variable
	if err != nil {
		return Document{}, fmt.Errorf("unable to read file: %s", err)
	}
	return parseDocument(rawData)
}

func parseDocument(rawData []byte) (Document, error) {
	var data Document
	var err error
	if detectJSON(rawData) {
		err = json.Unmarshal(rawData, &data)
	} else {
		err = yaml.Unmarshal(rawData, &data)
	}
	if err != nil {
		err = fmt.Errorf("error parsing file: %s", err)
	}
	return data, err
}

func detectJSON(rawData []byte) bool {
	// A valid JSON file for our purposes must be an object, i.e. it must start with '{'
	return strings.HasPrefix(strings.TrimLeftFunc(string(rawData), unicode.IsSpace), "{")
}

// AbsFilePaths converts each of the given paths to an absolute path.
func AbsFilePaths(paths []string) ([]string, error) {
	absPaths := make([]string, 0)
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			// COVERAGE: there's no reliable cross-platform way to simulate an invalid path in unit tests
			return nil, fmt.Errorf("unable to determine absolute path for '%s'", p)
		}
		absPaths = append(absPaths, absPath)
	}
	return absPaths, nil
}

// MakeFlagWithValue expands a flag-key-to-value entry into a full flag definition that
// returns the given value for every context.
func MakeFlagWithValue(key string, v interface{}) *ldmodel.FeatureFlag {
	flag := ldbuilders.NewFlagBuilder(key).SingleVariation(ldvalue.CopyArbitraryValue(v)).Build()
	return &flag
}
