package enforcement

import "strings"

const (
	DefaultTenant  = "default"
	DefaultTimeout = 30
	AllowKey       = "allow"
)

const (
	reqMethod           = "POST"
	reqContentTypeKey   = "Content-Type"
	reqContentTypeValue = "application/json"
	reqAuthKey          = "Authorization"
)

type packageName string
type sidecarPath string

const (
	mainPolicyPackage packageName = "permit.root"
	bulkPolicyPackage packageName = "permit.bulk"
)

const (
	mainPolicy sidecarPath = "/allowed"
	bulkPolicy sidecarPath = "/allowed/bulk"
)

type checkOperationConfig struct {
	sidecarPath sidecarPath
	opaPath     string
}

var policyMap = map[packageName]checkOperationConfig{
	mainPolicyPackage: {
		sidecarPath: mainPolicy,
		opaPath:     strings.Replace(string(mainPolicyPackage), ".", "/", -1),
	},
	bulkPolicyPackage: {
		sidecarPath: bulkPolicy,
		opaPath:     strings.Replace(string(bulkPolicyPackage), ".", "/", -1),
	},
}
