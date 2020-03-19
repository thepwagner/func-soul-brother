package flows_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thepwagner/func-soul-brother/flows"
	"gopkg.in/yaml.v2"
)

const data = `on:
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
          token: ${{ secrets.GITHUB_TOKEN }}`

func TestDecode(t *testing.T) {
	var wf flows.Workflow
	err := yaml.NewDecoder(strings.NewReader(data)).Decode(&wf)
	require.NoError(t, err)

	require.Contains(t, wf.Jobs, "echo-timer")
	job := wf.Jobs["echo-timer"]
	assert.Len(t, job.Steps, 1)

	step1 := job.Steps[0]
	assert.Equal(t, "thepwagner/echo-timer@master", step1.Uses)
}

func TestLoader_IsNodeStep(t *testing.T) {
	ctx := context.Background()
	l := flows.NewLoader(ctx, "")

	node, err := l.IsNodeStep(ctx, flows.Step{Uses: "thepwagner/echo-timer@master"})
	require.NoError(t, err)
	assert.True(t, node)
}
