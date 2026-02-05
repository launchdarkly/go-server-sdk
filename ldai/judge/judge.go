package judge

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/alexkappa/mustache"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
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
	TrackUsage(usage ldai.TokenUsage) error
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
		if j.logger != nil {
			j.logger.Warnf("Judge '%s': config must include messages", j.judgeConfigKey)
		}
		return nil, nil
	}

	if samplingRate < 1.0 && rand.Float64() > samplingRate {
		if j.logger != nil {
			j.logger.Debugf("Judge '%s': evaluation skipped due to sampling rate: %.2f", j.judgeConfigKey, samplingRate)
		}
		return nil, nil
	}

	messages := j.buildMessages(input, output)
	schema := buildSchema(j.metricKey)

	response, err := j.provider.InvokeStructuredModel(messages, schema)
	if err != nil {
		if j.logger != nil {
			j.logger.Errorf("Judge '%s': evaluation failed: %v", j.judgeConfigKey, err)
		}
		return &datamodel.JudgeResponse{
			Evals:          map[string]datamodel.EvalScore{},
			Success:        false,
			Error:          err.Error(),
			JudgeConfigKey: j.judgeConfigKey,
		}, nil
	}

	if response.Usage.Total > 0 || response.Usage.Input > 0 || response.Usage.Output > 0 {
		_ = j.tracker.TrackUsage(response.Usage)
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
	vars := map[string]interface{}{
		"message_history":      input,
		"response_to_evaluate": output,
	}

	messages := j.config.Messages()
	result := make([]datamodel.Message, len(messages))

	if j.logger != nil {
		j.logger.Debugf("Judge '%s': Building messages with input length=%d, output length=%d", j.judgeConfigKey, len(input), len(output))
		for i, msg := range messages {
			j.logger.Debugf("Judge '%s': Template %d [%s]: %q", j.judgeConfigKey, i+1, msg.Role, msg.Content)
		}
	}

	for i, msg := range messages {
		m := mustache.New()
		if err := m.ParseString(msg.Content); err != nil {
			if j.logger != nil {
				j.logger.Debugf("Judge '%s': failed to parse template: %v", j.judgeConfigKey, err)
			}
			result[i] = datamodel.Message{Content: msg.Content, Role: msg.Role}
			continue
		}
		content, err := m.RenderString(vars)
		if err != nil {
			if j.logger != nil {
				j.logger.Debugf("Judge '%s': failed to render template: %v", j.judgeConfigKey, err)
			}
			result[i] = datamodel.Message{Content: msg.Content, Role: msg.Role}
			continue
		}
		result[i] = datamodel.Message{Content: content, Role: msg.Role}
	}

	return result
}

func (j *Judge) parseResponse(data map[string]interface{}) *datamodel.JudgeResponse {
	evaluations, ok := data["evaluations"].(map[string]interface{})
	if !ok {
		if j.logger != nil {
			j.logger.Warnf("Judge '%s': invalid response - missing or invalid evaluations object", j.judgeConfigKey)
		}
		return &datamodel.JudgeResponse{
			Evals:          map[string]datamodel.EvalScore{},
			Success:        false,
			Error:          "missing evaluations object",
			JudgeConfigKey: j.judgeConfigKey,
		}
	}

	evalData, ok := evaluations[j.metricKey].(map[string]interface{})
	if !ok {
		if j.logger != nil {
			j.logger.Warnf("Judge '%s': missing evaluation for metric key: %s", j.judgeConfigKey, j.metricKey)
		}
		return &datamodel.JudgeResponse{
			Evals:          map[string]datamodel.EvalScore{},
			Success:        false,
			Error:          fmt.Sprintf("missing evaluation for %s", j.metricKey),
			JudgeConfigKey: j.judgeConfigKey,
		}
	}

	score, ok := evalData["score"].(float64)
	if !ok || score < 0 || score > 1 {
		if j.logger != nil {
			j.logger.Warnf("Judge '%s': invalid score for %s: %v. Score must be a number between 0 and 1 inclusive",
				j.judgeConfigKey, j.metricKey, evalData["score"])
		}
		return &datamodel.JudgeResponse{
			Evals:          map[string]datamodel.EvalScore{},
			Success:        false,
			Error:          "invalid score",
			JudgeConfigKey: j.judgeConfigKey,
		}
	}

	reasoning, ok := evalData["reasoning"].(string)
	if !ok {
		if j.logger != nil {
			j.logger.Warnf("Judge '%s': invalid reasoning for %s: %v. Reasoning must be a string",
				j.judgeConfigKey, j.metricKey, evalData["reasoning"])
		}
		return &datamodel.JudgeResponse{
			Evals:          map[string]datamodel.EvalScore{},
			Success:        false,
			Error:          "invalid reasoning",
			JudgeConfigKey: j.judgeConfigKey,
		}
	}

	if j.logger != nil {
		j.logger.Debugf("Judge '%s': Parsed score=%f for metric=%s", j.judgeConfigKey, score, j.metricKey)
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
			return trimmed, nil
		}
	}

	return "", fmt.Errorf("missing evaluationMetricKey")
}
