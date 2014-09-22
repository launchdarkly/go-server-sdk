LaunchDarkly SDK for Go
===========================

Quick setup
-----------

1. Install the SDK with the `go` tool:

        go get github.com/launchdarkly/go-client

2. Import the LaunchDarkly client:

        import ld "github.com/launchdarkly/go-client"

2. Create a new LDClient with your API key:

        ld_client := ld.MakeClient("YOUR_API_KEY")

Your first feature flag
-----------------------

1. Create a new feature flag on your [dashboard](https://app.launchdarkly.com)
2. In your application code, use the feature's key to check wthether the flag is on for each user:

        key := "user@test.com"
        show_feature := ld_client.GetFlag("your.flag.key", ld.User{Key: &key,}, false)
        if (show_feature) {
            # application code to show the feature
        } else {
            # the code to run if the feature is off 
        }