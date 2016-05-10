package ldtest

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
)

func LoadTestDataFiles(basePath string) (map[string][]Scenario, error) {
	filePaths, err := readTestDataDir(basePath)
	if err != nil {
		return nil, err
	}
	scenarioMap := make(map[string][]Scenario)
	for _, filePath := range filePaths {
		scenarios, err := loadTestDataFile(filePath)
		if err != nil {
			return nil, err
		}
		scenarioMap[filePath] = scenarios
	}
	return scenarioMap, nil
}

func loadTestDataFile(filePath string) ([]Scenario, error) {
	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var container []Scenario
	err = json.Unmarshal(file, &container)
	if err != nil {
		return nil, err
	}

	if len(container) == 0 {
		return nil, fmt.Errorf("Found zero Feature Flags to evaluate in file: %s", filePath)
	}
	return container, nil
}

// Given a baseDir returns an array of *.json file paths in that directory
func readTestDataDir(baseDir string) ([]string, error) {
	inputDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	fileInfos, err := ioutil.ReadDir(inputDir)

	if err != nil {
		return nil, err
	}
	filePaths := make([]string, 0, len(fileInfos))
	for _, fileInfo := range fileInfos {
		if strings.HasSuffix(fileInfo.Name(), ".json") {
			filePaths = append(filePaths, filepath.Join(inputDir, fileInfo.Name()))
		}
	}
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("Found zero *.json files in: %s", baseDir)
	}
	return filePaths, nil
}
