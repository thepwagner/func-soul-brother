package az_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thepwagner/func-soul-brother/az"
	"github.com/thepwagner/func-soul-brother/flows"
)

func TestGenerateEntrypoint(t *testing.T) {
	entrypoint := az.GenerateEntrypoint("topSecret", "testToken", flows.LoadedFlow{
		Name: "test",
		Triggers: []flows.Trigger{
			{Event: "issue_comment"},
		},
		Steps: []flows.LoadedStep{
			{
				Name: "step1",
				Inputs: map[string]string{
					"my_cool_token": "${{ secrets.GITHUB_TOKEN }}",
				},
			},
		},
	})
	t.Log(entrypoint)
	assert.Contains(t, entrypoint, `process.env.INPUT_MY_COOL_TOKEN = "testToken";`)

}
