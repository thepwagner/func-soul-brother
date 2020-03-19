package flows

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sync"

	"github.com/google/go-github/v30/github"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
)

const actionsPath = ".github/workflows"

type NWO struct {
	Owner, Name string
}

type Loader struct {
	ghPrivate *github.Client
	ghPublic  *github.Client
	client    *http.Client

	jsStepMu sync.Mutex
	jsSteps  map[string]bool
}

func NewLoader(ctx context.Context, token string) *Loader {
	var ghClient *http.Client
	if token != "" {
		ghClient = oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
	}

	return &Loader{
		ghPrivate: github.NewClient(ghClient),
		ghPublic:  github.NewClient(nil),
		client:    http.DefaultClient,
		jsSteps:   make(map[string]bool),
	}
}

func (l *Loader) Load(ctx context.Context, nwo NWO) error {
	// List the actions directory to detect workflows:
	logger := logrus.WithField("repo", fmt.Sprintf("%s/%s", nwo.Owner, nwo.Name))
	logger.WithField("path", actionsPath).Debug("Listing workflows...")
	_, listing, _, err := l.ghPrivate.Repositories.GetContents(ctx, nwo.Owner, nwo.Name, actionsPath, &github.RepositoryContentGetOptions{})
	if err != nil {
		return fmt.Errorf("fetching workflows: %w", err)
	}
	logger.WithField("workflows", len(listing)).Debug("Listed workflows")

	// Attempt to load each workflow:
	for _, wf := range listing {
		if err := l.loadWorkflow(ctx, logger, wf); err != nil {
			return err
		}
	}
	return nil
}

type Workflow struct {
	Jobs map[string]*Job `yaml:"jobs"`
}

type Job struct {
	Steps []Step `yaml:"steps"`
}

type Step struct {
	Uses string `yaml:"uses"`
}

type Action struct {
	Runs Runs `yaml:"runs"`
}

type Runs struct {
	Using string `yaml:"using"`
}

func (l *Loader) loadWorkflow(ctx context.Context, logger logrus.FieldLogger, wf *github.RepositoryContent) error {
	wfName := wf.GetName()
	logger = logrus.WithField("workflow", wfName)
	logger.Debug("Fetching workflow...")
	resp, err := l.client.Get(*wf.DownloadURL)
	if err != nil {
		return fmt.Errorf("fetching workflow %q: %w", wfName, err)
	}
	defer resp.Body.Close()

	var flow Workflow
	if err := yaml.NewDecoder(resp.Body).Decode(&flow); err != nil {
		return fmt.Errorf("decoding workflow %q: %w", wfName, err)
	}
	logger.Debug("Fetched and parsed workflow")

	for jobName, job := range flow.Jobs {
		jobLogger := logger.WithField("job", jobName)
		for stepIndex, step := range job.Steps {
			stepLogger := jobLogger.WithField("step", stepIndex)
			node, err := l.IsNodeStep(ctx, step)
			if err != nil {
				return fmt.Errorf("checking node step %s@%d: %w", jobName, stepIndex, err)
			}
			stepLogger.WithField("node", node).Debug("Parsed step")
			if !node {
				stepLogger.Info("Step is not node, skipping workflow")
				return nil
			}
		}
		jobLogger.Info("Node workflow detected, converting...")
	}
	return nil
}

var usesRe = regexp.MustCompile("([a-z]*)/([a-z-]*)@([a-z0-9]*)")

func (l *Loader) IsNodeStep(ctx context.Context, step Step) (bool, error) {
	l.jsStepMu.Lock()
	defer l.jsStepMu.Unlock()
	if js, cached := l.jsSteps[step.Uses]; cached {
		return js, nil
	}

	logrus.WithField("uses", step.Uses).Debug("Detecting node step...")
	// TODO: non-root paths
	// TODO: relative path in this repo
	match := usesRe.FindStringSubmatch(step.Uses)
	if len(match) == 0 {
		return false, nil
	}
	owner := match[1]
	name := match[2]
	ref := match[3]

	contents, _, _, err := l.ghPublic.Repositories.GetContents(ctx, owner, name, "action.yml", &github.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		return false, fmt.Errorf("fetching action.yml %q: %w", step.Uses, err)
	}
	actionsYAML, err := contents.GetContent()
	if err != nil {
		return false, fmt.Errorf("decoding action.yml contents %q: %w", step.Uses, err)
	}
	var action Action
	if err := yaml.Unmarshal([]byte(actionsYAML), &action); err != nil {
		return false, fmt.Errorf("decoding action.yml %q: %w", step.Uses, err)
	}

	nodeAction := action.Runs.Using == "node12"
	l.jsSteps[step.Uses] = nodeAction
	return nodeAction, nil
}
