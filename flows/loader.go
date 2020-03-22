package flows

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/go-github/v30/github"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
)

const (
	// Where in repositories do actions live?
	actionsPath = ".github/workflows"
	// Where in repositories does actions metadata live?
	actionsMetadataFile = "action.yml"
)

type Loader struct {
	ghPublic *github.Client
	client   *http.Client

	token          string
	ghPrivateSetup sync.Once
	ghPrivate      *github.Client

	jsStepMu sync.Mutex
	jsSteps  map[string]Action
}

func NewLoader(opts ...Opt) *Loader {
	l := &Loader{
		client:  http.DefaultClient,
		jsSteps: make(map[string]Action),
	}
	for _, opt := range opts {
		opt(l)
	}
	l.ghPublic = github.NewClient(l.client)
	return l
}

type Opt func(*Loader)

func WithToken(token string) Opt {
	return func(l *Loader) {
		l.token = token
	}
}

// LoadedFlow is a .yaml workflow that can be ported to AzureFunctions.
type LoadedFlow struct {
	Name     string
	Triggers []Trigger
	Steps    []LoadedStep
}

// Trigger is an event that triggers a workflow, from the YAML `on:`.
type Trigger struct {
	Event   string
	Actions []string
}

// LoadedStep is a step of a LoadedFlow
type LoadedStep struct {
	Name       string
	SourceCode string
	Inputs     map[string]string
}

func (l *Loader) Load(ctx context.Context, owner, name string) ([]LoadedFlow, error) {
	// List the actions directory to detect workflows:
	logger := logrus.WithField("repo", fmt.Sprintf("%s/%s", owner, name))
	logger.WithField("path", actionsPath).Debug("Listing workflows...")
	_, listing, _, err := l.ghPrivateClient(ctx).Repositories.GetContents(ctx, owner, name, actionsPath, &github.RepositoryContentGetOptions{})
	if err != nil {
		return nil, fmt.Errorf("fetching workflows: %w", err)
	}
	logger.WithField("workflows", len(listing)).Debug("Listed workflows")

	// Attempt to load each workflow:
	var jobs []LoadedFlow
	for _, wf := range listing {
		loaded, err := l.loadWorkflow(ctx, logger, wf)
		if err != nil {
			return nil, fmt.Errorf("loading workflow %q: %w", *wf.Path, err)
		}
		if loaded != nil {
			jobs = append(jobs, *loaded)
		}
	}
	return jobs, nil
}

func (l *Loader) ghPrivateClient(ctx context.Context) *github.Client {
	l.ghPrivateSetup.Do(func() {
		if l.token == "" {
			l.ghPrivate = l.ghPublic
			return
		}
		clientCtx := context.WithValue(ctx, oauth2.HTTPClient, l.client)
		l.ghPrivate = github.NewClient(
			oauth2.NewClient(clientCtx,
				oauth2.StaticTokenSource(&oauth2.Token{AccessToken: l.token}),
			))
	})
	return l.ghPrivate
}

func (l *Loader) loadWorkflow(ctx context.Context, logger logrus.FieldLogger, wf *github.RepositoryContent) (*LoadedFlow, error) {
	wfName := wf.GetName()
	logger = logrus.WithField("workflow", wfName)
	logger.Debug("Fetching workflow...")
	resp, err := l.client.Get(*wf.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("fetching workflow %q: %w", wfName, err)
	}
	defer resp.Body.Close()

	var flow Workflow
	if err := yaml.NewDecoder(resp.Body).Decode(&flow); err != nil {
		return nil, fmt.Errorf("decoding workflow %q: %w", wfName, err)
	}
	logger.Debug("Fetched and parsed workflow")

	var ls []LoadedStep
	for jobName, job := range flow.Jobs {
		jobLogger := logger.WithField("job", jobName)
		for stepIndex, step := range job.Steps {
			stepLogger := jobLogger.WithField("step", stepIndex)
			action, err := l.fetchActionYAML(ctx, step.Uses)
			if err != nil {
				return nil, fmt.Errorf("loading action metadata %q: %w", step.Uses, err)
			}
			if !action.FunctionCompatible() {
				stepLogger.Info("Step is not compatible, skipping workflow")
				return nil, nil
			}
			stepLogger.Debug("Compatible step detected")

			inputs := map[string]string{}
			for k, v := range step.With {
				if strings.Contains(v, "${{") && !strings.Contains(v, "secrets.GITHUB_TOKEN") {
					stepLogger.Info("Step uses interpolation, skipping workflow")
					return nil, nil
				}
				inputs[k] = v
			}

			ls = append(ls, LoadedStep{
				Name:       fmt.Sprintf("%s-%d", jobName, stepIndex),
				SourceCode: action.SourceCode,
				Inputs:     step.With,
			})
		}
		jobLogger.Info("Node workflow detected, converting...")
	}

	f := &LoadedFlow{
		Name:  filepath.Base(*wf.Path),
		Steps: ls,
	}

	switch on := flow.On.(type) {
	case string:
		f.Triggers = append(f.Triggers, Trigger{Event: on})
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
						return nil, fmt.Errorf("unexpected `types` type: %T", flow.On)
					}
				}
			}

			f.Triggers = append(f.Triggers, trigger)
		}
	default:
		return nil, fmt.Errorf("unexpected `on` type: %T", flow.On)
	}
	return f, nil
}

func (l *Loader) IsNodeStep(ctx context.Context, step Step) (bool, error) {
	logrus.WithField("uses", step.Uses).Debug("Detecting node step...")
	action, err := l.fetchActionYAML(ctx, step.Uses)
	if err != nil {
		return false, err
	}
	return action.FunctionCompatible(), nil
}

func (l *Loader) fetchActionYAML(ctx context.Context, uses string) (Action, error) {
	// Have we checked this step before?
	l.jsStepMu.Lock()
	defer l.jsStepMu.Unlock()
	if stored, cached := l.jsSteps[uses]; cached {
		return stored, nil
	}

	ref, ok := ParseActionReference(uses)
	if !ok {
		return Action{}, nil
	}

	ghClient := l.ghPublic
	contentsResp, _, _, err := ghClient.Repositories.GetContents(ctx, ref.RepoOwner, ref.RepoName, actionsMetadataFile, &github.RepositoryContentGetOptions{Ref: ref.Ref})
	if err != nil {
		return Action{}, fmt.Errorf("fetching action metadata: %w", err)
	}
	contents, err := contentsResp.GetContent()
	if err != nil {
		return Action{}, fmt.Errorf("decoding action metadata contents: %w", err)
	}
	var action Action
	if err := yaml.Unmarshal([]byte(contents), &action); err != nil {
		return Action{}, fmt.Errorf("decoding action metadata: %w", err)
	}

	if action.FunctionCompatible() {
		contentsResp, _, _, err := ghClient.Repositories.GetContents(ctx, ref.RepoOwner, ref.RepoName, action.Runs.Main, &github.RepositoryContentGetOptions{Ref: ref.Ref})
		if err != nil {
			return Action{}, fmt.Errorf("fetching action metadata: %w", err)
		}
		contents, err := contentsResp.GetContent()
		if err != nil {
			return Action{}, fmt.Errorf("decoding action metadata contents: %w", err)
		}
		action.SourceCode = contents
	}

	l.jsSteps[uses] = action
	return action, nil
}

func (s LoadedStep) Filename() string {
	hash := sha256.Sum256([]byte(s.SourceCode))
	return hex.EncodeToString(hash[:])
}
