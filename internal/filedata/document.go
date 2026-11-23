// Package filedata contains the file reading, parsing, and merging logic shared by the
// components that load flag and segment data from local files.
package filedata

import (
	"bytes"
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
		return Document{}, wrapReadError(err)
	}
	return parseDocument(rawData)
}

func wrapReadError(err error) error {
	return fmt.Errorf("unable to read file: %s", err)
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

// parseFileContent parses a file's content like parseDocument, and also reports whether the
// content holds a document. YAML with no document, such as only a comment or "---", parses
// as a document with no entries, like an empty file. JSON content is always an object, so it
// always holds one, even "{}".
func parseFileContent(rawData []byte) (Document, bool, error) {
	doc, err := parseDocument(rawData)
	if err != nil || detectJSON(rawData) || doc.Flags != nil || doc.FlagValues != nil || doc.Segments != nil {
		return doc, true, err
	}
	asJSON, err := yaml.YAMLToJSON(rawData)
	if err != nil {
		// COVERAGE: unreachable, because parseDocument converted the same content
		return doc, true, nil
	}
	return doc, !bytes.Equal(bytes.TrimSpace(asJSON), []byte("null")), nil
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

// FlagValueExpander builds the full flag definition for a flag-key-to-value entry. Each file
// source supplies the form its consumers expect.
type FlagValueExpander func(key string, value ldvalue.Value) *ldmodel.FeatureFlag

// MakeOffFlagWithValue expands a flag-key-to-value entry into a flag that is off and serves the
// value as its off variation. The file data sources use this form.
func MakeOffFlagWithValue(key string, value ldvalue.Value) *ldmodel.FeatureFlag {
	flag := ldbuilders.NewFlagBuilder(key).SingleVariation(value).Build()
	return &flag
}

// MakeFallthroughFlagWithValue expands a flag-key-to-value entry into a flag that is on and
// serves the value as its only variation through the fallthrough. The override source uses
// this form.
func MakeFallthroughFlagWithValue(key string, value ldvalue.Value) *ldmodel.FeatureFlag {
	flag := ldbuilders.NewFlagBuilder(key).On(true).Variations(value).FallthroughVariation(0).Build()
	return &flag
}
