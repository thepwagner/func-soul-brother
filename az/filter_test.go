package az_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thepwagner/func-soul-brother/az"
	"github.com/thepwagner/func-soul-brother/flows"
)

func TestGenerateFilterFunction(t *testing.T) {
	cases := map[string]struct {
		Triggers []flows.Trigger
		Body     string
	}{
		"1 event": {
			Triggers: []flows.Trigger{{Event: "issue_comment"}},
			Body:     `if (req.headers['x-github-event'] === "issue_comment") { return true; }`,
		},
		">1 events": {
			Triggers: []flows.Trigger{{Event: "project"}, {Event: "project_card"}},
			Body: `if (req.headers['x-github-event'] === "project") { return true; }
			       if (req.headers['x-github-event'] === "project_card") { return true; }`,
		},
		"1 and 1 action": {
			Triggers: []flows.Trigger{{Event: "issue_comment", Actions: []string{"created"}}},
			Body: `if (req.headers['x-github-event'] === "issue_comment") {
			         switch (req.body.action) { case "created": return true; }
			       }`,
		},
		"1 and >1 actions": {
			Triggers: []flows.Trigger{{Event: "issue_comment", Actions: []string{"created", "edited"}}},
			Body: `if (req.headers['x-github-event'] === "issue_comment") {
			         switch (req.body.action) { case "created": case "edited": return true; }
			       }`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			body := az.GenerateFilterFunction(tc.Triggers)
			t.Log(body)
			assert.Contains(t, strings.Join(strings.Fields(body), ""),
				strings.Join(strings.Fields(tc.Body), ""))
		})
	}
}
