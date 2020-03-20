package flows

import (
	"fmt"
	"regexp"
	"strings"
)

type Workflow struct {
	On   interface{}     `yaml:"on"`
	Jobs map[string]*Job `yaml:"jobs"`
}

type Job struct {
	Steps []Step `yaml:"steps"`
}

type Step struct {
	Uses string            `yaml:"uses"`
	With map[string]string `yaml:"with"`
}

type Action struct {
	Runs       Runs   `yaml:"runs"`
	SourceCode string `yaml:"-"`
}

type Runs struct {
	Using string `yaml:"using"`
	Main  string `yaml:"main"`
}

func (w Workflow) Triggers() ([]Trigger, error) {
	var t []Trigger
	switch on := w.On.(type) {
	case string:
		t = append(t, Trigger{Event: on})
	case map[interface{}]interface{}:
		for event, details := range on {
			trigger := Trigger{Event: event.(string)}
			if detailsMap, ok := details.(map[interface{}]interface{}); ok {
				if types, ok := detailsMap["types"]; ok {
					switch actions := types.(type) {
					case string:
						trigger.Actions = []string{actions}
					case []string:
						trigger.Actions = actions
					default:
						return nil, fmt.Errorf("unexpected `types` type: %T", actions)
					}
				}
			}
			t = append(t, trigger)
		}
	default:
		return nil, fmt.Errorf("unexpected `on` type: %T", w.On)
	}
	return t, nil
}

func (a Action) FunctionCompatible() bool {
	return a.Runs.Using == "node12" && strings.HasPrefix(a.Runs.Main, "dist/")
}

var usesRe = regexp.MustCompile("([a-z]*)/([a-z-]*)@([a-z0-9\\.\\-]*)")

type ActionReference struct {
	RepoOwner string
	RepoName  string
	Ref       string
}

func ParseActionReference(stepUses string) (ActionReference, bool) {
	// TODO: non-root paths
	// TODO: relative path in this repo
	match := usesRe.FindStringSubmatch(stepUses)
	if len(match) == 0 {
		return ActionReference{}, false
	}
	return ActionReference{
		RepoOwner: match[1],
		RepoName:  match[2],
		Ref:       match[3],
	}, true
}
