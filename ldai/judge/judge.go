package judge

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/launchdarkly/go-sdk-common/v4/ldvalue"
	"github.com/launchdarkly/go-server-sdk/ldai"
	"github.com/launchdarkly/go-server-sdk/ldai/datamodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

type Config interface {
	Messages() []datamodel.Message
	ModelParam(key string) (ldvalue.Value, bool)
	CustomModelParam(key string) (ldvalue.Value, bool)
	EvaluationMetricKey() string
	EvaluationMetricKeys() []string
}

type Tracker interface {
	TrackJudgeResponse(response datamodel.JudgeResponse) error
	TrackTokens(usage ldai.TokenUsage) error
}

type StructuredResponse struct {
	Content map[string]interface{}
	Usage   ldai.TokenUsage
}

type Provider interface {
	InvokeStructuredModel(messages []datamodel.Message, schema map[string]interface{}) (StructuredResponse, error)
}

type Judge struct {
	config         Config
	tracker        Tracker
	provider       Provider
	metricKey      string
	judgeConfigKey string
	logger         interfaces.LDLoggers
}

func New(config Config, tracker Tracker, provider Provider, configKey string, logger interfaces.LDLoggers) (*Judge, error) {
	if config == nil {
		return nil, fmt.Errorf("config must not be nil")
	}
	if tracker == nil {
		return nil, fmt.Errorf("tracker must not be nil")
	}
	if provider == nil {
		return nil, fmt.Errorf("provider must not be nil")
	}

	metricKey, err := getMetricKey(config, logger, configKey)
	if err != nil {
		return nil, err
	}

	return &Judge{
		config:         config,
		tracker:        tracker,
		provider:       provider,
		metricKey:      metricKey,
		judgeConfigKey: configKey,
		logger:         logger,
	}, nil
}

func (j *Judge) Evaluate(input, output string, samplingRate float64) (*datamodel.JudgeResponse, error) {
	if len(j.config.Messages()) == 0 {
		return nil, nil
	}

	if samplingRate < 1.0 && rand.Float64() > samplingRate {
		return nil, nil
	}

	messages := j.buildMessages(input, output)
	schema := buildSchema(j.metricKey)

	response, err := j.provider.InvokeStructuredModel(messages, schema)
	if err != nil {
		return &datamodel.JudgeResponse{
			Evals:          map[string]datamodel.EvalScore{},
			Success:        false,
			Error:          err.Error(),
			JudgeConfigKey: j.judgeConfigKey,
		}, nil
	}

	if response.Usage.Total > 0 || response.Usage.Input > 0 || response.Usage.Output > 0 {
		_ = j.tracker.TrackTokens(response.Usage)
	}

	result := j.parseResponse(response.Content)
	// Note: Judge response tracking should be done by the caller (AI config being evaluated)
	// not by the judge itself. This matches Python and JavaScript SDK behavior.

	return result, nil
}

func (j *Judge) EvaluateMessages(messages []datamodel.Message, response string, samplingRate float64) (*datamodel.JudgeResponse, error) {
	parts := make([]string, len(messages))
	for i, msg := range messages {
		parts[i] = msg.Content
	}
	input := strings.Join(parts, "\r\n")
	return j.Evaluate(input, response, samplingRate)
}

func (j *Judge) GetConfig() Config {
	return j.config
}

func (j *Judge) GetTracker() Tracker {
	return j.tracker
}

func (j *Judge) GetProvider() Provider {
	return j.provider
}

func (j *Judge) buildMessages(input, output string) []datamodel.Message {
	// Use string replacement to prevent context attributes like {{=[ ]=}}) from
	// influencing judge template parsing.
	replacer := strings.NewReplacer(
		ldai.JudgePlaceholderMessageHistory, input,
		ldai.JudgePlaceholderResponseToEvaluate, output,
	)

	messages := j.config.Messages()
	result := make([]datamodel.Message, len(messages))

	for i, msg := range messages {
		result[i] = datamodel.Message{Content: replacer.Replace(msg.Content), Role: msg.Role}
	}

	return result
}

func (j *Judge) parseResponse(data map[string]interface{}) *datamodel.JudgeResponse {
	evaluations, ok := data["evaluations"].(map[string]interface{})
	if !ok {
		return &datamodel.JudgeResponse{
			Evals:          map[string]datamodel.EvalScore{},
			Success:        false,
			Error:          "missing evaluations object",
			JudgeConfigKey: j.judgeConfigKey,
		}
	}

	evalData, ok := evaluations[j.metricKey].(map[string]interface{})
	if !ok {
		return &datamodel.JudgeResponse{
			Evals:          map[string]datamodel.EvalScore{},
			Success:        false,
			Error:          fmt.Sprintf("missing evaluation for %s", j.metricKey),
			JudgeConfigKey: j.judgeConfigKey,
		}
	}

	score, ok := evalData["score"].(float64)
	if !ok || score < 0 || score > 1 {
		return &datamodel.JudgeResponse{
			Evals:          map[string]datamodel.EvalScore{},
			Success:        false,
			Error:          "invalid score",
			JudgeConfigKey: j.judgeConfigKey,
		}
	}

	reasoning, ok := evalData["reasoning"].(string)
	if !ok {
		return &datamodel.JudgeResponse{
			Evals:          map[string]datamodel.EvalScore{},
			Success:        false,
			Error:          "invalid reasoning",
			JudgeConfigKey: j.judgeConfigKey,
		}
	}

	return &datamodel.JudgeResponse{
		Evals: map[string]datamodel.EvalScore{
			j.metricKey: {
				Score:     score,
				Reasoning: reasoning,
			},
		},
		Success:        true,
		JudgeConfigKey: j.judgeConfigKey,
	}
}

func getMetricKey(config Config, logger interfaces.LDLoggers, configKey string) (string, error) {
	// Priority 1: Check top-level evaluationMetricKey field (primary field)
	if metricKey := config.EvaluationMetricKey(); strings.TrimSpace(metricKey) != "" {
		return strings.TrimSpace(metricKey), nil
	}

	// Priority 2: Check top-level evaluationMetricKeys array (deprecated)
	keys := config.EvaluationMetricKeys()
	for _, key := range keys {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			if logger != nil {
				logger.Warnf("Judge config %q: using deprecated evaluationMetricKeys; use evaluationMetricKey instead", configKey)
			}
			return trimmed, nil
		}
	}

	if configKey != "" {
		return "", fmt.Errorf("judge config %q: missing evaluationMetricKey", configKey)
	}
	return "", fmt.Errorf("missing evaluationMetricKey")
}
