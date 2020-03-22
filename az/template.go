package az

import (
	"fmt"
	"strings"

	"github.com/thepwagner/func-soul-brother/flows"
)

var functionBindings = []byte(`{
  "bindings": [
    {
      "authLevel": "anonymous",
      "type": "httpTrigger",
      "direction": "in",
      "name": "req",
      "methods": [
        "get",
        "post"
      ]
    },
    {
      "type": "http",
      "direction": "out",
      "name": "res"
    }
  ]
}`)

func GenerateEntrypoint(secret, token string, flow flows.LoadedFlow) string {
	var s strings.Builder

	// Imports and constants:
	s.WriteString(`
const fs = require('fs');
const verify = require('@octokit/webhooks/verify');
`)
	_, _ = fmt.Fprintf(&s, "const secret = %q;\n", secret)

	// Function entrypoint, verify HMAC:
	s.WriteString(`
module.exports = async function (context, req) {
  if (!verify(secret, req.body, req.headers['x-hub-signature'])) {
    context.res = {
      status: 401,
      body: "Signature failed"
    };
    return;
  }

`)

	// Filter out events that don't trigger the workflow:
	for _, l := range strings.Split(GenerateFilterFunction(flow.Triggers), "\n") {
		_, _ = fmt.Fprintf(&s, "  %s\n", l)
	}
	s.WriteString(`
  if (!filterEvent(req)) {
    context.res = {
      status: 200,
      body: "Ignored event"
    };
    return;
  }
`)

	// HACK: ignore comments from actions for a smoother "reply to comments" demo
	s.WriteString(`
  if (req.body.comment && req.body.comment.user.login === 'github-actions[bot]') {
    context.res = {
      status: 200,
      body: "ignoring actions"
    };
    return;
  }
`)

	s.WriteString(`
  const eventFile = '/tmp/eventPayload.json';
  fs.writeFileSync(eventFile, JSON.stringify(req.body));
  process.env.GITHUB_EVENT_PATH = eventFile;  
  context.log('wrote event JSON, invoking action');
`)

	for _, f := range flow.Steps {
		_, _ = fmt.Fprintln(&s)
		for k, v := range f.Inputs {
			// HACK: replace identifier, so race in demo is clear:
			if k == "id" && v == "Cloud" {
				v = "Azure Functions"
			}

			// Replace token with provided PAT
			switch strings.Join(strings.Fields(v), "") {
			case "${{secrets.GITHUB_TOKEN}}":
				v = token
			}
			_, _ = fmt.Fprintf(&s, "  process.env.INPUT_%s = %q;\n", strings.ToUpper(k), v)
		}

		fn := f.Filename()
		_, _ = fmt.Fprintf(&s, "  delete require.cache[require.resolve('../%s')];\n", fn)
		_, _ = fmt.Fprintf(&s, "  await require('../%s');\n", fn)
	}
	s.WriteString(`
  context.res = {
    status: 200,
    body: "subprocess complete"
  };
};
`)
	return s.String()
}
