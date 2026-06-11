package judge

import (
	"fmt"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/ldai"
	"github.com/launchdarkly/go-server-sdk/ldai/datamodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockConfig struct {
	messages             []datamodel.Message
	modelParam           map[string]ldvalue.Value
	customParam          map[string]ldvalue.Value
	evaluationMetricKey  string
	evaluationMetricKeys []string
}

func (m *mockConfig) Messages() []datamodel.Message {
	return m.messages
}

func (m *mockConfig) ModelParam(key string) (ldvalue.Value, bool) {
	val, ok := m.modelParam[key]
	return val, ok
}

func (m *mockConfig) CustomModelParam(key string) (ldvalue.Value, bool) {
	val, ok := m.customParam[key]
	return val, ok
}

func (m *mockConfig) EvaluationMetricKey() string {
	return m.evaluationMetricKey
}

func (m *mockConfig) EvaluationMetricKeys() []string {
	return m.evaluationMetricKeys
}

type mockTracker struct {
	judgeResponses []datamodel.JudgeResponse
	usages         []ldai.TokenUsage
}

func (m *mockTracker) TrackJudgeResponse(response datamodel.JudgeResponse) error {
	m.judgeResponses = append(m.judgeResponses, response)
	return nil
}

func (m *mockTracker) TrackTokens(usage ldai.TokenUsage) error {
	m.usages = append(m.usages, usage)
	return nil
}

type mockProvider struct {
	response StructuredResponse
	err      error
	calls    [][]datamodel.Message
}

func (m *mockProvider) InvokeStructuredModel(messages []datamodel.Message, schema map[string]interface{}) (StructuredResponse, error) {
	m.calls = append(m.calls, messages)
	return m.response, m.err
}

func TestNew(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
	}
	tracker := &mockTracker{}
	provider := &mockProvider{}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)
	assert.NotNil(t, judge)
	assert.Equal(t, "$ld:ai:judge:relevance", judge.metricKey)
	assert.Equal(t, "test-judge", judge.judgeConfigKey)
}

func TestNew_MissingMetricKey(t *testing.T) {
	config := &mockConfig{}
	tracker := &mockTracker{}
	provider := &mockProvider{}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	assert.Error(t, err)
	assert.Nil(t, judge)
	assert.Contains(t, err.Error(), "missing evaluationMetricKey")
}

func TestNew_NilInputs(t *testing.T) {
	config := &mockConfig{evaluationMetricKey: "test"}
	tracker := &mockTracker{}
	provider := &mockProvider{}

	_, err := New(nil, tracker, provider, "test", nil)
	assert.Error(t, err)

	_, err = New(config, nil, provider, "test", nil)
	assert.Error(t, err)

	_, err = New(config, tracker, nil, "test", nil)
	assert.Error(t, err)
}

func TestEvaluate_Success(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages: []datamodel.Message{
			{Role: datamodel.System, Content: "Evaluate this"},
			{Role: datamodel.User, Content: "Input: {{message_history}}"},
			{Role: datamodel.User, Content: "Output: {{response_to_evaluate}}"},
		},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     0.85,
						"reasoning": "Highly relevant",
					},
				},
			},
			Usage: ldai.TokenUsage{Total: 100, Input: 60, Output: 40},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("test input", "test output", 1.0)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "test-judge", result.JudgeConfigKey)
	assert.Len(t, result.Evals, 1)
	assert.Equal(t, 0.85, result.Evals["$ld:ai:judge:relevance"].Score)
	assert.Equal(t, "Highly relevant", result.Evals["$ld:ai:judge:relevance"].Reasoning)

	assert.Len(t, tracker.usages, 1)
	assert.Equal(t, 100, tracker.usages[0].Total)
	// Note: Judge should NOT track responses internally - this is caller's responsibility
	// The judge's tracker is only used for usage/duration metrics
	assert.Len(t, tracker.judgeResponses, 0, "Judge should not track responses internally")

	require.Len(t, provider.calls, 1)
	assert.Equal(t, "Evaluate this", provider.calls[0][0].Content)
	assert.Equal(t, "Input: test input", provider.calls[0][1].Content)
	assert.Equal(t, "Output: test output", provider.calls[0][2].Content)
}

func TestEvaluate_NoMessages(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestEvaluate_Sampling(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	sampled := 0
	for i := 0; i < 100; i++ {
		result, _ := judge.Evaluate("input", "output", 0.0)
		if result != nil {
			sampled++
		}
	}
	assert.Equal(t, 0, sampled)
}

func TestEvaluate_ProviderError(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{err: fmt.Errorf("provider error")}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "test-judge", result.JudgeConfigKey)
	assert.Contains(t, result.Error, "provider error")
}

func TestEvaluate_InvalidResponse(t *testing.T) {
	tests := []struct {
		name     string
		response map[string]interface{}
	}{
		{
			name:     "missing evaluations",
			response: map[string]interface{}{},
		},
		{
			name: "missing metric key",
			response: map[string]interface{}{
				"evaluations": map[string]interface{}{},
			},
		},
		{
			name: "invalid score type",
			response: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     "not a number",
						"reasoning": "test",
					},
				},
			},
		},
		{
			name: "score out of range",
			response: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     1.5,
						"reasoning": "test",
					},
				},
			},
		},
		{
			name: "invalid reasoning type",
			response: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     0.5,
						"reasoning": 123,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &mockConfig{
				evaluationMetricKey: "$ld:ai:judge:relevance",
				messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
			}
			tracker := &mockTracker{}
			provider := &mockProvider{
				response: StructuredResponse{Content: tt.response},
			}

			judge, err := New(config, tracker, provider, "test-judge", nil)
			require.NoError(t, err)

			result, err := judge.Evaluate("input", "output", 1.0)
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.False(t, result.Success)
			assert.NotEmpty(t, result.Error)
		})
	}
}

func TestEvaluateMessages(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "{{message_history}}"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     0.9,
						"reasoning": "Excellent",
					},
				},
			},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	messages := []datamodel.Message{
		{Role: datamodel.User, Content: "Hello"},
		{Role: datamodel.Assistant, Content: "Hi there"},
	}

	result, err := judge.EvaluateMessages(messages, "response", 1.0)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)

	require.Len(t, provider.calls, 1)
	assert.Contains(t, provider.calls[0][0].Content, "Hello\r\nHi there")
}

func TestGetMetricKey(t *testing.T) {
	tests := []struct {
		name                 string
		evaluationMetricKey  string
		evaluationMetricKeys []string
		want                 string
		wantErr              bool
	}{
		{
			name:                "from top-level field (primary)",
			evaluationMetricKey: "$ld:ai:judge:toplevel",
			want:                "$ld:ai:judge:toplevel",
		},
		{
			name:                 "top-level field has priority over array",
			evaluationMetricKey:  "$ld:ai:judge:toplevel",
			evaluationMetricKeys: []string{"$ld:ai:judge:array"},
			want:                 "$ld:ai:judge:toplevel",
		},
		{
			name:    "missing",
			wantErr: true,
		},
		{
			name:                "trim whitespace from top-level",
			evaluationMetricKey: "  $ld:ai:judge:toplevel  ",
			want:                "$ld:ai:judge:toplevel",
		},
		{
			name:                 "from evaluationMetricKeys array",
			evaluationMetricKeys: []string{"$ld:ai:judge:relevance", "$ld:ai:judge:accuracy"},
			want:                 "$ld:ai:judge:relevance",
		},
		{
			name:                 "skip empty strings in array",
			evaluationMetricKeys: []string{"", "  ", "$ld:ai:judge:relevance"},
			want:                 "$ld:ai:judge:relevance",
		},
		{
			name:                 "trim whitespace from array entry",
			evaluationMetricKeys: []string{"  $ld:ai:judge:relevance  "},
			want:                 "$ld:ai:judge:relevance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &mockConfig{
				evaluationMetricKey:  tt.evaluationMetricKey,
				evaluationMetricKeys: tt.evaluationMetricKeys,
			}
			got, err := getMetricKey(config, nil, "test-judge")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestBuildSchema(t *testing.T) {
	schema := buildSchema("$ld:ai:judge:relevance")

	assert.Equal(t, "object", schema["type"])
	assert.Contains(t, schema, "properties")
	assert.Contains(t, schema, "required")

	props := schema["properties"].(map[string]interface{})
	evals := props["evaluations"].(map[string]interface{})
	evalProps := evals["properties"].(map[string]interface{})

	assert.Contains(t, evalProps, "$ld:ai:judge:relevance")

	metricSchema := evalProps["$ld:ai:judge:relevance"].(map[string]interface{})
	metricProps := metricSchema["properties"].(map[string]interface{})

	assert.Contains(t, metricProps, "score")
	assert.Contains(t, metricProps, "reasoning")

	scoreSchema := metricProps["score"].(map[string]interface{})
	assert.Equal(t, "number", scoreSchema["type"])
	assert.Equal(t, 0.0, scoreSchema["minimum"])
	assert.Equal(t, 1.0, scoreSchema["maximum"])
}

func TestBuildSchema_Empty(t *testing.T) {
	schema := buildSchema("")
	assert.Empty(t, schema)
}

func TestEvaluate_NegativeScore(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     -0.5,
						"reasoning": "negative score",
					},
				},
			},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "invalid score")
}

func TestEvaluate_ScoreGreaterThanOne(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     1.5,
						"reasoning": "over limit",
					},
				},
			},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "invalid score")
}

func TestEvaluate_NullEvaluationValue(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": nil,
				},
			},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "missing evaluation")
}

func TestEvaluate_NonObjectEvaluationValue(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": "not an object",
				},
			},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "missing evaluation")
}

func TestEvaluate_ScoreAsString(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     "0.5",
						"reasoning": "test",
					},
				},
			},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "invalid score")
}

func TestEvaluate_ReasoningAsNumber(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     0.5,
						"reasoning": 123,
					},
				},
			},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "invalid reasoning")
}

func TestEvaluate_EmptyEvaluationsObject(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{},
			},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "missing evaluation")
}

func TestGetMetricKey_EmptyArray(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKeys: []string{},
	}

	_, err := getMetricKey(config, nil, "test-judge")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing evaluationMetricKey")
}

func TestGetMetricKey_ArrayWithOnlyEmptyStrings(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKeys: []string{"", "  ", "\t"},
	}

	_, err := getMetricKey(config, nil, "test-judge")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing evaluationMetricKey")
}

func TestEvaluate_ReturnsCorrectResponse(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     0.75,
						"reasoning": "Good response",
					},
				},
			},
		},
	}

	judge, err := New(config, tracker, provider, "my-judge-config", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the returned response has correct values
	assert.Equal(t, "my-judge-config", result.JudgeConfigKey)
	assert.True(t, result.Success)
	assert.Equal(t, 0.75, result.Evals["$ld:ai:judge:relevance"].Score)
	assert.Equal(t, "Good response", result.Evals["$ld:ai:judge:relevance"].Reasoning)

	// Judge should NOT track responses internally - this is caller's responsibility
	assert.Len(t, tracker.judgeResponses, 0, "Judge should not track responses internally")
}

func TestEvaluate_TokenUsageTracked(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     0.5,
						"reasoning": "test",
					},
				},
			},
			Usage: ldai.TokenUsage{Total: 150, Input: 90, Output: 60},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	_, err = judge.Evaluate("input", "output", 1.0)
	require.NoError(t, err)

	require.Len(t, tracker.usages, 1)
	assert.Equal(t, 150, tracker.usages[0].Total)
	assert.Equal(t, 90, tracker.usages[0].Input)
	assert.Equal(t, 60, tracker.usages[0].Output)
}

func TestEvaluate_NoTokenUsageWhenZero(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     0.5,
						"reasoning": "test",
					},
				},
			},
			Usage: ldai.TokenUsage{},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	_, err = judge.Evaluate("input", "output", 1.0)
	require.NoError(t, err)

	assert.Len(t, tracker.usages, 0)
}

func TestEvaluate_ErrorResponseIncludesJudgeConfigKey(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}
	tracker := &mockTracker{}
	provider := &mockProvider{err: fmt.Errorf("test error")}

	judge, err := New(config, tracker, provider, "error-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "error-judge", result.JudgeConfigKey)
}

// Integration Tests - These verify end-to-end behavior and patterns not caught by unit tests

// TestDoubleInterpolation_ReservedVariables verifies that the double interpolation pattern works:
// 1. During config fetch, pass literal strings "{{message_history}}" and "{{response_to_evaluate}}"
// 2. First interpolation preserves these placeholders in the template
// 3. During evaluation, second interpolation replaces placeholders with actual values
func TestDoubleInterpolation_ReservedVariables(t *testing.T) {
	// Simulate what the client does when fetching a judge config
	// The config from LaunchDarkly has templates with {{message_history}} and {{response_to_evaluate}}
	rawTemplate := "Input: {{message_history}}\nOutput: {{response_to_evaluate}}"

	// Simulate first interpolation (done by client.JudgeConfig when fetching judge config)
	// Variables passed should include literal placeholder strings
	variablesForFirstInterpolation := map[string]interface{}{
		"message_history":      "{{message_history}}",      // Literal string!
		"response_to_evaluate": "{{response_to_evaluate}}", // Literal string!
	}

	// First interpolation - should preserve placeholders
	firstInterpolated := interpolateTemplateForTest(rawTemplate, variablesForFirstInterpolation)
	assert.Equal(t, rawTemplate, firstInterpolated, "First interpolation should preserve placeholders")

	// Now simulate what the judge does during Evaluate()
	// Second interpolation with actual values
	actualInput := "What is LaunchDarkly?"
	actualOutput := "LaunchDarkly is a feature management platform."

	variablesForSecondInterpolation := map[string]interface{}{
		"message_history":      actualInput,
		"response_to_evaluate": actualOutput,
	}

	// Second interpolation - should replace with actual values
	secondInterpolated := interpolateTemplateForTest(firstInterpolated, variablesForSecondInterpolation)
	expected := "Input: What is LaunchDarkly?\nOutput: LaunchDarkly is a feature management platform."
	assert.Equal(t, expected, secondInterpolated, "Second interpolation should replace with actual values")
}

// TestJudgeEvaluation_WithTemplateInterpolation verifies end-to-end template interpolation
func TestJudgeEvaluation_WithTemplateInterpolation(t *testing.T) {
	// Config with templates that should be interpolated during evaluation
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:test",
		messages: []datamodel.Message{
			{Role: datamodel.System, Content: "You are a judge"},
			{Role: datamodel.User, Content: "Input: {{message_history}}"},
			{Role: datamodel.User, Content: "Output: {{response_to_evaluate}}"},
		},
	}

	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:test": map[string]interface{}{
						"score":     0.9,
						"reasoning": "Good response",
					},
				},
			},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	// Evaluate with actual input/output
	actualInput := "What is AI?"
	actualOutput := "AI is artificial intelligence"

	result, err := judge.Evaluate(actualInput, actualOutput, 1.0)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the provider received interpolated messages
	require.Len(t, provider.calls, 1)
	messages := provider.calls[0]

	assert.Equal(t, "You are a judge", messages[0].Content)
	assert.Equal(t, "Input: What is AI?", messages[1].Content, "message_history should be interpolated")
	assert.Equal(t, "Output: AI is artificial intelligence", messages[2].Content, "response_to_evaluate should be interpolated")
}

// TestJudgeTracking_ShouldNotTrackInternally verifies that the judge does NOT track responses internally.
// Tracking should be the responsibility of the caller (AI config being evaluated).
func TestJudgeTracking_ShouldNotTrackInternally(t *testing.T) {
	config := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:test",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: "test"}},
	}

	tracker := &mockTracker{}
	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:test": map[string]interface{}{
						"score":     0.5,
						"reasoning": "Test",
					},
				},
			},
		},
	}

	judge, err := New(config, tracker, provider, "test-judge", nil)
	require.NoError(t, err)

	result, err := judge.Evaluate("input", "output", 1.0)
	require.NoError(t, err)
	require.NotNil(t, result)

	// CRITICAL: Judge should NOT track responses internally
	// This should be done by the caller (AI config being evaluated)
	assert.Len(t, tracker.judgeResponses, 0, "Judge should not track responses internally - caller's responsibility")

	// Verify usage is still tracked (this is judge-specific)
	assert.Len(t, tracker.usages, 0, "No usage was set in this test")
}

// TestIntegration_AIConfigTracksJudgeResults simulates the real-world pattern where
// an AI config evaluates with a judge and tracks results on its own tracker.
func TestIntegration_AIConfigTracksJudgeResults(t *testing.T) {
	// Simulate AI config's tracker
	aiConfigTracker := &mockTracker{}

	// Simulate judge's tracker (should NOT be used for judge response tracking)
	judgeTracker := &mockTracker{}

	// Judge configuration
	judgeConfig := &mockConfig{
		evaluationMetricKey: "$ld:ai:judge:relevance",
		messages: []datamodel.Message{
			{Role: datamodel.User, Content: "Evaluate: {{message_history}} -> {{response_to_evaluate}}"},
		},
	}

	provider := &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"$ld:ai:judge:relevance": map[string]interface{}{
						"score":     0.85,
						"reasoning": "Highly relevant",
					},
				},
			},
		},
	}

	// Create judge with its own tracker
	judge, err := New(judgeConfig, judgeTracker, provider, "test-judge", nil)
	require.NoError(t, err)

	// AI config evaluates with the judge
	result, err := judge.Evaluate("What is AI?", "AI is artificial intelligence", 1.0)
	require.NoError(t, err)
	require.NotNil(t, result)

	// AI config tracks the result on its own tracker (NOT the judge's tracker)
	err = aiConfigTracker.TrackJudgeResponse(*result)
	require.NoError(t, err)

	// Verify tracking happened on AI config's tracker
	assert.Len(t, aiConfigTracker.judgeResponses, 1, "AI config should track judge response")
	assert.Equal(t, 0.85, aiConfigTracker.judgeResponses[0].Evals["$ld:ai:judge:relevance"].Score)

	// Verify judge did NOT track on its own tracker
	assert.Len(t, judgeTracker.judgeResponses, 0, "Judge should not track responses internally")
}

// Helper function to test template interpolation
func interpolateTemplateForTest(template string, vars map[string]interface{}) string {
	// Simple string replacement for testing
	// Real code uses: mustache.New().ParseString(template).RenderString(vars)
	result := template
	for key, value := range vars {
		placeholder := "{{" + key + "}}"
		if str, ok := value.(string); ok {
			result = replaceAllForTest(result, placeholder, str)
		}
	}
	return result
}

func replaceAllForTest(s, old, new string) string {
	result := ""
	for {
		i := indexOfForTest(s, old)
		if i == -1 {
			result += s
			break
		}
		result += s[:i] + new
		s = s[i+len(old):]
	}
	return result
}

func indexOfForTest(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func newMinimalJudge(t *testing.T, content string) *Judge {
	t.Helper()
	config := &mockConfig{
		evaluationMetricKey: "metric",
		messages:            []datamodel.Message{{Role: datamodel.User, Content: content}},
	}
	j, err := New(config, &mockTracker{}, &mockProvider{
		response: StructuredResponse{
			Content: map[string]interface{}{
				"evaluations": map[string]interface{}{
					"metric": map[string]interface{}{"score": 1.0, "reasoning": "ok"},
				},
			},
		},
	}, "key", nil)
	require.NoError(t, err)
	return j
}

// TestBuildMessages_InjectionVariants is a regression test for HackerOne report #3591852.
// It covers the full set of Mustache control sequences an attacker could inject via a
// user-controlled context variable. All variants must be treated as inert literal text by
// pass 2 — none should cause a placeholder to go unsubstituted.
func TestBuildMessages_InjectionVariants(t *testing.T) {
	variants := []struct {
		name    string
		payload string // injected via {{ldctx.user.name}} in the template
	}{
		{"delimiter change brackets", "{{=[ ]=}}"},
		{"delimiter change angle", "{{=<% %>=}}"},
		{"partial", "{{> evil}}"},
		{"comment", "{{! drop everything }}"},
		{"triple stache", "{{{raw}}}"},
		{"section", "{{#section}}inject{{/section}}"},
		{"inverted section", "{{^section}}inject{{/section}}"},
	}

	for _, tt := range variants {
		t.Run(tt.name, func(t *testing.T) {
			// Hand-craft the pass-1 output: user.name resolved to the attack payload,
			// reserved placeholder survived as a literal string.
			afterPass1 := "Auditing " + tt.payload + ": " + ldai.JudgePlaceholderMessageHistory

			// Pass 2: judge substitutes placeholders
			judge := newMinimalJudge(t, afterPass1)
			actualHistory := "ACTUAL MESSAGE HISTORY"
			messages := judge.buildMessages(actualHistory, "some output")

			require.Len(t, messages, 1)
			assert.Contains(t, messages[0].Content, actualHistory,
				"payload %q must not blind the judge to the actual history", tt.payload)
			assert.NotContains(t, messages[0].Content, ldai.JudgePlaceholderMessageHistory,
				"placeholder must be fully substituted after payload %q", tt.payload)
		})
	}
}

// TestBuildMessages_InjectionViaResponse verifies that injection payloads in the response
// being evaluated (not just the history) are equally neutralized.
func TestBuildMessages_InjectionViaResponse(t *testing.T) {
	// Pass-1 output: no user vars resolved, both placeholders survived as literal strings.
	afterPass1 := "History: " + ldai.JudgePlaceholderMessageHistory +
		"\nResponse: " + ldai.JudgePlaceholderResponseToEvaluate

	judge := newMinimalJudge(t, afterPass1)
	maliciousResponse := "{{=[ ]=}} INJECTION ATTEMPT"
	messages := judge.buildMessages("normal history", maliciousResponse)

	require.Len(t, messages, 1)
	assert.Contains(t, messages[0].Content, maliciousResponse,
		"malicious content in response must appear verbatim — not silently dropped")
	assert.NotContains(t, messages[0].Content, ldai.JudgePlaceholderResponseToEvaluate,
		"response placeholder must be fully substituted")
}

// TestBuildMessages_MultiplePlaceholderOccurrences verifies that when a template contains
// the same placeholder more than once, every occurrence is substituted.
func TestBuildMessages_MultiplePlaceholderOccurrences(t *testing.T) {
	template := ldai.JudgePlaceholderMessageHistory + " | " + ldai.JudgePlaceholderMessageHistory
	judge := newMinimalJudge(t, template)
	messages := judge.buildMessages("HISTORY", "RESPONSE")

	require.Len(t, messages, 1)
	assert.Equal(t, "HISTORY | HISTORY", messages[0].Content)
}

// TestBuildMessages_MustacheSyntaxInContent verifies that Mustache-like syntax inside the
// actual history or response values is treated as literal text and not silently consumed.
// The old Mustache-based pass 2 would have rendered unrecognized tags (e.g. {{user}}) as
// empty strings, corrupting content such as code samples or questions about templating.
func TestBuildMessages_MustacheSyntaxInContent(t *testing.T) {
	template := "History: " + ldai.JudgePlaceholderMessageHistory +
		"\nResponse: " + ldai.JudgePlaceholderResponseToEvaluate
	judge := newMinimalJudge(t, template)

	historyWithMustache := "How do I use {{user}} in Mustache?"
	responseWithMustache := "Use {{user}} like this: {{#user}}Hello{{/user}}"

	messages := judge.buildMessages(historyWithMustache, responseWithMustache)

	require.Len(t, messages, 1)
	assert.Contains(t, messages[0].Content, historyWithMustache,
		"Mustache-like syntax in history must be preserved verbatim")
	assert.Contains(t, messages[0].Content, responseWithMustache,
		"Mustache-like syntax in response must be preserved verbatim")
}
