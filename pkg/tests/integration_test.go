package tests

import (
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"testing"
)

func TestIntegration(t *testing.T) {
	logger := zap.NewExample()
	permitContext := config.NewPermitContext(config.ProjectAPIKeyLevel, "project-id", "environment-id")
	permitClient := permit.New(config.NewConfigBuilder("permit_key_nfPgSYZLjOQ0kfWyiKFwvrN91Fzkg38zcdQ1SzyA9BUS73EjjWwcdFCkoBy4EwU96k4nboSsZviyHz9fIo1UIz", "http://localhost:7766").WithContext(permitContext).WithLogger(logger).Build())

	user := enforcement.UserBuilder("sandy@permit.io").Build()
	resource := enforcement.ResourceBuilder("folder").WithTenant("default").Build()
	allowed, err := permitClient.Check(user, "read", resource)
	if err != nil {
		t.Error(err)
	}

	assert.True(t, allowed)
}
