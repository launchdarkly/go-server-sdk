package ldtest

import (
	"encoding/json"
	"fmt"
	"github.com/launchdarkly/go-client"
	"io/ioutil"
	"path/filepath"
	"strings"
)

type Scenario struct {
	Name         string                 `json:"name"`
	FeatureKey   string                 `json:"featureKey"`
	DefaultValue string                 `json:"defaultValue"`
	ValueType    string 				`json:"valueType"`
	TestCases    []TestCase             `json:"testCases"`
	FeatureFlags []ldclient.FeatureFlag `json:"featureFlags"`
}

type TestCase struct {
	ExpectedValue string        `json:"expectedValue"`
	ExpectError   bool          `json:"expectError"`
	User          ldclient.User `json:"user"`
}

func LoadTestDataFile(filePath string) ([]Scenario, error) {
	//t.Logf("Loading test data from file: %s", filePath)
	//_, fileName := filepath.Split(filePath)

	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var container []Scenario
	err = json.Unmarshal(file, &container)
	if err != nil {
		//t.Fatalf("FATAL: Error unmarshalling json from: %s; %v", filePath, err)
		return nil, err
	}

	if len(container) == 0 {
		return nil, fmt.Errorf("FATAL: Found zero Feature Flags to evaluate")
	}
	//t.Logf("[%s]\tFound %d Feature Flags to evaluate:", fileName, len(container))
	return container, nil
}

func ReadTestDataDir(baseDir string) ([]string, error) {
	inputDir, err := filepath.Abs(baseDir)
	if err != nil {
		//t.Fatalf("Could not get absolute path from: %s; %+v", baseDir, err)
		return nil, err
	}
	//t.Logf("Using base directory for test data: %v", inputDir)
	fileInfos, err := ioutil.ReadDir(inputDir)

	if err != nil {
		return nil, err
		//t.Fatalf("FATAL: Error reading %s test data directory: %v", baseDir, err)
	}
	filePaths := make([]string, 0, len(fileInfos))
	for _, fileInfo := range fileInfos {
		if strings.HasSuffix(fileInfo.Name(), ".json") {
			filePaths = append(filePaths, filepath.Join(inputDir, fileInfo.Name()))
		}
	}
	return filePaths, nil
}
