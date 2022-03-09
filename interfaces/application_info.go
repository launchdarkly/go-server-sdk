package interfaces

// ApplicationInfo allows configuration of application metadata.
//
// If you want to set non-default values for any of these fields, set the ApplicationInfo field
// in the SDK's Config struct.
type ApplicationInfo struct {
	// ApplicationID is a string that will be sent to LaunchDarkly to identify activity by this application
	// as distinct from other applications using the same account.
	//
	// This can be any value as long as it only uses the following characters: ASCII letters, ASCII digits,
	// period, hyphen, underscore. A string containing any other characters will be ignored.
	ApplicationID string

	// ApplicationVersion is a string that will be sent to LaunchDarkly to identify the version of this
	// application.
	//
	// This can be any value as long as it only uses the following characters: ASCII letters, ASCII digits,
	// period, hyphen, underscore. A string containing any other characters will be ignored.
	ApplicationVersion string
}
