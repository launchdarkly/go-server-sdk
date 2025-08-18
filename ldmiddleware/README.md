LaunchDarkly HTTP Middleware
===============================

# ⛔️⛔️⛔️⛔️
> [!CAUTION]
> This library is a alpha version and should not be considered ready for production use while this message is visible.
# ☝️☝️☝️☝️☝️☝️

[![Actions Status](https://github.com/launchdarkly/go-server-sdk/actions/workflows/ldmiddleware-ci.yml/badge.svg?branch=v7)](https://github.com/launchdarkly/go-server-sdk/actions/workflows/ldmiddleware-ci.yml)

This package provides a set of HTTP middleware functions that speed up the process of instrumenting your application with LaunchDarkly.

## Usage

```go
import (
    ld "github.com/launchdarkly/go-server-sdk/v7"
    ldmiddleware "github.com/launchdarkly/go-server-sdk/v7/ldmiddleware"
    "net/http"
    "time"
    "github.com/gorilla/mux"
)

func main() {
    client, err := ld.MakeClient("your-sdk-key", 5*time.Second)
    if err != nil {
        log.Fatal(err)
    }

    // Add the LaunchDarkly middleware functions to your middleware chain.
    // The order of the middleware functions is important.
    // The AddScopedClientForRequest function must be called before the TrackTiming and TrackErrorResponses functions.

    r := mux.NewRouter()
    r.Use(ldmiddleware.AddScopedClientForRequest(client))
    r.Use(ldmiddleware.TrackTiming)
    r.Use(ldmiddleware.TrackErrorResponses)

    r.Handle("/", http.HandlerFunc(myHandler))
    http.ListenAndServe(":8080", r)
}

func myHandler(w http.ResponseWriter, r *http.Request) {
    // Thanks to `AddScopedClientForRequest`, a scoped client is available in the request context.
    // We can use it to evaluate feature flags, track analytics, etc. without having to provide the LaunchDarkly context.
    var enableBetaFeatures bool
    if client, ok := ld.GetScopedClient(r.Context()); ok {
        enableBetaFeatures = client.BoolVariation("your-feature-key", false)
    }

    if enableBetaFeatures {
        // Do something
    }

    w.WriteHeader(200)
}
```