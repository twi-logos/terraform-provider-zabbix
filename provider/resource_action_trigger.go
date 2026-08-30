package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/tpretz/terraform-provider-zabbix/internal/zabbix"
)

var actionStatus = map[string]int{"enabled": 0, "disabled": 1}
var actionStatusRev = reverseIntMap(actionStatus)
var actionEvalType = map[string]int{"and_or": 0, "and": 1, "or": 2, "custom_expression": 3}
var actionEvalTypeRev = reverseIntMap(actionEvalType)
var actionConditionType = map[string]int{
	"host_group":         0,
	"host":               1,
	"trigger":            2,
	"trigger_name":       3,
	"trigger_severity":   4,
	"time_period":        6,
	"host_template":      13,
	"maintenance_status": 16,
	"event_tag":          25,
	"event_tag_value":    26,
}
var actionConditionTypeRev = reverseIntMap(actionConditionType)
var actionOperator = map[string]int{
	"equals": 0, "not_equals": 1, "like": 2, "not_like": 3,
	"in": 4, "greater_or_equals": 5, "less_or_equals": 6, "not_in": 7,
	"yes": 10, "no": 11,
}
var actionOperatorRev = reverseIntMap(actionOperator)
var actionOperationEvalType = map[string]int{"and_or": 0, "and": 1, "or": 2}
var actionOperationEvalTypeRev = reverseIntMap(actionOperationEvalType)

var actionAllowedOperators = map[int]map[int]bool{
	0:  {0: true, 1: true},
	1:  {0: true, 1: true},
	2:  {0: true, 1: true},
	3:  {2: true, 3: true},
	4:  {0: true, 1: true, 5: true, 6: true},
	6:  {4: true, 7: true},
	13: {0: true, 1: true},
	16: {10: true, 11: true},
	25: {0: true, 1: true, 2: true, 3: true},
	26: {0: true, 1: true, 2: true, 3: true},
}

func reverseIntMap(values map[string]int) map[int]string {
	reversed := make(map[int]string, len(values))
	for name, value := range values {
		reversed[value] = name
	}
	return reversed
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func numericStringSchema(description string) *schema.Schema {
	return &schema.Schema{
		Type:         schema.TypeString,
		Required:     true,
		Description:  description,
		ValidateFunc: validation.StringMatch(regexp.MustCompile(`^[0-9]+$`), "must be a numeric string"),
	}
}

func actionMessageSchema(requireRecipients bool) *schema.Resource {
	message := &schema.Resource{Schema: map[string]*schema.Schema{
		"use_default_message": {
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     true,
			Description: "Use the media type's default message. Set false to send subject and message.",
		},
		"subject": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Custom message subject when use_default_message is false.",
		},
		"message": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Custom message body when use_default_message is false.",
		},
		"media_type_id": {
			Type:         schema.TypeString,
			Optional:     true,
			Default:      "0",
			Description:  "Media type ID. Zero means all media types configured for the recipient.",
			ValidateFunc: validation.StringMatch(regexp.MustCompile(`^[0-9]+$`), "must be a numeric string"),
		},
	}}
	if requireRecipients {
		message.Schema["user_group_ids"] = &schema.Schema{
			Type:        schema.TypeSet,
			Optional:    true,
			Elem:        &schema.Schema{Type: schema.TypeString},
			Description: "User group recipients.",
		}
		message.Schema["user_ids"] = &schema.Schema{
			Type:        schema.TypeSet,
			Optional:    true,
			Elem:        &schema.Schema{Type: schema.TypeString},
			Description: "User recipients.",
		}
	}
	return message
}

func actionRemoteCommandSchema() *schema.Resource {
	return &schema.Resource{Schema: map[string]*schema.Schema{
		"current_host": {
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     false,
			Description: "Run the global script on the host that generated the event.",
		},
		"host_ids": {
			Type:        schema.TypeSet,
			Optional:    true,
			Elem:        &schema.Schema{Type: schema.TypeString},
			Description: "Explicit host targets.",
		},
		"host_group_ids": {
			Type:        schema.TypeSet,
			Optional:    true,
			Elem:        &schema.Schema{Type: schema.TypeString},
			Description: "Host group targets.",
		},
		"global_script": {
			Type:     schema.TypeList,
			Required: true,
			MinItems: 1,
			MaxItems: 1,
			Elem: &schema.Resource{Schema: map[string]*schema.Schema{
				"script_id": numericStringSchema("ID of the Zabbix global script to run."),
			}},
			Description: "Global script reference. Action-level inline command definitions are not supported by Zabbix 6.0 or newer.",
		},
	}}
}

func actionOperationConditionSchema() *schema.Resource {
	return &schema.Resource{Schema: map[string]*schema.Schema{
		"acknowledged": {
			Type:        schema.TypeBool,
			Required:    true,
			Description: "Whether the event must be acknowledged.",
		},
	}}
}

func actionProblemOperationSchema() *schema.Resource {
	return &schema.Resource{Schema: map[string]*schema.Schema{
		"escalation_period": {
			Type:        schema.TypeString,
			Optional:    true,
			Default:     "0",
			Description: "Escalation period for this operation. Zero inherits the action period.",
		},
		"escalation_step_from": {
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      1,
			ValidateFunc: validation.IntAtLeast(1),
			Description:  "First escalation step.",
		},
		"escalation_step_to": {
			Type:         schema.TypeInt,
			Optional:     true,
			Default:      1,
			ValidateFunc: validation.IntAtLeast(0),
			Description:  "Last escalation step. Zero means all remaining steps.",
		},
		"condition_evaluation_type": {
			Type:         schema.TypeString,
			Optional:     true,
			Default:      "and_or",
			ValidateFunc: validation.StringInSlice(sortedKeys(actionOperationEvalType), false),
			Description:  "How acknowledgement conditions are evaluated: and_or, and, or.",
		},
		"condition": {
			Type:        schema.TypeSet,
			Optional:    true,
			Elem:        actionOperationConditionSchema(),
			Description: "Event acknowledgement conditions.",
		},
		"send_message": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Elem:        actionMessageSchema(true),
			Description: "Send a message.",
		},
		"remote_command": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Elem:        actionRemoteCommandSchema(),
			Description: "Run a global script.",
		},
	}}
}

func actionRecoveryOperationSchema(prefix string, update bool) *schema.Resource {
	notifyMessage := actionMessageSchema(false)
	if !update {
		delete(notifyMessage.Schema, "media_type_id")
	}
	return &schema.Resource{Schema: map[string]*schema.Schema{
		"notify_all_involved": {
			Type:        schema.TypeBool,
			Optional:    true,
			Default:     false,
			Description: "Notify all users who previously received problem notifications.",
		},
		"notify_all_message": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Elem:        notifyMessage,
			Description: "Message state for notify_all_involved. This preserves custom notify-all messages during refresh and update.",
		},
		"send_message": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Elem:        actionMessageSchema(true),
			Description: "Send a message to explicit recipients.",
		},
		"remote_command": {
			Type:        schema.TypeList,
			Optional:    true,
			MaxItems:    1,
			Elem:        actionRemoteCommandSchema(),
			Description: "Run a global script.",
		},
	}}
}

func actionConditionSchema() *schema.Resource {
	return &schema.Resource{Schema: map[string]*schema.Schema{
		"condition_type": {
			Type:         schema.TypeString,
			Required:     true,
			ValidateFunc: validation.StringInSlice(sortedKeys(actionConditionType), false),
			Description:  "Trigger-action condition type: host_group, host, trigger, trigger_name, trigger_severity, time_period, host_template, maintenance_status, event_tag, or event_tag_value. trigger_name means Zabbix Event name; maintenance_status means Problem is suppressed.",
		},
		"operator": {
			Type:         schema.TypeString,
			Required:     true,
			ValidateFunc: validation.StringInSlice(sortedKeys(actionOperator), false),
			Description:  "Comparison operator: equals, not_equals, like, not_like, in, greater_or_equals, less_or_equals, not_in, yes, or no.",
		},
		"value": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Primary value. Must be absent for Problem is suppressed.",
		},
		"value2": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "Tag name for event_tag_value. Its value attribute contains the tag value.",
		},
		"label": {
			Type:         schema.TypeString,
			Optional:     true,
			ValidateFunc: validation.StringMatch(regexp.MustCompile(`^[A-Z]+$`), "must contain uppercase letters only"),
			Description:  "Uppercase identifier used by a custom expression, such as A or AA.",
		},
	}}
}

func resourceActionTrigger() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages a Zabbix trigger action. Unsupported filter or operation state fails closed instead of being discarded.",
		Create:        resourceActionTriggerCreate,
		Read:          resourceActionTriggerRead,
		Update:        resourceActionTriggerUpdate,
		Delete:        resourceActionTriggerDelete,
		CustomizeDiff: resourceActionTriggerCustomizeDiff,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringIsNotWhiteSpace,
				Description:  "Display name of the trigger action.",
			},
			"status": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "enabled",
				ValidateFunc: validation.StringInSlice(sortedKeys(actionStatus), false),
				Description:  "Whether the action is enabled or disabled.",
			},
			"escalation_period": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "1h",
				Description: "Default escalation period.",
			},
			"pause_suppressed": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Pause escalation for suppressed problems.",
			},
			"pause_symptoms": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Pause escalation for symptom problems. Zabbix 6.4 or newer is required to set this false; Zabbix 6.0 uses the default true state.",
			},
			"notify_if_canceled": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Whether to notify when an escalation is canceled.",
			},
			"filter": {
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
				MaxItems: 1,
				Elem: &schema.Resource{Schema: map[string]*schema.Schema{
					"evaluation_type": {
						Type:         schema.TypeString,
						Required:     true,
						ValidateFunc: validation.StringInSlice(sortedKeys(actionEvalType), false),
						Description:  "How conditions are evaluated: and_or, and, or, or custom_expression.",
					},
					"formula": {
						Type:        schema.TypeString,
						Optional:    true,
						Description: "Custom expression such as A and (B or C). Do not use braces.",
					},
					"condition": {
						Type:        schema.TypeList,
						Optional:    true,
						Elem:        actionConditionSchema(),
						Description: "Conditions in stable formula order.",
					},
				}},
				Description: "Conditions that select trigger events.",
			},
			"operations": {
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        actionProblemOperationSchema(),
				Description: "Problem escalation operations.",
			},
			"recovery_operations": {
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        actionRecoveryOperationSchema("recovery_operations", false),
				Description: "Operations run when the problem recovers.",
			},
			"update_operations": {
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        actionRecoveryOperationSchema("update_operations", true),
				Description: "Operations run when the problem is updated.",
			},
		},
	}
}

func buildActionTrigger(d *schema.ResourceData) (zabbix.Action, error) {
	action, err := buildActionTriggerConfig(d)
	action.ActionID = d.Id()
	return action, err
}

type actionTriggerConfigGetter interface {
	Get(string) interface{}
}

func buildActionTriggerConfig(d actionTriggerConfigGetter) (zabbix.Action, error) {
	action := zabbix.Action{
		Name:             d.Get("name").(string),
		Status:           actionStatus[d.Get("status").(string)],
		EscPeriod:        d.Get("escalation_period").(string),
		PauseSuppressed:  boolInt(d.Get("pause_suppressed").(bool)),
		PauseSymptoms:    boolInt(d.Get("pause_symptoms").(bool)),
		NotifyIfCanceled: boolInt(d.Get("notify_if_canceled").(bool)),
	}
	filter, err := buildActionFilter(d.Get("filter"))
	if err != nil {
		return action, err
	}
	action.Filter = filter
	if action.Operations, err = buildActionProblemOperations(d.Get("operations")); err != nil {
		return action, err
	}
	if action.RecoveryOperations, err = buildActionRecoveryOperations(d.Get("recovery_operations"), false); err != nil {
		return action, err
	}
	if action.UpdateOperations, err = buildActionRecoveryOperations(d.Get("update_operations"), true); err != nil {
		return action, err
	}
	return action, nil
}

func resourceActionTriggerCustomizeDiff(_ context.Context, d *schema.ResourceDiff, _ interface{}) error {
	raw := d.GetRawConfig()
	if !raw.IsNull() && !raw.IsWhollyKnown() {
		return nil
	}
	for _, key := range []string{"name", "filter", "operations", "recovery_operations", "update_operations"} {
		if !d.NewValueKnown(key) {
			return nil
		}
	}
	_, err := buildActionTriggerConfig(d)
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func buildActionFilter(raw interface{}) (zabbix.ActionFilter, error) {
	items := raw.([]interface{})
	if len(items) != 1 || items[0] == nil {
		return zabbix.ActionFilter{}, errors.New("filter must contain exactly one block")
	}
	block := items[0].(map[string]interface{})
	evalName := block["evaluation_type"].(string)
	filter := zabbix.ActionFilter{EvalType: actionEvalType[evalName], Formula: block["formula"].(string)}
	conditions := block["condition"].([]interface{})
	filter.Conditions = make(zabbix.ActionConditions, len(conditions))
	labels := make(map[string]bool)
	for i, rawCondition := range conditions {
		condition := rawCondition.(map[string]interface{})
		typeName := condition["condition_type"].(string)
		operatorName := condition["operator"].(string)
		conditionType := actionConditionType[typeName]
		operator := actionOperator[operatorName]
		if !actionAllowedOperators[conditionType][operator] {
			return filter, fmt.Errorf("filter condition %d: operator %q is not valid for %q", i+1, operatorName, typeName)
		}
		value, valueSet := condition["value"].(string)
		value2, value2Set := condition["value2"].(string)
		label := condition["label"].(string)
		out := zabbix.ActionCondition{ConditionType: conditionType, Operator: operator, FormulaID: label}
		if conditionType == 16 {
			if value != "" || value2 != "" {
				return filter, fmt.Errorf("filter condition %d: maintenance_status forbids value and value2", i+1)
			}
		} else {
			if !valueSet || value == "" {
				return filter, fmt.Errorf("filter condition %d: %s requires value", i+1, typeName)
			}
			out.Value = &value
			if conditionType == 26 {
				if !value2Set || value2 == "" {
					return filter, fmt.Errorf("filter condition %d: event_tag_value requires value2 as the tag name", i+1)
				}
				out.Value2 = &value2
			} else if value2 != "" {
				return filter, fmt.Errorf("filter condition %d: %s forbids value2", i+1, typeName)
			}
		}
		if evalName == "custom_expression" {
			if label == "" {
				return filter, fmt.Errorf("filter condition %d: custom_expression requires label", i+1)
			}
			if labels[label] {
				return filter, fmt.Errorf("filter condition %d: duplicate label %q", i+1, label)
			}
			labels[label] = true
		} else if label != "" {
			return filter, fmt.Errorf("filter condition %d: label requires custom_expression", i+1)
		}
		filter.Conditions[i] = out
	}
	if evalName == "custom_expression" {
		if filter.Formula == "" {
			return filter, errors.New("custom_expression requires formula")
		}
		if err := validateActionFormula(filter.Formula, labels); err != nil {
			return filter, err
		}
	} else if filter.Formula != "" {
		return filter, errors.New("formula requires custom_expression")
	}
	return filter, nil
}

func actionFormulaID(index int) string {
	var label []byte
	for index++; index > 0; index = (index - 1) / 26 {
		label = append([]byte{byte('A' + (index-1)%26)}, label...)
	}
	return string(label)
}

func validateActionFormula(formula string, labels map[string]bool) error {
	if strings.ContainsAny(formula, "{}") {
		return errors.New("custom expression formula must not use braces")
	}
	tokens := strings.Fields(strings.NewReplacer("(", " ( ", ")", " ) ").Replace(formula))
	if len(tokens) == 0 {
		return errors.New("custom_expression requires formula")
	}
	used := make(map[string]bool, len(labels))
	labelCount := 0
	expectOperand := true
	depth := 0
	for _, token := range tokens {
		if expectOperand {
			switch {
			case token == "(":
				depth++
			case labels[token]:
				if !used[token] {
					expected := actionFormulaID(labelCount)
					if token != expected {
						return fmt.Errorf("custom expression label %q must be %q in first-appearance order", token, expected)
					}
					labelCount++
				}
				used[token] = true
				expectOperand = false
			default:
				return fmt.Errorf("custom expression expected a configured label, got %q", token)
			}
			continue
		}
		switch {
		case token == "and" || token == "or":
			expectOperand = true
		case token == ")":
			if depth == 0 {
				return errors.New("custom expression has an unmatched closing parenthesis")
			}
			depth--
		default:
			return fmt.Errorf("custom expression expected and, or, or a closing parenthesis, got %q", token)
		}
	}
	if expectOperand {
		return errors.New("custom expression ends before a condition label")
	}
	if depth != 0 {
		return errors.New("custom expression has an unmatched opening parenthesis")
	}
	for label := range labels {
		if !used[label] {
			return fmt.Errorf("custom expression does not reference condition label %q", label)
		}
	}
	return nil
}

func buildActionProblemOperations(raw interface{}) (zabbix.ActionOperations, error) {
	items := actionOperationItems(raw)
	operations := make(zabbix.ActionOperations, len(items))
	for i, rawItem := range items {
		block := rawItem.(map[string]interface{})
		operation := zabbix.ActionOperation{
			EscPeriod:   block["escalation_period"].(string),
			EscStepFrom: block["escalation_step_from"].(int),
			EscStepTo:   block["escalation_step_to"].(int),
			EvalType:    actionOperationEvalType[block["condition_evaluation_type"].(string)],
		}
		if operation.EscStepTo != 0 && operation.EscStepTo < operation.EscStepFrom {
			return operations, fmt.Errorf("operations %d: escalation_step_to must be zero or at least escalation_step_from", i+1)
		}
		conditions := block["condition"].(*schema.Set).List()
		operation.OpConditions = make(zabbix.ActionOperationConditions, len(conditions))
		for j, rawCondition := range conditions {
			acknowledged := rawCondition.(map[string]interface{})["acknowledged"].(bool)
			value := "0"
			if acknowledged {
				value = "1"
			}
			operation.OpConditions[j] = zabbix.ActionOperationCondition{ConditionType: 14, Operator: 0, Value: value}
		}
		if err := fillActionOperation(&operation, block, false); err != nil {
			return operations, fmt.Errorf("operations %d: %w", i+1, err)
		}
		operations[i] = operation
	}
	return operations, nil
}

func buildActionRecoveryOperations(raw interface{}, update bool) (zabbix.ActionOperations, error) {
	items := actionOperationItems(raw)
	operations := make(zabbix.ActionOperations, len(items))
	for i, rawItem := range items {
		block := rawItem.(map[string]interface{})
		notifyAll := block["notify_all_involved"].(bool)
		operation := zabbix.ActionOperation{}
		if notifyAll {
			if update {
				operation.OperationType = 12
			} else {
				operation.OperationType = 11
			}
			message, err := buildActionNotifyAllMessage(block["notify_all_message"], update)
			if err != nil {
				return operations, fmt.Errorf("operation %d: %w", i+1, err)
			}
			operation.OpMessage = message
			if hasBlock(block["send_message"]) || hasBlock(block["remote_command"]) {
				return operations, fmt.Errorf("operation %d: notify_all_involved conflicts with send_message and remote_command", i+1)
			}
		} else {
			if hasBlock(block["notify_all_message"]) {
				return operations, fmt.Errorf("operation %d: notify_all_message requires notify_all_involved", i+1)
			}
			if err := fillActionOperation(&operation, block, false); err != nil {
				return operations, fmt.Errorf("operation %d: %w", i+1, err)
			}
		}
		operations[i] = operation
	}
	return operations, nil
}

func actionOperationItems(raw interface{}) []interface{} {
	if set, ok := raw.(*schema.Set); ok {
		return set.List()
	}
	items, _ := raw.([]interface{})
	return items
}

func fillActionOperation(operation *zabbix.ActionOperation, block map[string]interface{}, _ bool) error {
	hasMessage := hasBlock(block["send_message"])
	hasCommand := hasBlock(block["remote_command"])
	if hasMessage == hasCommand {
		return errors.New("exactly one of send_message or remote_command must be set")
	}
	if hasMessage {
		operation.OperationType = 0
		message, groups, users, err := buildActionMessage(block["send_message"])
		if err != nil {
			return err
		}
		operation.OpMessage = message
		operation.MessageGroups = groups
		operation.MessageUsers = users
		return nil
	}
	operation.OperationType = 1
	command, hosts, groups, err := buildActionRemoteCommand(block["remote_command"])
	if err != nil {
		return err
	}
	operation.OpCommand = command
	operation.CommandHosts = hosts
	operation.CommandGroups = groups
	return nil
}

func hasBlock(raw interface{}) bool {
	items := actionBlockItems(raw)
	return len(items) == 1 && items[0] != nil
}

func actionBlockItems(raw interface{}) []interface{} {
	if set, ok := raw.(*schema.Set); ok {
		return set.List()
	}
	items, _ := raw.([]interface{})
	return items
}

func buildActionMessage(raw interface{}) (*zabbix.ActionOperationMessage, zabbix.ActionMessageGroups, zabbix.ActionMessageUsers, error) {
	block := actionBlockItems(raw)[0].(map[string]interface{})
	message, err := actionMessageFromMap(block, true)
	if err != nil {
		return nil, nil, nil, err
	}
	groupValues := block["user_group_ids"].(*schema.Set).List()
	userValues := block["user_ids"].(*schema.Set).List()
	if len(groupValues)+len(userValues) == 0 {
		return nil, nil, nil, errors.New("send_message requires at least one user_group_id or user_id")
	}
	groups := make(zabbix.ActionMessageGroups, len(groupValues))
	for i, value := range groupValues {
		id := value.(string)
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(id) {
			return nil, nil, nil, fmt.Errorf("user_group_id %q must be numeric", id)
		}
		groups[i] = zabbix.ActionMessageGroup{UserGroupID: id}
	}
	users := make(zabbix.ActionMessageUsers, len(userValues))
	for i, value := range userValues {
		id := value.(string)
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(id) {
			return nil, nil, nil, fmt.Errorf("user_id %q must be numeric", id)
		}
		users[i] = zabbix.ActionMessageUser{UserID: id}
	}
	return message, groups, users, nil
}

func buildActionNotifyAllMessage(raw interface{}, update bool) (*zabbix.ActionOperationMessage, error) {
	if !hasBlock(raw) {
		message := &zabbix.ActionOperationMessage{UseDefault: 1}
		if update {
			message.MediaTypeID = "0"
		}
		return message, nil
	}
	block := actionBlockItems(raw)[0].(map[string]interface{})
	message, err := actionMessageFromMap(block, update)
	if err != nil {
		return nil, err
	}
	if !update {
		message.MediaTypeID = ""
	}
	return message, nil
}

func actionMessageFromMap(block map[string]interface{}, mediaTypeAllowed bool) (*zabbix.ActionOperationMessage, error) {
	useDefault := block["use_default_message"].(bool)
	subject := block["subject"].(string)
	messageText := block["message"].(string)
	if useDefault && (subject != "" || messageText != "") {
		return nil, errors.New("subject and message must be absent when use_default_message is true")
	}
	if !useDefault && (subject == "" || messageText == "") {
		return nil, errors.New("subject and message are required when use_default_message is false")
	}
	message := &zabbix.ActionOperationMessage{
		UseDefault: boolInt(useDefault),
		Subject:    subject,
		Message:    messageText,
	}
	if mediaTypeAllowed {
		message.MediaTypeID = block["media_type_id"].(string)
	}
	return message, nil
}

func buildActionRemoteCommand(raw interface{}) (*zabbix.ActionOperationCommand, zabbix.ActionCommandHosts, zabbix.ActionCommandGroups, error) {
	block := actionBlockItems(raw)[0].(map[string]interface{})
	scriptBlocks := actionBlockItems(block["global_script"])
	if len(scriptBlocks) != 1 || scriptBlocks[0] == nil {
		return nil, nil, nil, errors.New("remote_command requires global_script")
	}
	scriptID := scriptBlocks[0].(map[string]interface{})["script_id"].(string)
	hostValues := block["host_ids"].(*schema.Set).List()
	groupValues := block["host_group_ids"].(*schema.Set).List()
	currentHost := block["current_host"].(bool)
	if !currentHost && len(hostValues)+len(groupValues) == 0 {
		return nil, nil, nil, errors.New("remote_command requires current_host, a host_id, or a host_group_id")
	}
	hosts := make(zabbix.ActionCommandHosts, 0, len(hostValues)+1)
	if currentHost {
		hosts = append(hosts, zabbix.ActionCommandHost{HostID: "0"})
	}
	for _, value := range hostValues {
		id := value.(string)
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(id) {
			return nil, nil, nil, fmt.Errorf("host_id %q must be numeric", id)
		}
		hosts = append(hosts, zabbix.ActionCommandHost{HostID: id})
	}
	groups := make(zabbix.ActionCommandGroups, len(groupValues))
	for i, value := range groupValues {
		id := value.(string)
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(id) {
			return nil, nil, nil, fmt.Errorf("host_group_id %q must be numeric", id)
		}
		groups[i] = zabbix.ActionCommandGroup{GroupID: id}
	}
	return &zabbix.ActionOperationCommand{ScriptID: scriptID}, hosts, groups, nil
}

func flattenActionTrigger(action zabbix.Action) (map[string]interface{}, error) {
	status, ok := actionStatusRev[action.Status]
	if !ok {
		return nil, fmt.Errorf("unsupported action status %d", action.Status)
	}
	pauseSuppressed, err := actionBooleanState("pause_suppressed", action.PauseSuppressed)
	if err != nil {
		return nil, err
	}
	pauseSymptoms, err := actionBooleanState("pause_symptoms", action.PauseSymptoms)
	if err != nil {
		return nil, err
	}
	notifyIfCanceled, err := actionBooleanState("notify_if_canceled", action.NotifyIfCanceled)
	if err != nil {
		return nil, err
	}
	filter, err := flattenActionFilter(action.Filter)
	if err != nil {
		return nil, err
	}
	problem, err := flattenActionProblemOperations(action.Operations)
	if err != nil {
		return nil, err
	}
	recovery, err := flattenActionRecoveryOperations(action.RecoveryOperations, false)
	if err != nil {
		return nil, err
	}
	update, err := flattenActionRecoveryOperations(action.UpdateOperations, true)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"name":                action.Name,
		"status":              status,
		"escalation_period":   action.EscPeriod,
		"pause_suppressed":    pauseSuppressed,
		"pause_symptoms":      pauseSymptoms,
		"notify_if_canceled":  notifyIfCanceled,
		"filter":              []interface{}{filter},
		"operations":          problem,
		"recovery_operations": recovery,
		"update_operations":   update,
	}, nil
}

func actionBooleanState(name string, value int) (bool, error) {
	switch value {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("unsupported %s value %d", name, value)
	}
}

func flattenActionFilter(filter zabbix.ActionFilter) (map[string]interface{}, error) {
	evalName, ok := actionEvalTypeRev[filter.EvalType]
	if !ok {
		return nil, fmt.Errorf("unsupported action filter evaltype %d", filter.EvalType)
	}
	conditions := make([]interface{}, len(filter.Conditions))
	for i, condition := range filter.Conditions {
		typeName, ok := actionConditionTypeRev[condition.ConditionType]
		if !ok {
			return nil, fmt.Errorf("unsupported action condition type %d", condition.ConditionType)
		}
		operatorName, ok := actionOperatorRev[condition.Operator]
		if !ok || !actionAllowedOperators[condition.ConditionType][condition.Operator] {
			return nil, fmt.Errorf("unsupported operator %d for action condition type %d", condition.Operator, condition.ConditionType)
		}
		out := map[string]interface{}{"condition_type": typeName, "operator": operatorName}
		if filter.EvalType == 3 {
			out["label"] = condition.FormulaID
		}
		if condition.ConditionType == 16 {
			if condition.Value != nil && *condition.Value != "" || condition.Value2 != nil && *condition.Value2 != "" {
				return nil, errors.New("problem-suppressed action condition unexpectedly contains value state")
			}
		} else {
			if condition.Value == nil {
				return nil, fmt.Errorf("action condition type %d has no value", condition.ConditionType)
			}
			out["value"] = *condition.Value
			if condition.ConditionType == 26 {
				if condition.Value2 == nil {
					return nil, errors.New("event-tag-value action condition has no tag name in value2")
				}
				out["value2"] = *condition.Value2
			} else if condition.Value2 != nil && *condition.Value2 != "" {
				return nil, fmt.Errorf("action condition type %d unexpectedly contains value2", condition.ConditionType)
			}
		}
		conditions[i] = out
	}
	return map[string]interface{}{"evaluation_type": evalName, "formula": filter.Formula, "condition": conditions}, nil
}

func flattenActionProblemOperations(operations zabbix.ActionOperations) ([]interface{}, error) {
	result := make([]interface{}, len(operations))
	for i, operation := range operations {
		if operation.OperationType != 0 && operation.OperationType != 1 {
			return nil, fmt.Errorf("unsupported problem operation type %d", operation.OperationType)
		}
		evalName, ok := actionOperationEvalTypeRev[operation.EvalType]
		if !ok {
			return nil, fmt.Errorf("unsupported operation evaltype %d", operation.EvalType)
		}
		out := map[string]interface{}{
			"escalation_period":         operation.EscPeriod,
			"escalation_step_from":      operation.EscStepFrom,
			"escalation_step_to":        operation.EscStepTo,
			"condition_evaluation_type": evalName,
		}
		conditions := make([]interface{}, len(operation.OpConditions))
		for j, condition := range operation.OpConditions {
			if condition.ConditionType != 14 || condition.Operator != 0 || (condition.Value != "0" && condition.Value != "1") {
				return nil, fmt.Errorf("unsupported operation condition type=%d operator=%d value=%q", condition.ConditionType, condition.Operator, condition.Value)
			}
			conditions[j] = map[string]interface{}{"acknowledged": condition.Value == "1"}
		}
		out["condition"] = conditions
		if err := flattenActionOperationInto(out, operation); err != nil {
			return nil, fmt.Errorf("problem operation %d: %w", i+1, err)
		}
		result[i] = out
	}
	return result, nil
}

func flattenActionRecoveryOperations(operations zabbix.ActionOperations, update bool) ([]interface{}, error) {
	result := make([]interface{}, len(operations))
	for i, operation := range operations {
		out := map[string]interface{}{"notify_all_involved": false}
		notifyType := 11
		operationGroup := "recovery"
		if update {
			notifyType = 12
			operationGroup = "update"
		}
		if operation.OperationType == notifyType {
			if operation.OpMessage == nil {
				return nil, fmt.Errorf("notify-all operation %d has no opmessage", i+1)
			}
			message, err := flattenActionMessage(operation.OpMessage, update)
			if err != nil {
				return nil, fmt.Errorf("%s operation %d: %w", operationGroup, i+1, err)
			}
			out["notify_all_involved"] = true
			if operation.OpMessage.UseDefault != 1 || update && operation.OpMessage.MediaTypeID != "" && operation.OpMessage.MediaTypeID != "0" {
				out["notify_all_message"] = []interface{}{message}
			}
		} else {
			if operation.OperationType != 0 && operation.OperationType != 1 {
				return nil, fmt.Errorf("unsupported operation type %d", operation.OperationType)
			}
			if err := flattenActionOperationInto(out, operation); err != nil {
				return nil, fmt.Errorf("operation %d: %w", i+1, err)
			}
		}
		result[i] = out
	}
	return result, nil
}

func flattenActionOperationInto(out map[string]interface{}, operation zabbix.ActionOperation) error {
	switch operation.OperationType {
	case 0:
		if operation.OpMessage == nil {
			return errors.New("send-message operation has no opmessage")
		}
		message, err := flattenActionMessage(operation.OpMessage, true)
		if err != nil {
			return err
		}
		groups := make([]interface{}, len(operation.MessageGroups))
		for i, group := range operation.MessageGroups {
			groups[i] = group.UserGroupID
		}
		users := make([]interface{}, len(operation.MessageUsers))
		for i, user := range operation.MessageUsers {
			users[i] = user.UserID
		}
		if len(groups)+len(users) == 0 {
			return errors.New("send-message operation has no recipients")
		}
		if len(groups) > 0 {
			message["user_group_ids"] = groups
		}
		if len(users) > 0 {
			message["user_ids"] = users
		}
		out["send_message"] = []interface{}{message}
	case 1:
		if operation.OpCommand == nil || operation.OpCommand.ScriptID == "" || operation.OpCommand.ScriptID == "0" {
			return errors.New("remote-command operation has no global script ID")
		}
		command := map[string]interface{}{
			"current_host":  false,
			"global_script": []interface{}{map[string]interface{}{"script_id": operation.OpCommand.ScriptID}},
		}
		hosts := make([]interface{}, 0, len(operation.CommandHosts))
		for _, host := range operation.CommandHosts {
			if host.HostID == "0" {
				command["current_host"] = true
			} else {
				hosts = append(hosts, host.HostID)
			}
		}
		groups := make([]interface{}, len(operation.CommandGroups))
		for i, group := range operation.CommandGroups {
			groups[i] = group.GroupID
		}
		if command["current_host"] == false && len(hosts)+len(groups) == 0 {
			return errors.New("remote-command operation has no targets")
		}
		if len(hosts) > 0 {
			command["host_ids"] = hosts
		}
		if len(groups) > 0 {
			command["host_group_ids"] = groups
		}
		out["remote_command"] = []interface{}{command}
	default:
		return fmt.Errorf("unsupported operation type %d", operation.OperationType)
	}
	return nil
}

func flattenActionMessage(message *zabbix.ActionOperationMessage, mediaTypeAllowed bool) (map[string]interface{}, error) {
	useDefault, err := actionBooleanState("operation default_msg", message.UseDefault)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{"use_default_message": useDefault}
	if !useDefault {
		out["subject"] = message.Subject
		out["message"] = message.Message
	}
	if mediaTypeAllowed {
		out["media_type_id"] = message.MediaTypeID
	}
	return out, nil
}

func resourceActionTriggerCreate(d *schema.ResourceData, meta interface{}) error {
	action, err := buildActionTrigger(d)
	if err != nil {
		return err
	}
	actions := []zabbix.Action{action}
	if err := meta.(*zabbix.API).ActionsCreate(actions); err != nil {
		return err
	}
	d.SetId(actions[0].ActionID)
	return resourceActionTriggerRead(d, meta)
}

func resourceActionTriggerRead(d *schema.ResourceData, meta interface{}) error {
	action, err := meta.(*zabbix.API).ActionGetByID(d.Id())
	if err != nil {
		return err
	}
	if action == nil {
		return fmt.Errorf("managed trigger action %s was not returned by action.get; it may still exist but be hidden by API-user permissions", d.Id())
	}
	action.Filter.Conditions = alignActionConditionOrder(action.Filter.Conditions, d.Get("filter"))
	action.Operations = alignActionOperationOrder(action.Operations, d.Get("operations"), 0)
	action.RecoveryOperations = alignActionOperationOrder(action.RecoveryOperations, d.Get("recovery_operations"), 11)
	action.UpdateOperations = alignActionOperationOrder(action.UpdateOperations, d.Get("update_operations"), 12)
	state, err := flattenActionTrigger(*action)
	if err != nil {
		return fmt.Errorf("trigger action %s cannot be represented safely: %w", d.Id(), err)
	}
	for key, value := range state {
		if err := d.Set(key, value); err != nil {
			return fmt.Errorf("set trigger action %s state %s: %w", d.Id(), key, err)
		}
	}
	return nil
}

func alignActionConditionOrder(conditions zabbix.ActionConditions, raw interface{}) zabbix.ActionConditions {
	configured, err := buildActionFilter(raw)
	if err != nil || len(configured.Conditions) != len(conditions) {
		return conditions
	}

	remaining := append(zabbix.ActionConditions(nil), conditions...)
	aligned := make(zabbix.ActionConditions, len(conditions))
	for i, wanted := range configured.Conditions {
		match := -1
		for j, candidate := range remaining {
			if actionConditionsEqual(wanted, candidate) && wanted.FormulaID != "" && wanted.FormulaID == candidate.FormulaID {
				match = j
				break
			}
		}
		if match == -1 {
			for j, candidate := range remaining {
				if actionConditionsEqual(wanted, candidate) {
					match = j
					break
				}
			}
		}
		if match == -1 {
			return conditions
		}
		aligned[i] = remaining[match]
		remaining = append(remaining[:match], remaining[match+1:]...)
	}
	return aligned
}

func actionConditionsEqual(left, right zabbix.ActionCondition) bool {
	return left.ConditionType == right.ConditionType &&
		left.Operator == right.Operator &&
		actionConditionValueEqual(left.Value, right.Value) &&
		actionConditionValueEqual(left.Value2, right.Value2)
}

func actionConditionValueEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func alignActionOperationOrder(operations zabbix.ActionOperations, raw interface{}, notifyType int) zabbix.ActionOperations {
	configuredItems := actionOperationItems(raw)
	if len(configuredItems) == 0 {
		if notifyType == 0 {
			return operations
		}
		aligned := append(zabbix.ActionOperations(nil), operations...)
		sort.SliceStable(aligned, func(i, j int) bool {
			return aligned[i].OperationType != notifyType && aligned[j].OperationType == notifyType
		})
		return aligned
	}
	if len(configuredItems) != len(operations) {
		return operations
	}
	byType := make(map[int]zabbix.ActionOperations)
	for _, operation := range operations {
		byType[operation.OperationType] = append(byType[operation.OperationType], operation)
	}
	uniqueTypes := true
	for _, matching := range byType {
		if len(matching) > 1 {
			uniqueTypes = false
			break
		}
	}
	if uniqueTypes {
		aligned := make(zabbix.ActionOperations, len(operations))
		for i, rawOperation := range configuredItems {
			block, ok := rawOperation.(map[string]interface{})
			if !ok {
				return operations
			}
			operationType := 1
			if hasBlock(block["send_message"]) {
				operationType = 0
			} else if notifyType != 0 {
				notifyAll, ok := block["notify_all_involved"].(bool)
				if !ok {
					return operations
				}
				if notifyAll {
					operationType = notifyType
				}
			}
			matching := byType[operationType]
			if len(matching) == 0 {
				return operations
			}
			aligned[i] = matching[0]
			delete(byType, operationType)
		}
		return aligned
	}

	var configured zabbix.ActionOperations
	var err error
	switch notifyType {
	case 0:
		configured, err = buildActionProblemOperations(raw)
	case 11:
		configured, err = buildActionRecoveryOperations(raw, false)
	case 12:
		configured, err = buildActionRecoveryOperations(raw, true)
	default:
		return operations
	}
	if err != nil || len(configured) != len(operations) {
		return operations
	}

	remaining := append(zabbix.ActionOperations(nil), operations...)
	aligned := make(zabbix.ActionOperations, len(operations))
	for i, wanted := range configured {
		match := -1
		for j, candidate := range remaining {
			if actionOperationsEqual(wanted, candidate) {
				match = j
				break
			}
		}
		if match == -1 {
			return operations
		}
		aligned[i] = remaining[match]
		remaining = append(remaining[:match], remaining[match+1:]...)
	}
	return aligned
}

func actionOperationsEqual(left, right zabbix.ActionOperation) bool {
	return left.OperationType == right.OperationType &&
		left.EscPeriod == right.EscPeriod &&
		left.EscStepFrom == right.EscStepFrom &&
		left.EscStepTo == right.EscStepTo &&
		left.EvalType == right.EvalType &&
		actionOperationMessagesEqual(left.OpMessage, right.OpMessage) &&
		actionOperationCommandsEqual(left.OpCommand, right.OpCommand) &&
		actionOperationValuesEqual(operationConditionValues(left.OpConditions), operationConditionValues(right.OpConditions)) &&
		actionOperationValuesEqual(messageGroupValues(left.MessageGroups), messageGroupValues(right.MessageGroups)) &&
		actionOperationValuesEqual(messageUserValues(left.MessageUsers), messageUserValues(right.MessageUsers)) &&
		actionOperationValuesEqual(commandHostValues(left.CommandHosts), commandHostValues(right.CommandHosts)) &&
		actionOperationValuesEqual(commandGroupValues(left.CommandGroups), commandGroupValues(right.CommandGroups))
}

func actionOperationMessagesEqual(left, right *zabbix.ActionOperationMessage) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if left.UseDefault != right.UseDefault || left.MediaTypeID != right.MediaTypeID {
		return false
	}
	return left.UseDefault == 1 || left.Subject == right.Subject && left.Message == right.Message
}

func actionOperationCommandsEqual(left, right *zabbix.ActionOperationCommand) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func actionOperationValuesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	sort.Strings(left)
	sort.Strings(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func operationConditionValues(conditions zabbix.ActionOperationConditions) []string {
	values := make([]string, len(conditions))
	for i, condition := range conditions {
		values[i] = fmt.Sprintf("%d:%d:%s", condition.ConditionType, condition.Operator, condition.Value)
	}
	return values
}

func messageGroupValues(groups zabbix.ActionMessageGroups) []string {
	values := make([]string, len(groups))
	for i, group := range groups {
		values[i] = group.UserGroupID
	}
	return values
}

func messageUserValues(users zabbix.ActionMessageUsers) []string {
	values := make([]string, len(users))
	for i, user := range users {
		values[i] = user.UserID
	}
	return values
}

func commandHostValues(hosts zabbix.ActionCommandHosts) []string {
	values := make([]string, len(hosts))
	for i, host := range hosts {
		values[i] = host.HostID
	}
	return values
}

func commandGroupValues(groups zabbix.ActionCommandGroups) []string {
	values := make([]string, len(groups))
	for i, group := range groups {
		values[i] = group.GroupID
	}
	return values
}

func resourceActionTriggerUpdate(d *schema.ResourceData, meta interface{}) error {
	action, err := buildActionTrigger(d)
	if err != nil {
		return err
	}
	if err := meta.(*zabbix.API).ActionsUpdate([]zabbix.Action{action}); err != nil {
		return err
	}
	return resourceActionTriggerRead(d, meta)
}

func resourceActionTriggerDelete(d *schema.ResourceData, meta interface{}) error {
	if err := meta.(*zabbix.API).ActionsDeleteByIDs([]string{d.Id()}); err != nil {
		return err
	}
	d.SetId("")
	return nil
}
