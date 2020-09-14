// Package ldfiledata allows the LaunchDarkly client to read feature flag data from a file.
//
// This is different from ldtestdata.DataSource, which allows you to simulate flag configurations
// programmatically rather than using a file.
//
// To use the file-based data source in your SDK configuration, call ldfiledata.DataSource to obtain a
// configurable object that you will use as the configuration's DataSource:
//
//     config := ld.Config{
//         DataSource: ldfiledata.DataSource()
//             .FilePaths("./test-data/my-flags.json"),
//     }
//     client := ld.MakeCustomClient(mySdkKey, config, 5*time.Second)
//
// Use FilePaths to specify any number of file paths. The files are not actually loaded until the
// client starts up. At that point, if any file does not exist or cannot be parsed, the data source
// will log an error and will not load any data.
//
// Files may contain either JSON or YAML; if the first non-whitespace character is '{', the file is parsed
// as JSON, otherwise it is parsed as YAML. The file data should consist of an object with up to three
// properties:
//
// - "flags": Feature flag definitions.
//
// - "flagValues": Simplified feature flags that contain only a value.
//
// - "segments": User segment definitions.
//
// The format of the data in "flags" and "segments" is defined by the LaunchDarkly application and is
// subject to change. Rather than trying to construct these objects yourself, it is simpler to request
// existing flags directly from the LaunchDarkly server in JSON format, and use this output as the starting
// point for your file. In Linux you would do this:
//
//     curl -H "Authorization: <your sdk key>" https://app.launchdarkly.com/sdk/latest-all
//
// The output will look something like this (but with many more properties):
//
//     {
//       "flags": {
//         "flag-key-1": {
//           "key": "flag-key-1",
//           "on": true,
//           "variations": [ "a", "b" ]
//         }
//       },
//       "segments": {
//         "segment-key-1": {
//           "key": "segment-key-1",
//           "includes": [ "user-key-1" ]
//         }
//       }
//     }
//
// Data in this format allows the SDK to exactly duplicate all the kinds of flag behavior supported by
// LaunchDarkly. However, in many cases you will not need this complexity, but will just want to set
// specific flag keys to specific values. For that, you can use a much simpler format:
//
//     {
//       "flagValues": {
//         "my-string-flag-key": "value-1",
//         "my-boolean-flag-key": true,
//         "my-integer-flag-key": 3
//       }
//     }
//
// Or, in YAML:
//
//     flagValues:
//       my-string-flag-key: "value-1"
//       my-boolean-flag-key: true
//       my-integer-flag-key: 3
//
// It is also possible to specify both "flags" and "flagValues", if you want some flags to have simple
// values and others to have complex behavior. However, it is an error to use the same flag key or
// segment key more than once, either in a single file or across multiple files.
//
// If the data source encounters any error in any file-- malformed content, a missing file, or a
// duplicate key-- it will not load flags from any of the files.
package ldfiledata
