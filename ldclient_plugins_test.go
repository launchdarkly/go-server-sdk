package ldclient

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/launchdarkly/go-server-sdk/v7/ldplugins"
)

func TestPluginsAreProvidedEnvironmentMetadata(t *testing.T) {
	plugin := sharedtest.NewTestPlugin("test-plugin", []ldhooks.Hook{})

	client, _ := MakeCustomClient("sdk-key", Config{
		Offline: true,
		Plugins: []ldplugins.Plugin{plugin},
	}, 0)

	expectedEnvironment := ldplugins.EnvironmentMetadata{
		Sdk: ldplugins.SdkMetadata{
			Name:           "GoClient",
			Version:        internal.SDKVersion,
			WrapperName:    "",
			WrapperVersion: "",
		},
		SdkKey: "sdk-key",
		Application: ldplugins.ApplicationMetadata{
			ID:      "",
			Version: "",
		},
	}

	plugin.ExpectGetHooksCalled(t, expectedEnvironment)
	plugin.ExpectRegisterCalled(t, client, expectedEnvironment)
}

func TestPluginsAreProvidedEnvironmentMetadataWithAppInfo(t *testing.T) {
	plugin := sharedtest.NewTestPlugin("test-plugin", []ldhooks.Hook{})

	client, _ := MakeCustomClient("sdk-key", Config{
		Offline: true,
		ApplicationInfo: interfaces.ApplicationInfo{
			ApplicationID:      "app-id",
			ApplicationVersion: "app-version",
		},
		Plugins: []ldplugins.Plugin{plugin},
	}, 0)

	expectedEnvironment := ldplugins.EnvironmentMetadata{
		Sdk: ldplugins.SdkMetadata{
			Name:           "GoClient",
			Version:        internal.SDKVersion,
			WrapperName:    "",
			WrapperVersion: "",
		},
		SdkKey: "sdk-key",
		Application: ldplugins.ApplicationMetadata{
			ID:      "app-id",
			Version: "app-version",
		},
	}

	plugin.ExpectGetHooksCalled(t, expectedEnvironment)
	plugin.ExpectRegisterCalled(t, client, expectedEnvironment)
}

func TestPluginsAreProvidedEnvironmentMetadataWithWrapper(t *testing.T) {
	plugin := sharedtest.NewTestPlugin("test-plugin", []ldhooks.Hook{})

	client, _ := MakeCustomClient("sdk-key", Config{
		Offline: true,
		HTTP:    ldcomponents.HTTPConfiguration().Wrapper("wrapper-name", "wrapper-version"),
		Plugins: []ldplugins.Plugin{plugin},
	}, 0)

	expectedEnvironment := ldplugins.EnvironmentMetadata{
		Sdk: ldplugins.SdkMetadata{
			Name:           "GoClient",
			Version:        internal.SDKVersion,
			WrapperName:    "wrapper-name",
			WrapperVersion: "wrapper-version",
		},
		SdkKey: "sdk-key",
		Application: ldplugins.ApplicationMetadata{
			ID:      "",
			Version: "",
		},
	}

	plugin.ExpectGetHooksCalled(t, expectedEnvironment)
	plugin.ExpectRegisterCalled(t, client, expectedEnvironment)
}

func TestPluginHooksAreRegistered(t *testing.T) {
	context := ldcontext.New("test-context")
	pluginHook := sharedtest.NewTestPluginHook("test-plugin-hook")
	plugin := sharedtest.NewTestPlugin("test-plugin", []ldhooks.Hook{pluginHook})

	client, _ := MakeCustomClient("", Config{Offline: true, Plugins: []ldplugins.Plugin{plugin}}, 0)

	client.BoolVariation("test-flag", context, false)

	pluginHook.ExpectBeforeEvaluationCalled(t)
	pluginHook.ExpectAfterEvaluationCalled(t)
}

func TestPluginHooksAreRegisteredWithExistingHooks(t *testing.T) {
	context := ldcontext.New("test-context")
	existingHook := sharedtest.NewTestPluginHook("test-existing-hook")
	pluginHook := sharedtest.NewTestPluginHook("test-plugin-hook")
	plugin := sharedtest.NewTestPlugin("test-plugin", []ldhooks.Hook{pluginHook})

	client, _ := MakeCustomClient("", Config{
		Offline: true,
		Hooks:   []ldhooks.Hook{existingHook},
		Plugins: []ldplugins.Plugin{plugin},
	}, 0)

	client.BoolVariation("test-flag", context, false)

	existingHook.ExpectBeforeEvaluationCalled(t)
	existingHook.ExpectAfterEvaluationCalled(t)
	pluginHook.ExpectBeforeEvaluationCalled(t)
	pluginHook.ExpectAfterEvaluationCalled(t)
}
