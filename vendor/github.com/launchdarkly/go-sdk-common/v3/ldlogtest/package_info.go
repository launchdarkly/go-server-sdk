// Package ldlogtest contains test helpers for use with [ldlog].
//
// This package provides the [MockLog] type, which allows you to capture output that is sent to the
// ldlog.Loggers API. This can be useful in test code for verifying that some component produces the
// log output you expect it to. It is separate from the ldlog package because production code
// normally will not use this tool.
package ldlogtest
