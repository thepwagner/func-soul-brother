package main

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/thepwagner/func-soul-brother/flows"
)

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	ctx := context.Background()

	// Query target repo for workflows
	token := os.Getenv("GITHUB_TOKEN")
	loader := flows.NewLoader(ctx, token)

	nwo := flows.NWO{
		Owner: "thepwagner",
		Name:  "echo-chamber",
	}

	err := loader.Load(ctx, nwo)
	if err != nil {
		panic(err)
	}

}
