// The example "Init the SDK" of the README of v1.3.0. Each line that uses v1 ends with a comment
// "v1:" that lists what the scanner must report on it.
package main

import "github.com/permitio/permit-golang/pkg/permit" // v1: github.com/permitio/permit-golang/pkg/permit
import "github.com/permitio/permit-golang/pkg/config" // v1: github.com/permitio/permit-golang/pkg/config

func main() {
	PermitConfig := config.NewConfigBuilder("<YOUR_API_TOKEN>").Build() // v1: config.NewConfigBuilder config.PermitConfig.Build
	Permit := permit.New(PermitConfig)                                  // v1: permit.New
	_ = Permit                                                          // call the methods shown below on Permit
}
