package enforcement

import "strings"

const (
	DefaultTenant     = "default"
	DefaultTimeout    = 30
	AllowKey          = "allow"
	resultKey         = "result"
	mainPolicyPackage = "permit.root"
)

var mainPolicyPath = strings.Replace(mainPolicyPackage, ".", "/", -1)
