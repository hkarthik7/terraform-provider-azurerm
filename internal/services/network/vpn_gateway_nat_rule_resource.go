// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package network

import (
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-sdk/resource-manager/network/2024-05-01/virtualwans"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
)

func resourceVPNGatewayNatRule() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceVPNGatewayNatRuleCreate,
		Read:   resourceVPNGatewayNatRuleRead,
		Update: resourceVPNGatewayNatRuleUpdate,
		Delete: resourceVPNGatewayNatRuleDelete,

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := virtualwans.ParseNatRuleID(id)
			return err
		}),

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},

			"vpn_gateway_id": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: virtualwans.ValidateVpnGatewayID,
			},

			"external_mapping": {
				Type:     pluginsdk.TypeList,
				Optional: true,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"address_space": {
							Type:         pluginsdk.TypeString,
							Required:     true,
							ValidateFunc: validation.IsCIDR,
						},

						"port_range": {
							Type:         pluginsdk.TypeString,
							Optional:     true,
							ValidateFunc: validation.StringIsNotEmpty,
						},
					},
				},
			},

			"internal_mapping": {
				Type:     pluginsdk.TypeList,
				Optional: true,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"address_space": {
							Type:         pluginsdk.TypeString,
							Required:     true,
							ValidateFunc: validation.IsCIDR,
						},

						"port_range": {
							Type:         pluginsdk.TypeString,
							Optional:     true,
							ValidateFunc: validation.StringIsNotEmpty,
						},
					},
				},
			},

			"ip_configuration_id": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					"Instance0",
					"Instance1",
				}, false),
			},

			"mode": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  string(virtualwans.VpnNatRuleModeEgressSnat),
				ValidateFunc: validation.StringInSlice([]string{
					string(virtualwans.VpnNatRuleModeEgressSnat),
					string(virtualwans.VpnNatRuleModeIngressSnat),
				}, false),
			},

			"type": {
				Type:     pluginsdk.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  string(virtualwans.VpnNatRuleTypeStatic),
				ValidateFunc: validation.StringInSlice([]string{
					string(virtualwans.VpnNatRuleTypeStatic),
					string(virtualwans.VpnNatRuleTypeDynamic),
				}, false),
			},
		},
	}
}

func resourceVPNGatewayNatRuleCreate(d *pluginsdk.ResourceData, meta interface{}) error {
	subscriptionId := meta.(*clients.Client).Account.SubscriptionId
	client := meta.(*clients.Client).Network.VirtualWANs
	ctx, cancel := timeouts.ForCreate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	vpnGatewayId, err := virtualwans.ParseVpnGatewayID(d.Get("vpn_gateway_id").(string))
	if err != nil {
		return err
	}

	id := virtualwans.NewNatRuleID(subscriptionId, vpnGatewayId.ResourceGroupName, vpnGatewayId.VpnGatewayName, d.Get("name").(string))

	existing, err := client.NatRulesGet(ctx, id)
	if err != nil {
		if !response.WasNotFound(existing.HttpResponse) {
			return fmt.Errorf("checking for existing %s: %+v", id, err)
		}
	}
	if !response.WasNotFound(existing.HttpResponse) {
		return tf.ImportAsExistsError("azurerm_vpn_gateway_nat_rule", id.ID())
	}

	props := virtualwans.VpnGatewayNatRule{
		Name: pointer.To(d.Get("name").(string)),
		Properties: &virtualwans.VpnGatewayNatRuleProperties{
			Mode: pointer.To(virtualwans.VpnNatRuleMode(d.Get("mode").(string))),
			Type: pointer.To(virtualwans.VpnNatRuleType(d.Get("type").(string))),
		},
	}

	if v, ok := d.GetOk("external_mapping"); ok {
		props.Properties.ExternalMappings = expandVpnGatewayNatRuleMappings(v.([]interface{}))
	}

	if v, ok := d.GetOk("internal_mapping"); ok {
		props.Properties.InternalMappings = expandVpnGatewayNatRuleMappings(v.([]interface{}))
	}

	if v, ok := d.GetOk("ip_configuration_id"); ok {
		props.Properties.IPConfigurationId = pointer.To(v.(string))
	}

	if err := client.NatRulesCreateOrUpdateThenPoll(ctx, id, props); err != nil {
		return fmt.Errorf("creating %s: %+v", id, err)
	}

	d.SetId(id.ID())

	return resourceVPNGatewayNatRuleRead(d, meta)
}

func resourceVPNGatewayNatRuleRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Network.VirtualWANs
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := virtualwans.ParseNatRuleID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.NatRulesGet(ctx, *id)
	if err != nil {
		if response.WasNotFound(resp.HttpResponse) {
			log.Printf("[INFO] %s was not found - removing from state", id)
			d.SetId("")
			return nil
		}
		return fmt.Errorf("retrieving %s: %+v", id, err)
	}

	d.Set("name", id.NatRuleName)

	gatewayId := virtualwans.NewVpnGatewayID(id.SubscriptionId, id.ResourceGroupName, id.VpnGatewayName)
	d.Set("vpn_gateway_id", gatewayId.ID())

	if model := resp.Model; model != nil {
		if props := model.Properties; props != nil {
			d.Set("ip_configuration_id", props.IPConfigurationId)
			d.Set("mode", pointer.From(props.Mode))
			d.Set("type", pointer.From(props.Type))

			if err := d.Set("external_mapping", flattenVpnGatewayNatRuleMappings(props.ExternalMappings)); err != nil {
				return fmt.Errorf("setting `external_mapping`: %+v", err)
			}

			if err := d.Set("internal_mapping", flattenVpnGatewayNatRuleMappings(props.InternalMappings)); err != nil {
				return fmt.Errorf("setting `internal_mapping`: %+v", err)
			}
		}
	}

	return nil
}

func resourceVPNGatewayNatRuleUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Network.VirtualWANs
	ctx, cancel := timeouts.ForUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := virtualwans.ParseNatRuleID(d.Id())
	if err != nil {
		return err
	}

	existing, err := client.NatRulesGet(ctx, *id)
	if err != nil {
		return fmt.Errorf("retrieving %s: %+v", id, err)
	}

	if existing.Model == nil {
		return fmt.Errorf("retrieving %s: `model` was nil", id)
	}
	if existing.Model.Properties == nil {
		return fmt.Errorf("retrieving %s: `properties` was nil", id)
	}

	props := virtualwans.VpnGatewayNatRule{
		Name: pointer.To(d.Get("name").(string)),
		Properties: &virtualwans.VpnGatewayNatRuleProperties{
			Mode:             pointer.To(virtualwans.VpnNatRuleMode(d.Get("mode").(string))),
			Type:             pointer.To(virtualwans.VpnNatRuleType(d.Get("type").(string))),
			ExternalMappings: existing.Model.Properties.ExternalMappings,
			InternalMappings: existing.Model.Properties.InternalMappings,
		},
	}

	if ok := d.HasChange("external_mapping"); ok {
		props.Properties.ExternalMappings = expandVpnGatewayNatRuleMappings(d.Get("external_mapping").([]interface{}))
	}

	if ok := d.HasChange("internal_mapping"); ok {
		props.Properties.InternalMappings = expandVpnGatewayNatRuleMappings(d.Get("internal_mapping").([]interface{}))
	}

	if v, ok := d.GetOk("ip_configuration_id"); ok {
		props.Properties.IPConfigurationId = pointer.To(v.(string))
	}

	if err := client.NatRulesCreateOrUpdateThenPoll(ctx, *id, props); err != nil {
		return fmt.Errorf("updating %s: %+v", id, err)
	}

	return resourceVPNGatewayNatRuleRead(d, meta)
}

func resourceVPNGatewayNatRuleDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Network.VirtualWANs
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := virtualwans.ParseNatRuleID(d.Id())
	if err != nil {
		return err
	}

	if err := client.NatRulesDeleteThenPoll(ctx, *id); err != nil {
		return fmt.Errorf("deleting %s: %+v", id, err)
	}

	return nil
}

func expandVpnGatewayNatRuleMappings(input []interface{}) *[]virtualwans.VpnNatRuleMapping {
	results := make([]virtualwans.VpnNatRuleMapping, 0)

	for _, item := range input {
		v := item.(map[string]interface{})

		result := virtualwans.VpnNatRuleMapping{
			AddressSpace: pointer.To(v["address_space"].(string)),
		}

		if portRange := v["port_range"].(string); portRange != "" {
			result.PortRange = pointer.To(portRange)
		}

		results = append(results, result)
	}

	return &results
}

func flattenVpnGatewayNatRuleMappings(input *[]virtualwans.VpnNatRuleMapping) []interface{} {
	results := make([]interface{}, 0)
	if input == nil {
		return results
	}

	for _, item := range *input {
		var addressSpace string
		if item.AddressSpace != nil {
			addressSpace = *item.AddressSpace
		}

		var portRange string
		if item.PortRange != nil {
			portRange = *item.PortRange
		}

		results = append(results, map[string]interface{}{
			"address_space": addressSpace,
			"port_range":    portRange,
		})
	}

	return results
}
