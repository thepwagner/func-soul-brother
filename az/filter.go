package az

import (
	"fmt"
	"strings"

	"github.com/thepwagner/func-soul-brother/flows"
)

func GenerateFilterFunction(triggers []flows.Trigger) string {
	var s strings.Builder
	s.WriteString("const filterEvent = (req) => {\n")
	for _, t := range triggers {
		_, _ = fmt.Fprintf(&s, "  if (req.headers['x-github-event'] === %q) {\n", t.Event)
		if len(t.Actions) == 0 {
			s.WriteString("    return true;\n")
		} else {
			s.WriteString("    switch (req.body.action) {\n")
			for _, a := range t.Actions {
				_, _ = fmt.Fprintf(&s, "      case %q:\n", a)
			}
			s.WriteString("        return true;\n")
			s.WriteString("    }\n")
		}
		s.WriteString("  }\n")
	}

	s.WriteString("  return false;\n")
	s.WriteString("};\n")
	return s.String()
}
