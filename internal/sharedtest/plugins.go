package sharedtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v4/ldreason"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/launchdarkly/go-server-sdk/v7/ldplugins"
)

type getHooksCall struct {
	metadata ldplugins.EnvironmentMetadata
}

type registerCall struct {
	client   interfaces.LDClientInterface
	metadata ldplugins.EnvironmentMetadata
}

type pluginTestData struct {
	getHooksCalls []getHooksCall
	registerCalls []registerCall
}

// TestPlugin is a plugin for testing to be used only by the SDK tests.
type TestPlugin struct {
	testData *pluginTestData
	hooks    []ldhooks.Hook
	metadata ldplugins.Metadata
}

// NewTestPlugin creates a new test plugin.
func NewTestPlugin(name string, hooks []ldhooks.Hook) TestPlugin {
	return TestPlugin{
		testData: &pluginTestData{
			getHooksCalls: make([]getHooksCall, 0),
			registerCalls: make([]registerCall, 0),
		},
		hooks:    hooks,
		metadata: ldplugins.NewMetadata(name),
	}
}

// Metadata gets the meta-data for the plugin.
func (p TestPlugin) Metadata() ldplugins.Metadata {
	return p.metadata
}

// Register testing implementation of the Register method.
func (p TestPlugin) Register(client interfaces.LDClientInterface, metadata ldplugins.EnvironmentMetadata) {
	p.testData.registerCalls = append(p.testData.registerCalls, registerCall{
		client:   client,
		metadata: metadata,
	})
}

// GetHooks testing implementation of the GetHooks method.
func (p TestPlugin) GetHooks(metadata ldplugins.EnvironmentMetadata) []ldhooks.Hook {
	p.testData.getHooksCalls = append(p.testData.getHooksCalls, getHooksCall{
		metadata: metadata,
	})
	return p.hooks
}

// ExpectRegisterCalled asserts that Register was called with the given arguments.
func (p TestPlugin) ExpectRegisterCalled(
	t *testing.T,
	expectedClient interfaces.LDClientInterface,
	expectedMetadata ldplugins.EnvironmentMetadata,
) {
	assert.Len(t, p.testData.registerCalls, 1)
	assert.Equal(t, expectedClient, p.testData.registerCalls[0].client)
	assert.Equal(t, expectedMetadata, p.testData.registerCalls[0].metadata)
}

// ExpectGetHooksCalled asserts that GetHooks was called with the given arguments.
func (p TestPlugin) ExpectGetHooksCalled(t *testing.T, expectedMetadata ldplugins.EnvironmentMetadata) {
	assert.Len(t, p.testData.getHooksCalls, 1)
	assert.Equal(t, expectedMetadata, p.testData.getHooksCalls[0].metadata)
}

type pluginHookTestData struct {
	beforeEvaluationCalled bool
	afterEvaluationCalled  bool
}

// TestPluginHook is a plugin hook for testing to be used only by the SDK tests.
//
// This differs from TestHook in that we only care if the evaluation hook methods
// are called and not about testing evaluation hook logic itself.
type TestPluginHook struct {
	ldhooks.Unimplemented
	testData *pluginHookTestData
	metadata ldhooks.Metadata
}

// NewTestPluginHook creates a new test plugin hook.
func NewTestPluginHook(name string) TestPluginHook {
	return TestPluginHook{
		testData: &pluginHookTestData{
			beforeEvaluationCalled: false,
			afterEvaluationCalled:  false,
		},
		metadata: ldhooks.NewMetadata(name),
	}
}

// Metadata gets the meta-data for the plugin hook.
func (h TestPluginHook) Metadata() ldhooks.Metadata {
	return h.metadata
}

// BeforeEvaluation testing implementation of the BeforeEvaluation stage.
func (h TestPluginHook) BeforeEvaluation(
	ctx context.Context,
	seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData,
) (ldhooks.EvaluationSeriesData, error) {
	h.testData.beforeEvaluationCalled = true
	return data, nil
}

// AfterEvaluation testing implementation of the AfterEvaluation stage.
func (h TestPluginHook) AfterEvaluation(
	ctx context.Context,
	seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData,
	detail ldreason.EvaluationDetail,
) (ldhooks.EvaluationSeriesData, error) {
	h.testData.afterEvaluationCalled = true
	return data, nil
}

// ExpectBeforeEvaluationCalled asserts that BeforeEvaluation was called.
func (h TestPluginHook) ExpectBeforeEvaluationCalled(t *testing.T) {
	assert.True(t, h.testData.beforeEvaluationCalled)
}

// ExpectAfterEvaluationCalled asserts that AfterEvaluation was called.
func (h TestPluginHook) ExpectAfterEvaluationCalled(t *testing.T) {
	assert.True(t, h.testData.afterEvaluationCalled)
}
