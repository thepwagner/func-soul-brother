package az

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-06-01/storage"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/thepwagner/func-soul-brother/flows"

	"github.com/sirupsen/logrus"
)

type FunctionUploader struct {
	deploys           resources.DeploymentsClient
	resourceGroupName string
	storage           storage.AccountsClient
}

func NewFunctionUploader(subscriptionID, resourceGroupName string) (*FunctionUploader, error) {
	logrus.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"rg_name":         resourceGroupName,
	}).Debug("Initializing uploader")

	// FIXME: real authorizer
	const resource = "https://management.core.windows.net/"
	authorizer, err := auth.NewAuthorizerFromCLIWithResource(resource)
	if err != nil {
		return nil, fmt.Errorf("initializing authorizer: %w", err)
	}

	deploys := resources.NewDeploymentsClient(subscriptionID)
	deploys.Authorizer = authorizer
	if err := deploys.AddToUserAgent("func-soul-brother-1.0.0"); err != nil {
		return nil, fmt.Errorf("configuring user agent: %w", err)
	}
	deploys.PollingDelay = 1 * time.Second

	storageAccounts := storage.NewAccountsClient(subscriptionID)
	storageAccounts.Authorizer = authorizer

	return &FunctionUploader{
		deploys:           deploys,
		resourceGroupName: resourceGroupName,
		storage:           storageAccounts,
	}, nil
}

func (f *FunctionUploader) Upload(ctx context.Context, flow *flows.LoadedFlow) error {
	deploymentName := strings.ReplaceAll(flow.Name, "-", "")
	deploymentName = strings.ReplaceAll(deploymentName, ".", "")
	deploymentName = fmt.Sprintf("dsp%s", deploymentName)

	blobURL, err := f.uploadCode(ctx, deploymentName)
	if err != nil {
		return fmt.Errorf("uploading code: %w", err)
	}
	if err := f.deployFunction(ctx, deploymentName, flow.Name, blobURL); err != nil {
		return fmt.Errorf("deploying function: %w", err)
	}
	return nil
}

func (f *FunctionUploader) deployFunction(ctx context.Context, deploymentName, workflowName, blobURL string) error {
	deployLogger := logrus.WithField("deployment", deploymentName)
	deployLogger.Debug("Updating function deployment...")

	// FIXME: sync.Once
	tmpl := make(map[string]interface{})
	if err := json.Unmarshal([]byte(template), &tmpl); err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}
	update, err := f.deploys.CreateOrUpdate(ctx, f.resourceGroupName, deploymentName, resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Template: &tmpl,
			Parameters: map[string]interface{}{
				"appName": map[string]interface{}{
					"value": workflowName,
				},
				"cleanAppName": map[string]interface{}{
					"value": deploymentName,
				},
				"blobURL": map[string]interface{}{
					"value": blobURL,
				},
			},
			Mode: resources.Incremental,
		},
	})
	if err != nil {
		return fmt.Errorf("updating deployment: %w", err)
	}
	if err := update.WaitForCompletionRef(ctx, f.deploys.Client); err != nil {
		return fmt.Errorf("waiting for deployment: %w", err)
	}
	result, err := update.Result(f.deploys)
	if err != nil {
		return fmt.Errorf("getting result: %w", err)
	}
	deployLogger.WithField("deploy_id", *result.ID).Info("Check it out now")
	return nil
}

func (f *FunctionUploader) uploadCode(ctx context.Context, storageAccountName string) (string, error) {
	codeZip, err := packageFunctionZip()
	if err != nil {
		return "", err
	}

	keys, err := f.storage.ListKeys(ctx, f.resourceGroupName, storageAccountName, "")
	if err != nil {
		return "", err
	}
	key := *(((*keys.Keys)[0]).Value)

	u, err := url.Parse(fmt.Sprintf(`https://%s.blob.core.windows.net`, storageAccountName))
	if err != nil {
		return "", fmt.Errorf("parsing url: %w", err)
	}
	c, _ := azblob.NewSharedKeyCredential(storageAccountName, key)
	service := azblob.NewServiceURL(*u, azblob.NewPipeline(c, azblob.PipelineOptions{}))
	const containerName = "azureappservice-run-from-package"
	containerURL := service.NewContainerURL(containerName)
	//_, err = containerURL.Create(ctx, azblob.Metadata{}, azblob.PublicAccessNone)
	//if err != nil {
	//	return fmt.Errorf("creating container: %w", err)
	//}

	blobName := time.Now().Format(time.RFC3339)
	blobURL := containerURL.NewBlockBlobURL(blobName)
	logrus.WithField("blob_url", blobURL.String()).Debug("uploading zip package")
	_, err = blobURL.Upload(ctx, bytes.NewReader(codeZip),
		azblob.BlobHTTPHeaders{
			ContentType: "application/zip",
		},
		azblob.Metadata{},
		azblob.BlobAccessConditions{},
	)
	if err != nil {
		return "", fmt.Errorf("uploading zip: %w", err)
	}

	sasQueryParams, err := azblob.BlobSASSignatureValues{
		Protocol:      azblob.SASProtocolHTTPS,
		ExpiryTime:    time.Now().UTC().Add(48 * time.Hour),
		ContainerName: containerName,
		BlobName:      blobName,
		Permissions:   azblob.BlobSASPermissions{Read: true}.String(),
	}.NewSASQueryParameters(c)
	qp := sasQueryParams.Encode()
	signedURL := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s?%s",
		storageAccountName, containerName, blobName, qp)
	return signedURL, nil
}

func packageFunctionZip() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	dotFuncIgnore, err := zw.Create(".funcignore")
	if err != nil {
		return nil, err
	}
	if _, err := dotFuncIgnore.Write(funcIgnore); err != nil {
		return nil, err
	}

	functionJSON, err := zw.Create("FuncSoulBrother/function.json")
	if err != nil {
		return nil, err
	}
	if _, err := functionJSON.Write(functionBindings); err != nil {
		return nil, err
	}

	indexJS, err := zw.Create("FuncSoulBrother/index.js")
	if err != nil {
		return nil, err
	}

	index := []byte(fmt.Sprintf(entryPoint, os.Getenv("GITHUB_TOKEN")))
	if _, err := indexJS.Write(index); err != nil {
		return nil, err
	}

	if err := addFile(zw, "host.json"); err != nil {
		return nil, err
	}

	if err := addFile(zw, "package.json"); err != nil {
		return nil, err
	}
	if err := addFile(zw, "package-lock.json"); err != nil {
		return nil, err
	}
	modulesWalkErr := filepath.Walk("node_modules", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		return addFile(zw, path)
	})
	if modulesWalkErr != nil {
		return nil, modulesWalkErr
	}

	// FIXME: from scanned repo, not hardcode:
	if err := addFile(zw, "echo-timer.js"); err != nil {
		return nil, err
	}

	proxiesJSON, err := zw.Create("proxies.json")
	if err != nil {
		return nil, err
	}
	if _, err := proxiesJSON.Write(proxiesConfig); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("closing zip: %w", err)
	}
	return buf.Bytes(), nil
}

func addFile(zw *zip.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	zf, err := zw.Create(path)
	if err != nil {
		return err
	}
	_, err = io.Copy(zf, f)
	return err
}
