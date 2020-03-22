package az

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAzureResourcesTemplate(t *testing.T) {
	if assert.Contains(t, azureResourcesTemplate, "$schema") {
		assert.Equal(t, "https://schema.management.azure.com/schemas/2015-01-01/deploymentTemplate.json#", azureResourcesTemplate["$schema"])
	}
}
