package az

import (
	"encoding/json"
)

var azureResourcesTemplate map[string]interface{}

func init() {
	// https://github.com/Azure/azure-quickstart-templates/blob/master/101-function-app-create-dynamic/azuredeploy.json
	const resourcesJSON = `
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
	if err := json.Unmarshal([]byte(resourcesJSON), &azureResourcesTemplate); err != nil {
		panic(err)
	}
}
