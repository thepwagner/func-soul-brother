package az

// https://github.com/Azure/azure-quickstart-templates/blob/master/101-function-app-create-dynamic/azuredeploy.json
const template = `
{
  "$schema": "https://schema.management.azure.com/schemas/2015-01-01/deploymentTemplate.json#",
  "contentVersion": "1.0.0.0",
  "parameters": {
    "appName": {
      "type": "string",
      "metadata": {
        "description": "The name of the function app that you wish to create."
      }
    },
    "cleanAppName": {
      "type": "string",
      "metadata": {
        "description": "The name of the function app that you wish to create."
      }
    },
	"blobURL": {
      "type": "string",
      "metadata": {
        "description": "The name of the function app that you wish to create."
      }
    },
    "storageAccountType": {
      "type": "string",
      "defaultValue": "Standard_LRS",
      "allowedValues": ["Standard_LRS", "Standard_GRS", "Standard_RAGRS"],
      "metadata": {
        "description": "Storage Account type"
      }
    },
    "location": {
      "type": "string",
      "defaultValue": "[resourceGroup().location]",
      "metadata": {
        "description": "Location for all resources."
      }
    }
  },
  "variables": {
    "functionAppName": "[parameters('cleanAppName')]",
    "hostingPlanName": "[parameters('appName')]",
    "applicationInsightsName": "[parameters('appName')]",
    "storageAccountName": "[parameters('cleanAppName')]",
    "storageAccountid": "[concat(resourceGroup().id,'/providers/','Microsoft.Storage/storageAccounts/', variables('storageAccountName'))]"
  },
  "resources": [
    {
      "type": "Microsoft.Storage/storageAccounts",
      "name": "[variables('storageAccountName')]",
      "apiVersion": "2016-12-01",
      "location": "[parameters('location')]",
      "kind": "Storage",
      "sku": {
        "name": "[parameters('storageAccountType')]"
      }
    },
    {
      "type": "Microsoft.Web/serverfarms",
      "apiVersion": "2019-08-01",
      "name": "[variables('hostingPlanName')]",
      "kind": "functionapp",
      "location": "[parameters('location')]",
      "sku": {
        "name": "Y1",
        "tier": "Dynamic"
      },
      "properties": {
        "name": "[variables('hostingPlanName')]",
        "computeMode": "Dynamic",
        "reserved": true
      }
    },
    {
      "apiVersion": "2015-08-01",
      "type": "Microsoft.Web/sites",
      "name": "[variables('functionAppName')]",
      "location": "[parameters('location')]",
      "kind": "functionapp,linux",
      "dependsOn": [
        "[resourceId('Microsoft.Web/serverfarms', variables('hostingPlanName'))]",
        "[resourceId('Microsoft.Storage/storageAccounts', variables('storageAccountName'))]"
      ],
      "properties": {
        "serverFarmId": "[resourceId('Microsoft.Web/serverfarms', variables('hostingPlanName'))]",
        "siteConfig": {
          "appSettings": [
            {
              "name": "AzureWebJobsStorage",
              "value": "[concat('DefaultEndpointsProtocol=https;AccountName=', variables('storageAccountName'), ';AccountKey=', listKeys(variables('storageAccountid'),'2015-05-01-preview').key1)]"
            },
			{
				"name": "WEBSITE_RUN_FROM_PACKAGE",
				"value": "[parameters('blobURL')]"
			},
            {
              "name": "FUNCTIONS_EXTENSION_VERSION",
              "value": "~3"
            },
            {
              "name": "APPINSIGHTS_INSTRUMENTATIONKEY",
              "value": "[reference(resourceId('microsoft.insights/components/', variables('applicationInsightsName')), '2015-05-01').InstrumentationKey]"
            },
            {
              "name": "FUNCTIONS_WORKER_RUNTIME",
              "value": "node"
            },
			{
				"name": "WEBSITE_NODE_DEFAULT_VERSION",
				"value": "~12"
			}
          ]
        }
      }
    },
    {
      "type": "Microsoft.Insights/components",
      "apiVersion": "2018-05-01-preview",
      "name": "[variables('applicationInsightsName')]",
      "location": "[parameters('location')]",
      "tags": {
        "[concat('hidden-link:', resourceGroup().id, '/providers/Microsoft.Web/sites/', variables('applicationInsightsName'))]": "Resource"
      },
      "properties": {
        "ApplicationId": "[variables('applicationInsightsName')]",
        "Request_Source": "IbizaWebAppExtensionCreate"
      }
    }
  ]
}`

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

var proxiesConfig = []byte(`{
  "$schema": "http://json.schemastore.org/proxies",
  "proxies": {}
}`)

var entryPoint = `
const fs = require('fs');
const verify = require('@octokit/webhooks/verify');

// FIXME: should be templated
const secret = 'eM9FdMwTVtg83MTyVXUCedui0BWewkLDZ91bGao3BfU+bNg4+eEoEOeEpPOjDMs8';

module.exports = async function (context, req) {
  try {
    if (!verify(secret, req.body, req.headers['x-hub-signature'])) {
      context.res = {
        status: 401,
        body: "Signature failed"
      };
      return;
    }

    // FIXME: via template
    if (req.headers['x-github-event'] !== 'issue_comment') {
      context.res = {
        status: 200,
        body: "not issue_comment"
      };
      return;
    }
    if (req.body.action !== "created") {
      context.res = {
        status: 200,
        body: "not issue_comment.created"
      };
      return;
    }
    if (req.body.comment.user.login === 'github-actions[bot]') {
      context.res = {
        status: 200,
        body: "ignoring actions"
      };
      return;
    }

    const eventFile = '/tmp/eventPayload.json';
    fs.writeFileSync(eventFile, JSON.stringify(req.body));
	context.log('wrote event JSON, invoking action');

    process.env.GITHUB_EVENT_PATH = eventFile;
	process.env.INPUT_TOKEN = %q;
	process.env.INPUT_ID = "Azure Functions";
	
	const moduleName = require.resolve('../echo-timer');
	delete require.cache[moduleName];

    await require('../echo-timer');
    context.res = {
      status: 200,
      body: "subprocess complete"
    };
  } catch (e) {
    context.log(e);
    context.log(e.stack);
    context.res = {
      status: 500,
      body: e.message,
    };
  }
};
`

var funcIgnore = []byte(`*.js.map
*.ts
.git*
.vscode
local.settings.json
test
tsconfig.json`)
