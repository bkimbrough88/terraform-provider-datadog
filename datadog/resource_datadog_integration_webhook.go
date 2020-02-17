package datadog

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/zorkian/go-datadog-api"
)

var integrationWebhookMutex = sync.Mutex{}

func getWebhookSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"name": {
			Type:     schema.TypeString,
			Required: true,
		},
		"url": {
			Type:     schema.TypeString,
			Required: true,
		},
		"use_custom_payload": {
			Type:     schema.TypeBool,
			Optional: true,
		},
		"custom_payload": {
			Type:     schema.TypeString,
			Optional: true,
		},
		"encode_as_form": {
			Type:     schema.TypeBool,
			Optional: true,
		},
		"headers": {
			Type:     schema.TypeMap,
			Optional: true,
			Elem:     &schema.Schema{Type: schema.TypeString},
		},
	}
}

func resourceDatadogIntegrationWebhook() *schema.Resource {
	return &schema.Resource{
		Create: resourceDatadogIntegrationWebhookCreate,
		Read:   resourceDatadogIntegrationWebhookRead,
		Delete: resourceDatadogIntegrationWebhookDelete,
		Exists: resourceDatadogIntegrationWebhookExists,
		Importer: &schema.ResourceImporter{
			State: resourceDatadogIntegrationWebhookImport,
		},

		Schema: map[string]*schema.Schema{
			"hooks": {
				Type:     schema.TypeList,
				Required: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: getWebhookSchema(),
				},
			},
		},
	}
}

func resourceDatadogIntegrationWebhookExists(d *schema.ResourceData, meta interface{}) (b bool, e error) {
	// Exists - This is called to verify a resource still exists. It is called prior to Read,
	// and lowers the burden of Read to be able to assume the resource exists.
	client := meta.(*datadog.Client)

	integration, err := client.GetIntegrationWebhook()

	if err != nil && err.Error() == "Not Found" {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return len(integration.Webhooks) > 0, nil
}

func buildDatadogHeader(headers map[string]string) string {
	var headerList []string

	for key, value := range headers {
		headerList = append(headerList, fmt.Sprintf("%s: %s", key, value))
	}

	return strings.Join(headerList, "\n")
}

func buildDatadogWebhook(terraformWebhook map[string]interface{}) datadog.Webhook {
	webhook := datadog.Webhook{
		Name: datadog.String(terraformWebhook["name"].(string)),
		URL:  datadog.String(terraformWebhook["url"].(string)),
	}

	if attr, ok := terraformWebhook["use_custom_payload"]; ok {
		webhook.UseCustomPayload = datadog.String(strconv.FormatBool(attr.(bool)))
	}

	if attr, ok := terraformWebhook["custom_payload"]; ok {
		webhook.CustomPayload = datadog.String(attr.(string))
	}

	if attr, ok := terraformWebhook["encode_as_form"]; ok {
		webhook.EncodeAsForm = datadog.String(strconv.FormatBool(attr.(bool)))
	}

	if attr, ok := terraformWebhook["headers"]; ok {
		webhook.Headers = datadog.String(buildDatadogHeader(attr.(map[string]string)))
	}

	return webhook
}

func resourceDatadogIntegrationWebhookPrepareCreateRequest(d *schema.ResourceData) datadog.IntegrationWebhookRequest {

	iwebhook := datadog.IntegrationWebhookRequest{
		Webhooks: []datadog.Webhook{},
	}

	for _, hook := range d.Get("hooks").([]interface{}) {
		iwebhook.Webhooks = append(iwebhook.Webhooks, buildDatadogWebhook(hook.(map[string]interface{})))
	}

	return iwebhook
}

func resourceDatadogIntegrationWebhookCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*datadog.Client)
	integrationWebhookMutex.Lock()
	defer integrationWebhookMutex.Unlock()

	iwebhook := resourceDatadogIntegrationWebhookPrepareCreateRequest(d)

	if err := client.CreateIntegrationWebhook(&iwebhook); err != nil {
		return fmt.Errorf("error creating a Webhook integration: %s", err.Error())
	}

	return resourceDatadogIntegrationWebhookRead(d, meta)
}

func buildTerraformHeader(datadogHeader *string) (*map[string]string, error) {
	terraformHeaders := map[string]string{}

	if strings.Trim(*datadogHeader, " \t\n") != "" {
		headerStrList := strings.Split(*datadogHeader, "\n")

		for _, headerStr := range headerStrList {
			if strings.Contains(headerStr, ":") {
				split := strings.Split(headerStr, ":")

				terraformHeaders[split[0]] = strings.TrimLeft(strings.Join(split[1:], ""), " ")
			} else {
				return nil, fmt.Errorf("header not correctly formatted, expected ':' in '%s'", headerStr)
			}
		}
	}

	return &terraformHeaders, nil
}

func buildTerraformWebhooks(datadogWebhooks []datadog.Webhook) (*[]map[string]interface{}, error) {
	terraformWebhooks := make([]map[string]interface{}, len(datadogWebhooks))

	for idx, datadogWebhook := range datadogWebhooks {
		terraformWebhook := map[string]interface{}{}
		terraformWebhook["name"] = datadogWebhook.Name
		terraformWebhook["url"] = datadogWebhook.URL

		if datadogWebhook.UseCustomPayload != nil {
			val, err := strconv.ParseBool(*datadogWebhook.UseCustomPayload)
			if err != nil {
				return nil, err
			}
			terraformWebhook["use_custom_payload"] = val
		}

		if datadogWebhook.CustomPayload != nil {
			terraformWebhook["custom_payload"] = datadogWebhook.CustomPayload
		}

		if datadogWebhook.EncodeAsForm != nil {
			val, err := strconv.ParseBool(*datadogWebhook.EncodeAsForm)
			if err != nil {
				return nil, err
			}
			terraformWebhook["encode_as_form"] = val
		}

		if datadogWebhook.Headers != nil {
			val, err := buildTerraformHeader(datadogWebhook.Headers)
			if err != nil {
				return nil, err
			}
			terraformWebhook["headers"] = val
		}

		terraformWebhooks[idx] = terraformWebhook
	}

	return &terraformWebhooks, nil
}

func resourceDatadogIntegrationWebhookRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*datadog.Client)

	integration, err := client.GetIntegrationWebhook()
	if err != nil {
		return fmt.Errorf("error reading the Webhook integration: %s", err.Error())
	}

	terraformWebhooks, err := buildTerraformWebhooks(integration.Webhooks)
	if err != nil {
		return err
	}

	return d.Set("hooks", terraformWebhooks)
}

func resourceDatadogIntegrationWebhookDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*datadog.Client)
	integrationWebhookMutex.Lock()
	defer integrationWebhookMutex.Unlock()

	if err := client.DeleteIntegrationWebhook(); err != nil {
		return fmt.Errorf("error deleting a Webhook integration: %s", err.Error())
	}

	return nil
}

func resourceDatadogIntegrationWebhookImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	if err := resourceDatadogIntegrationWebhookRead(d, meta); err != nil {
		return nil, err
	}
	return []*schema.ResourceData{d}, nil
}
