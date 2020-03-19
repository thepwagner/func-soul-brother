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
	ctx := context.Background()

	// Target repository to convert:
	nwo := flows.NWO{
		Owner: "thepwagner",
		Name:  "echo-chamber",
	}
	azSubscriptionID := os.Getenv("AZ_SUBSCRIPTION")
	azResourceGroup := "funcsoulbrother"
	ghToken := os.Getenv("GITHUB_TOKEN")

	// Query target repo for workflows
	loader := flows.NewLoader(ctx, ghToken)
	err := loader.Load(ctx, nwo)
	if err != nil {
		panic(err)
	}

	uploader, err := az.NewFunctionUploader(azSubscriptionID, azResourceGroup)
	if err != nil {
		panic(err)
	}
	wfName := "fsb-cloud.yaml"
	if err := uploader.Upload(ctx, wfName); err != nil {
		panic(err)
	}
}
