package main

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/thepwagner/func-soul-brother/az"
	"github.com/thepwagner/func-soul-brother/flows"
)

func main() {
	logrus.SetLevel(logrus.DebugLevel)

	// Target repository to convert:
	const (
		owner           = "thepwagner"
		name            = "echo-chamber"
		azResourceGroup = "funcsoulbrother"
	)
	azSubscriptionID := os.Getenv("AZ_SUBSCRIPTION")
	ghToken := os.Getenv("GITHUB_TOKEN")
	webhookSecret := os.Getenv("WEBHOOK_SECRET")

	loader := flows.NewLoader(flows.WithToken(ghToken))

	// Query target repo for workflows
	ctx := context.Background()
	loaded, err := loader.Load(ctx, owner, name)
	if err != nil {
		logrus.WithError(err).Fatal("Loading repo workflows")
	}
	if len(loaded) == 0 {
		logrus.Fatal("No convertible flows found")
	}
	logrus.WithField("flows", len(loaded)).Info("Loaded flows")

	uploader, err := az.NewFunctionUploader(azSubscriptionID, azResourceGroup, webhookSecret, ghToken)
	if err != nil {
		logrus.WithError(err).Fatal("Preparing function uploader")
	}
	for _, flow := range loaded {
		// TODO: receive a endpoint, configure the repo webhook according to flow.Triggers
		if err := uploader.Upload(ctx, flow); err != nil {
			logrus.WithError(err).Error("Uploading workflow")
		}
	}
}
