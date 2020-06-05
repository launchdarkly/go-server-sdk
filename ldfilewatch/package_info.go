// Package ldfilewatch allows the LaunchDarkly client to read feature flag data from a file
// that will be automatically reloaded if the file changes.
//
// It should be used in conjunction with the ldfiledata package:
//
//     config := ld.Config{
//         DataSource: ldfiledata.DataSource().
//             FilePaths(filePaths).
//             Reloader(ldfilewatch.WatchFiles),
//     }
//
// The two packages are separate so as to avoid bringing additional dependencies for users who
// do not need automatic reloading.
package ldfilewatch
