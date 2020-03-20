package flows_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thepwagner/func-soul-brother/flows"
)

func TestLoader_Load(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")

	l := flows.NewLoader(flows.WithToken(token))

	ctx := context.Background()
	jobs, err := l.Load(ctx, "thepwagner", "echo-chamber")
	require.NoError(t, err)

	if assert.Len(t, jobs, 1) {
		job1 := jobs[0]
		assert.Equal(t, job1.Name, "cloud.yaml")
		assert.Equal(t, job1.Triggers, []flows.Trigger{
			{Event: "issue_comment", Actions: []string{"created"}},
		})
		if assert.Len(t, job1.Steps, 1) {
			step1 := job1.Steps[0]
			assert.Equal(t, "echo-timer-0", step1.Name)
			assert.Equal(t, map[string]string{
				"id":    "Cloud",
				"token": "${{ secrets.GITHUB_TOKEN }}",
			}, step1.Inputs)
			assert.True(t, len(step1.SourceCode) > 1024)
		}
	}
}

func TestLoader_IsNodeStep(t *testing.T) {
	if os.Getenv("GITHUB_API_TESTS") == "" {
		t.Skip("skipping test that hits GitHub API, set GITHUB_API_TESTS")
	}

	ctx := context.Background()
	l := flows.NewLoader()

	cases := []struct {
		uses string
		node bool
	}{
		{uses: "thepwagner/echo-timer@master", node: true},
		// cached:
		{uses: "thepwagner/echo-timer@master", node: true},

		// potentially useful actions:
		{uses: "actions/labeler@v3-preview", node: true},
		{uses: "actions/github-script@0.8.0", node: true},

		// node, but not ncc:
		{uses: "actions/labeler@v2.1.0", node: false},
	}

	for _, tc := range cases {
		node, err := l.IsNodeStep(ctx, flows.Step{Uses: tc.uses})
		require.NoError(t, err)
		assert.Equal(t, tc.node, node, tc.uses)
	}
}
