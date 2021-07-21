// Package bigsegments is an internal package containing implementation details for the SDK's big
// segment functionality, not including the part that is in go-server-sdk-evaluation. These are
// not visible from outside of the SDK.
//
// This does not include implementations of specific big segment store integrations such as Redis.
// Those are implemented in separate repositories such as
// https://github.com/launchdarkly/go-server-sdk-redis-redigo.
package bigsegments
