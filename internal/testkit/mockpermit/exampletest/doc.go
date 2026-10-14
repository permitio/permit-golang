// Package exampletest checks that mockpermit.StartExample serves the Examples of a package, the
// way the SDK's packages use it: its tests are an Example that sends requests to the servers, and
// a TestMain that starts them and runs the tests through Run. It is for tests only.
package exampletest
