package az

import (
	"archive/zip"
	"bytes"
	"context"
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
	webhookSecret     string
	githubToken       string
	storage           storage.AccountsClient
}

func NewFunctionUploader(subscriptionID, resourceGroupName, webhookSecret, githubToken string) (*FunctionUploader, error) {
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
		webhookSecret:     webhookSecret,
		githubToken:       githubToken,
	}, nil
}

func (f *FunctionUploader) Upload(ctx context.Context, flow flows.LoadedFlow) error {
	deploymentName := strings.ReplaceAll(flow.Name, "-", "")
	deploymentName = strings.ReplaceAll(deploymentName, ".", "")
	deploymentName = fmt.Sprintf("fsb%s", deploymentName)

	// FIXME: the storage account may not exist on first deploy; break the template up to separate storage from the function
	codeZip, err := packageFunctionZip(f.webhookSecret, f.githubToken, flow)
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}
	blobURL, err := f.uploadCode(ctx, deploymentName, codeZip)
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

	update, err := f.deploys.CreateOrUpdate(ctx, f.resourceGroupName, deploymentName, resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Template: &azureResourcesTemplate,
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

func (f *FunctionUploader) uploadCode(ctx context.Context, storageAccountName string, codeZip []byte) (string, error) {

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
	logrus.WithField("blob_url", blobURL.String()).Debug("Uploading zip package")
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

func packageFunctionZip(secret, token string, flow flows.LoadedFlow) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

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
	if _, err := indexJS.Write([]byte(GenerateEntrypoint(secret, token, flow))); err != nil {
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

	stepFiles := map[string]struct{}{}
	for _, step := range flow.Steps {
		fn := step.Filename()

		if _, ok := stepFiles[fn]; ok {
			continue
		}
		stepFiles[fn] = struct{}{}

		stepFile, err := zw.Create(fmt.Sprintf("%s.js", fn))
		if err != nil {
			return nil, err
		}
		if _, err := fmt.Fprint(stepFile, step.SourceCode); err != nil {
			return nil, err
		}
	}

	if err := addFile(zw, "proxies.json"); err != nil {
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
