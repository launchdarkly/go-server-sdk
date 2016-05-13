package ldtest

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"github.com/launchdarkly/go-client"
	"github.com/deckarep/golang-set"
)
type AllTestData struct {
	Scenarios            []Scenario             // All scenarios for evaluation
	FeatureKeysToDelete  []string               // Feature keys to delete before starting
	FeatureFlagsToCreate []ldclient.FeatureFlag // Flags to create before starting
}


// Load all test data and uniqify each feature key in case there are duplicates
// create a set of feature keys to delete before creating them
// create a set of feature flags to create in the system being tested
func LoadTestData(basePath string) (*AllTestData, error) {
	scenariosMap, err := loadTestDataFiles(basePath)
	if err != nil {
		return nil, err
	}

	allScenarios := make([]Scenario, 0, 0)
	featureKeysToDeleteSet := mapset.NewSet()
	featureFlagsToCreate := make([]ldclient.FeatureFlag, 0, 0)

	for filePath, scenarios := range scenariosMap {
		_, fileName := filepath.Split(filePath)
		fileNameWithoutSuffix := strings.TrimSuffix(fileName, ".json")
		for i, s := range scenarios {
			//prefix all feature keys so they are unique.
			featureKeyPrefix := fmt.Sprintf("%s.%02d.", fileNameWithoutSuffix, i)
			featureKey := featureKeyPrefix + s.FeatureKey
			s.FeatureKey = featureKey
			featureKeysToDeleteSet.Add(featureKey)
			if len(s.TestCases) == 0 {
				return nil, fmt.Errorf("Found zero test cases to evaluate for file: %s for scenario: %s", filePath, s.Name)
			}
			for f, _ := range s.FeatureFlags {
				featureKey = featureKeyPrefix + s.FeatureFlags[f].Key
				s.FeatureFlags[f].Key = featureKey
				featureKeysToDeleteSet.Add(featureKey)
				featureFlagsToCreate = append(featureFlagsToCreate, s.FeatureFlags[f])
				for p, _ := range s.FeatureFlags[f].Prerequisites {
					featureKey = featureKeyPrefix + s.FeatureFlags[f].Prerequisites[p].Key
					s.FeatureFlags[f].Prerequisites[p].Key = featureKey
					featureKeysToDeleteSet.Add(featureKey)
				}
			}
			allScenarios = append(allScenarios, s)
		}
	}

	slice := featureKeysToDeleteSet.ToSlice()
	featureKeysToDelete := make([]string, len(slice))
	for i, key := range slice {
		if keyString, ok := key.(string) ; ok {
			featureKeysToDelete[i] = keyString
		}
	}
	return &AllTestData{allScenarios, featureKeysToDelete, featureFlagsToCreate}, nil
}

func loadTestDataFiles(basePath string) (map[string][]Scenario, error) {
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
