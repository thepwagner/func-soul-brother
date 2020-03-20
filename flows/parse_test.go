package flows_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thepwagner/func-soul-brother/flows"
	"gopkg.in/yaml.v2"
)

func TestDecode(t *testing.T) {
	const data = `
on:
  issue_comment:
    types: created
jobs:
  echo-timer:
    name: Cloud Echo Timer
    runs-on: ubuntu-latest
    steps:
      - name: Echo timer
        uses: thepwagner/echo-timer@master
        with:
          id: Cloud
          token: ${{ secrets.GITHUB_TOKEN }}
`

	var wf flows.Workflow
	err := yaml.NewDecoder(strings.NewReader(data)).Decode(&wf)
	require.NoError(t, err)

	assert.Len(t, wf.Jobs, 1)
	require.Contains(t, wf.Jobs, "echo-timer")

	job := wf.Jobs["echo-timer"]
	if assert.Len(t, job.Steps, 1) {
		step1 := job.Steps[0]
		assert.Equal(t, "thepwagner/echo-timer@master", step1.Uses)
	}
}

func TestParseActionReference(t *testing.T) {
	cases := []struct {
		uses     string
		expected *flows.ActionReference
	}{
		{
			uses:     "",
			expected: nil,
		},
		{
			uses: "thepwagner/echo-timer@master",
			expected: &flows.ActionReference{
				RepoOwner: "thepwagner",
				RepoName:  "echo-timer",
				Ref:       "master",
			},
		},
		{
			uses: "actions/labeler@v2.1.0",
			expected: &flows.ActionReference{
				RepoOwner: "actions",
				RepoName:  "labeler",
				Ref:       "v2.1.0",
			},
		},
	}

	for _, tc := range cases {
		actual, ok := flows.ParseActionReference(tc.uses)
		if tc.expected == nil {
			assert.False(t, ok)
			continue
		}
		assert.Equal(t, tc.expected.RepoOwner, actual.RepoOwner)
		assert.Equal(t, tc.expected.RepoName, actual.RepoName)
		assert.Equal(t, tc.expected.Ref, actual.Ref)
	}
}
