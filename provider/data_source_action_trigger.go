package provider

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/twi-logos/terraform-provider-zabbix/internal/zabbix"
)

func dataActionTrigger() *schema.Resource {
	actionSchema := computedActionTriggerSchema(resourceActionTrigger().Schema)
	actionSchema["action_id"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		ExactlyOneOf: []string{"action_id", "name"},
		ValidateFunc: validation.StringMatch(regexp.MustCompile(`^[0-9]+$`), "must be a numeric Zabbix action ID"),
		Description:  "Zabbix action ID. Exactly one of `action_id` or `name` must be given.",
	}
	actionSchema["name"].Optional = true
	actionSchema["name"].ExactlyOneOf = []string{"action_id", "name"}
	actionSchema["name"].ValidateFunc = validation.StringIsNotWhiteSpace
	actionSchema["name"].Description = "Trigger action name. Exactly one of `action_id` or `name` must be given."

	return &schema.Resource{
		Description: "Looks up an existing Zabbix trigger action by ID or exact name.",
		Read:        dataActionTriggerRead,
		Schema:      actionSchema,
	}
}

func computedActionTriggerSchema(input map[string]*schema.Schema) map[string]*schema.Schema {
	result := make(map[string]*schema.Schema, len(input))
	for name, original := range input {
		cloned := *original
		cloned.Required = false
		cloned.Optional = false
		cloned.Computed = true
		cloned.ForceNew = false
		cloned.Default = nil
		cloned.DefaultFunc = nil
		cloned.ValidateFunc = nil
		cloned.ValidateDiagFunc = nil
		cloned.ConflictsWith = nil
		cloned.ExactlyOneOf = nil
		cloned.AtLeastOneOf = nil
		cloned.RequiredWith = nil
		cloned.MinItems = 0
		cloned.MaxItems = 0
		if nested, ok := original.Elem.(*schema.Resource); ok {
			nestedClone := *nested
			nestedClone.Schema = computedActionTriggerSchema(nested.Schema)
			cloned.Elem = &nestedClone
		}
		result[name] = &cloned
	}
	return result
}

func dataActionTriggerRead(d *schema.ResourceData, meta interface{}) error {
	api := meta.(*zabbix.API)
	var action *zabbix.Action

	if actionID, lookupByID := d.GetOk("action_id"); lookupByID {
		var err error
		action, err = api.ActionGetByID(actionID.(string))
		if err != nil {
			return fmt.Errorf("read trigger action by ID %q: %w", actionID, err)
		}
		if action == nil {
			return fmt.Errorf("no trigger action found with ID %q; it may not exist or may be hidden by Zabbix permissions", actionID)
		}
	} else {
		name := d.Get("name").(string)
		actions, err := api.ActionsGetByName(name)
		if err != nil {
			return fmt.Errorf("read trigger action by name %q: %w", name, err)
		}
		switch len(actions) {
		case 0:
			return fmt.Errorf("no trigger action found with name %q", name)
		case 1:
			action = &actions[0]
		default:
			return fmt.Errorf("found %d trigger actions with name %q; use action_id to disambiguate", len(actions), name)
		}
	}

	state, err := flattenActionTrigger(*action)
	if err != nil {
		return fmt.Errorf("trigger action %s cannot be represented safely: %w", action.ActionID, err)
	}
	for key, value := range state {
		if err := d.Set(key, value); err != nil {
			return fmt.Errorf("set trigger action %s state %s: %w", action.ActionID, key, err)
		}
	}
	d.SetId(action.ActionID)
	return nil
}
