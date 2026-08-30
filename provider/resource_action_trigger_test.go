package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/tpretz/terraform-provider-zabbix/internal/zabbix"
)

func TestResourceActionTriggerInternalValidate(t *testing.T) {
	if err := resourceActionTrigger().InternalValidate(nil, true); err != nil {
		t.Fatalf("resource schema is invalid: %v", err)
	}
}

func TestBuildActionTriggerGlobalScript(t *testing.T) {
	data := schema.TestResourceDataRaw(t, resourceActionTrigger().Schema, validActionTriggerConfig())
	action, err := buildActionTrigger(data)
	if err != nil {
		t.Fatalf("build action: %v", err)
	}

	if len(action.Operations) != 1 {
		t.Fatalf("operation count = %d, want 1", len(action.Operations))
	}
	operation := action.Operations[0]
	if operation.OperationType != 1 {
		t.Fatalf("operation type = %d, want 1", operation.OperationType)
	}
	if operation.OpCommand == nil || operation.OpCommand.ScriptID != "57" {
		t.Fatalf("global script = %#v, want script ID 57", operation.OpCommand)
	}
	if len(operation.CommandHosts) != 1 || operation.CommandHosts[0].HostID != "0" {
		t.Fatalf("command hosts = %#v, want current host target", operation.CommandHosts)
	}
	if action.PauseSymptoms != 1 {
		t.Fatalf("pause symptoms = %d, want 1", action.PauseSymptoms)
	}
}

func TestBuildActionTriggerPreservesNotifyAllMessageState(t *testing.T) {
	config := validActionTriggerConfig()
	config["operations"] = []interface{}{}
	config["recovery_operations"] = []interface{}{
		map[string]interface{}{
			"notify_all_involved": true,
			"notify_all_message": []interface{}{
				map[string]interface{}{
					"use_default_message": false,
					"subject":             "Recovered",
					"message":             "Problem recovered",
				},
			},
		},
	}
	config["update_operations"] = []interface{}{
		map[string]interface{}{
			"notify_all_involved": true,
			"notify_all_message": []interface{}{
				map[string]interface{}{
					"use_default_message": false,
					"subject":             "Updated",
					"message":             "Problem updated",
					"media_type_id":       "7",
				},
			},
		},
	}

	data := schema.TestResourceDataRaw(t, resourceActionTrigger().Schema, config)
	action, err := buildActionTrigger(data)
	if err != nil {
		t.Fatalf("build action: %v", err)
	}

	recovery := action.RecoveryOperations[0]
	if recovery.OperationType != 11 || recovery.OpMessage == nil {
		t.Fatalf("recovery operation = %#v, want notify-all type 11", recovery)
	}
	if recovery.OpMessage.MediaTypeID != "" || recovery.OpMessage.Subject != "Recovered" {
		t.Fatalf("recovery message = %#v", recovery.OpMessage)
	}
	update := action.UpdateOperations[0]
	if update.OperationType != 12 || update.OpMessage == nil {
		t.Fatalf("update operation = %#v, want notify-all type 12", update)
	}
	if update.OpMessage.MediaTypeID != "7" || update.OpMessage.Message != "Problem updated" {
		t.Fatalf("update message = %#v", update.OpMessage)
	}

	state, err := flattenActionTrigger(action)
	if err != nil {
		t.Fatalf("flatten action: %v", err)
	}
	flattenedRecovery := state["recovery_operations"].([]interface{})[0].(map[string]interface{})
	message := flattenedRecovery["notify_all_message"].([]interface{})[0].(map[string]interface{})
	if message["subject"] != "Recovered" || message["message"] != "Problem recovered" {
		t.Fatalf("flattened recovery message = %#v", message)
	}
	if _, exists := message["media_type_id"]; exists {
		t.Fatalf("recovery notify-all message must not contain media_type_id: %#v", message)
	}
}

func TestActionTriggerOperationConditionsRoundTrip(t *testing.T) {
	config := validActionTriggerConfig()
	config["operations"] = []interface{}{map[string]interface{}{
		"condition_evaluation_type": "or",
		"condition": []interface{}{
			map[string]interface{}{"acknowledged": false},
			map[string]interface{}{"acknowledged": true},
		},
		"send_message": []interface{}{map[string]interface{}{
			"user_ids": []interface{}{"9"},
		}},
	}}

	data := schema.TestResourceDataRaw(t, resourceActionTrigger().Schema, config)
	action, err := buildActionTrigger(data)
	if err != nil {
		t.Fatalf("build action: %v", err)
	}
	state, err := flattenActionTrigger(action)
	if err != nil {
		t.Fatalf("flatten action: %v", err)
	}
	rebuiltData := schema.TestResourceDataRaw(t, resourceActionTrigger().Schema, state)
	rebuilt, err := buildActionTrigger(rebuiltData)
	if err != nil {
		t.Fatalf("rebuild action: %v", err)
	}

	if len(rebuilt.Operations) != 1 || rebuilt.Operations[0].EvalType != 2 {
		t.Fatalf("rebuilt operations = %#v, want one operation with evaltype 2", rebuilt.Operations)
	}
	conditions := map[string]bool{}
	for _, condition := range rebuilt.Operations[0].OpConditions {
		if condition.ConditionType != 14 || condition.Operator != 0 {
			t.Fatalf("rebuilt condition = %#v, want type 14 operator 0", condition)
		}
		conditions[condition.Value] = true
	}
	if !reflect.DeepEqual(conditions, map[string]bool{"0": true, "1": true}) {
		t.Fatalf("rebuilt condition values = %#v, want acknowledged and unacknowledged", conditions)
	}
}

func TestBuildActionNotifyAllMessageDefaultsUpdateMediaType(t *testing.T) {
	message, err := buildActionNotifyAllMessage([]interface{}{}, true)
	if err != nil {
		t.Fatalf("build notify-all message: %v", err)
	}
	if message.UseDefault != 1 || message.MediaTypeID != "0" {
		t.Fatalf("notify-all message = %#v, want default message and media type 0", message)
	}
}

func TestFlattenActionFilterOmitsGeneratedLabelsOutsideCustomExpressions(t *testing.T) {
	value := "trigger"
	filter, err := flattenActionFilter(zabbix.ActionFilter{
		EvalType: 0,
		Conditions: zabbix.ActionConditions{{
			ConditionType: 3,
			Operator:      2,
			Value:         &value,
			FormulaID:     "A",
		}},
	})
	if err != nil {
		t.Fatalf("flatten filter: %v", err)
	}
	condition := filter["condition"].([]interface{})[0].(map[string]interface{})
	if _, exists := condition["label"]; exists {
		t.Fatalf("non-custom condition contains server-generated label: %#v", condition)
	}
}

func TestFlattenActionMessageOmitsIgnoredDefaultText(t *testing.T) {
	message, err := flattenActionMessage(&zabbix.ActionOperationMessage{
		UseDefault:  1,
		Subject:     "stale subject",
		Message:     "stale message",
		MediaTypeID: "0",
	}, true)
	if err != nil {
		t.Fatalf("flatten message: %v", err)
	}
	if _, exists := message["subject"]; exists {
		t.Fatalf("default message contains ignored subject: %#v", message)
	}
	if _, exists := message["message"]; exists {
		t.Fatalf("default message contains ignored body: %#v", message)
	}
	if message["media_type_id"] != "0" {
		t.Fatalf("default message media type = %#v, want 0", message["media_type_id"])
	}

	message, err = flattenActionMessage(&zabbix.ActionOperationMessage{
		UseDefault: 0,
		Subject:    "custom subject",
		Message:    "custom message",
	}, false)
	if err != nil {
		t.Fatalf("flatten custom message: %v", err)
	}
	if message["use_default_message"] != false || message["subject"] != "custom subject" || message["message"] != "custom message" {
		t.Fatalf("custom message = %#v, want custom subject and body", message)
	}
}

func TestAlignActionOperationOrderUsesConfiguredTypes(t *testing.T) {
	server := zabbix.ActionOperations{
		{OperationType: 11},
		{OperationType: 0},
		{OperationType: 1},
	}
	configured := []interface{}{
		map[string]interface{}{"notify_all_involved": false, "send_message": []interface{}{map[string]interface{}{}}},
		map[string]interface{}{"notify_all_involved": false, "remote_command": []interface{}{map[string]interface{}{}}},
		map[string]interface{}{"notify_all_involved": true},
	}

	aligned := alignActionOperationOrder(server, configured, 11)
	want := []int{0, 1, 11}
	for i, operation := range aligned {
		if operation.OperationType != want[i] {
			t.Fatalf("operation %d type = %d, want %d", i, operation.OperationType, want[i])
		}
	}
}

func TestAlignActionOperationOrderUsesCanonicalImportOrder(t *testing.T) {
	server := zabbix.ActionOperations{
		{OperationType: 11},
		{OperationType: 0},
		{OperationType: 1},
	}

	aligned := alignActionOperationOrder(server, nil, 11)
	want := []int{0, 1, 11}
	for i, operation := range aligned {
		if operation.OperationType != want[i] {
			t.Fatalf("operation %d type = %d, want %d", i, operation.OperationType, want[i])
		}
	}
	if server[0].OperationType != 11 {
		t.Fatalf("server operations mutated to %#v", server)
	}

	problem := zabbix.ActionOperations{{OperationType: 1}, {OperationType: 0}}
	aligned = alignActionOperationOrder(problem, nil, 0)
	if aligned[0].OperationType != 1 || aligned[1].OperationType != 0 {
		t.Fatalf("problem operation types = [%d, %d], want server order [1, 0]", aligned[0].OperationType, aligned[1].OperationType)
	}
}

func TestAlignActionOperationOrderMatchesSameTypeByContent(t *testing.T) {
	configured := []interface{}{
		map[string]interface{}{
			"escalation_period": "0", "escalation_step_from": 1, "escalation_step_to": 1,
			"condition_evaluation_type": "and_or", "condition": schema.NewSet(schema.HashResource(actionOperationConditionSchema()), nil),
			"send_message": []interface{}{map[string]interface{}{
				"use_default_message": true, "subject": "", "message": "", "media_type_id": "0",
				"user_group_ids": schema.NewSet(schema.HashString, nil),
				"user_ids":       schema.NewSet(schema.HashString, []interface{}{"10"}),
			}},
		},
		map[string]interface{}{
			"escalation_period": "0", "escalation_step_from": 1, "escalation_step_to": 1,
			"condition_evaluation_type": "and_or", "condition": schema.NewSet(schema.HashResource(actionOperationConditionSchema()), nil),
			"send_message": []interface{}{map[string]interface{}{
				"use_default_message": true, "subject": "", "message": "", "media_type_id": "0",
				"user_group_ids": schema.NewSet(schema.HashString, nil),
				"user_ids":       schema.NewSet(schema.HashString, []interface{}{"20"}),
			}},
		},
	}
	server := zabbix.ActionOperations{
		{
			OperationType: 0, EscPeriod: "0", EscStepFrom: 1, EscStepTo: 1, EvalType: 0,
			OpMessage:    &zabbix.ActionOperationMessage{UseDefault: 1, MediaTypeID: "0"},
			MessageUsers: zabbix.ActionMessageUsers{{UserID: "20"}},
		},
		{
			OperationType: 0, EscPeriod: "0", EscStepFrom: 1, EscStepTo: 1, EvalType: 0,
			OpMessage:    &zabbix.ActionOperationMessage{UseDefault: 1, MediaTypeID: "0"},
			MessageUsers: zabbix.ActionMessageUsers{{UserID: "10"}},
		},
	}

	aligned := alignActionOperationOrder(server, configured, 0)
	if got := aligned[0].MessageUsers[0].UserID; got != "10" {
		t.Fatalf("first aligned recipient = %q, want 10", got)
	}
}

func TestActionOperationMessagesEqualIgnoresDefaultText(t *testing.T) {
	configured := &zabbix.ActionOperationMessage{UseDefault: 1, MediaTypeID: "0"}
	server := &zabbix.ActionOperationMessage{
		UseDefault:  1,
		Subject:     "stale subject",
		Message:     "stale message",
		MediaTypeID: "0",
	}
	if !actionOperationMessagesEqual(configured, server) {
		t.Fatalf("default messages differ: configured=%#v server=%#v", configured, server)
	}
	server.MediaTypeID = "3"
	if actionOperationMessagesEqual(configured, server) {
		t.Fatalf("default messages with different media types compare equal: configured=%#v server=%#v", configured, server)
	}
	if actionOperationMessagesEqual(
		&zabbix.ActionOperationMessage{UseDefault: 0, Subject: "a", Message: "body", MediaTypeID: "0"},
		&zabbix.ActionOperationMessage{UseDefault: 0, Subject: "b", Message: "body", MediaTypeID: "0"},
	) {
		t.Fatal("custom messages with different subjects compare equal")
	}
}

func TestAlignActionOperationOrderDoesNotReuseServerOperation(t *testing.T) {
	server := zabbix.ActionOperations{{OperationType: 0}, {OperationType: 1}}
	configured := []interface{}{
		map[string]interface{}{"send_message": []interface{}{map[string]interface{}{}}},
		map[string]interface{}{"send_message": []interface{}{map[string]interface{}{}}},
	}

	aligned := alignActionOperationOrder(server, configured, 0)
	if aligned[0].OperationType != 0 || aligned[1].OperationType != 1 {
		t.Fatalf("operation types = [%d, %d], want original [0, 1]", aligned[0].OperationType, aligned[1].OperationType)
	}
}

func TestAlignActionOperationOrderFallsBackWhenNotifyFlagMissing(t *testing.T) {
	server := zabbix.ActionOperations{{OperationType: 11}}
	configured := []interface{}{map[string]interface{}{
		"notify_all_message": []interface{}{map[string]interface{}{}},
	}}

	aligned := alignActionOperationOrder(server, configured, 11)
	if len(aligned) != 1 || aligned[0].OperationType != 11 {
		t.Fatalf("aligned operations = %#v, want original server order", aligned)
	}
}

func TestBuildActionTriggerRejectsSuppressedConditionValue(t *testing.T) {
	config := validActionTriggerConfig()
	filter := config["filter"].([]interface{})[0].(map[string]interface{})
	filter["condition"] = []interface{}{
		map[string]interface{}{
			"condition_type": "maintenance_status",
			"operator":       "yes",
			"value":          "1",
		},
	}

	data := schema.TestResourceDataRaw(t, resourceActionTrigger().Schema, config)
	_, err := buildActionTrigger(data)
	if err == nil || !strings.Contains(err.Error(), "forbids value") {
		t.Fatalf("error = %v, want suppressed-condition value rejection", err)
	}
}

func TestActionTriggerConditionMappings(t *testing.T) {
	want := map[string]int{
		"host_group": 0, "host": 1, "trigger": 2, "trigger_name": 3,
		"trigger_severity": 4, "time_period": 6, "host_template": 13,
		"maintenance_status": 16, "event_tag": 25, "event_tag_value": 26,
	}
	if !reflect.DeepEqual(actionConditionType, want) {
		t.Fatalf("condition mappings = %#v, want %#v", actionConditionType, want)
	}
	for name, value := range want {
		if got := actionConditionTypeRev[value]; got != name {
			t.Errorf("reverse mapping %d = %q, want %q", value, got, name)
		}
	}
	if _, present := actionConditionType["host_ip"]; present {
		t.Fatal("host_ip must not be supported by trigger actions")
	}
}

func TestActionTriggerConditionMappingsRoundTrip(t *testing.T) {
	tests := map[string]struct {
		conditionType int
		operator      string
	}{
		"time_period":   {conditionType: 6, operator: "in"},
		"host_template": {conditionType: 13, operator: "equals"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := validActionTriggerConfig()
			filter := config["filter"].([]interface{})[0].(map[string]interface{})
			filter["evaluation_type"] = "and_or"
			filter["formula"] = ""
			filter["condition"] = []interface{}{map[string]interface{}{
				"condition_type": name,
				"operator":       test.operator,
				"value":          "42",
			}}

			data := schema.TestResourceDataRaw(t, resourceActionTrigger().Schema, config)
			action, err := buildActionTrigger(data)
			if err != nil {
				t.Fatalf("build action: %v", err)
			}
			if action.Filter.Conditions[0].ConditionType != test.conditionType {
				t.Fatalf("condition type = %d, want %d", action.Filter.Conditions[0].ConditionType, test.conditionType)
			}

			state, err := flattenActionTrigger(action)
			if err != nil {
				t.Fatalf("flatten action: %v", err)
			}
			condition := state["filter"].([]interface{})[0].(map[string]interface{})["condition"].([]interface{})[0].(map[string]interface{})
			if condition["condition_type"] != name || condition["value"] != "42" {
				t.Fatalf("flattened condition = %#v", condition)
			}
		})
	}
}

func TestFlattenActionFilterRejectsUnsupportedConditionType(t *testing.T) {
	value := "unsupported"
	_, err := flattenActionFilter(zabbix.ActionFilter{
		EvalType: 0,
		Conditions: zabbix.ActionConditions{{
			ConditionType: 99,
			Operator:      0,
			Value:         &value,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported action condition type 99") {
		t.Fatalf("error = %v, want unsupported condition diagnostic", err)
	}
}

func TestBuildActionTriggerProblemSuppressedRoundTrip(t *testing.T) {
	for name, operator := range map[string]int{"yes": 10, "no": 11} {
		t.Run(name, func(t *testing.T) {
			config := validActionTriggerConfig()
			filter := config["filter"].([]interface{})[0].(map[string]interface{})
			filter["condition"] = []interface{}{map[string]interface{}{
				"condition_type": "maintenance_status",
				"operator":       name,
			}}

			data := schema.TestResourceDataRaw(t, resourceActionTrigger().Schema, config)
			action, err := buildActionTrigger(data)
			if err != nil {
				t.Fatalf("build action: %v", err)
			}
			condition := action.Filter.Conditions[0]
			if condition.ConditionType != 16 || condition.Operator != operator || condition.Value != nil || condition.Value2 != nil {
				t.Fatalf("condition = %#v, want type 16 operator %d without values", condition, operator)
			}

			state, err := flattenActionTrigger(action)
			if err != nil {
				t.Fatalf("flatten action: %v", err)
			}
			flattened := state["filter"].([]interface{})[0].(map[string]interface{})["condition"].([]interface{})[0].(map[string]interface{})
			if flattened["condition_type"] != "maintenance_status" || flattened["operator"] != name {
				t.Fatalf("flattened condition = %#v", flattened)
			}
			if _, present := flattened["value"]; present {
				t.Fatalf("flattened condition contains value: %#v", flattened)
			}
			if _, present := flattened["value2"]; present {
				t.Fatalf("flattened condition contains value2: %#v", flattened)
			}
		})
	}
}

func TestBuildActionTriggerRejectsMissingOperationRequirements(t *testing.T) {
	tests := []struct {
		name    string
		block   map[string]interface{}
		wantErr string
	}{
		{
			name: "message recipients",
			block: map[string]interface{}{
				"send_message": []interface{}{map[string]interface{}{}},
			},
			wantErr: "requires at least one",
		},
		{
			name: "global script targets",
			block: map[string]interface{}{
				"remote_command": []interface{}{map[string]interface{}{
					"global_script": []interface{}{map[string]interface{}{"script_id": "57"}},
				}},
			},
			wantErr: "requires current_host",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validActionTriggerConfig()
			config["operations"] = []interface{}{test.block}
			data := schema.TestResourceDataRaw(t, resourceActionTrigger().Schema, config)
			_, err := buildActionTrigger(data)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestResourceActionTriggerReadFailsClosedWhenActionMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			ID     int32  `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		var result interface{}
		switch strings.ToLower(request.Method) {
		case "apiinfo.version":
			result = "7.4.14"
		case "action.get":
			result = []interface{}{}
		default:
			t.Errorf("unexpected method %q", request.Method)
			result = nil
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0", "id": request.ID, "result": result,
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	api, err := zabbix.NewAPI(zabbix.Config{Url: server.URL})
	if err != nil {
		t.Fatalf("create API client: %v", err)
	}
	data := schema.TestResourceDataRaw(t, resourceActionTrigger().Schema, map[string]interface{}{})
	data.SetId("117")
	err = resourceActionTriggerRead(data, api)
	if err == nil || !strings.Contains(err.Error(), "may still exist but be hidden") {
		t.Fatalf("error = %v, want fail-closed permission diagnostic", err)
	}
	if data.Id() != "117" {
		t.Fatalf("managed ID was removed from state: %q", data.Id())
	}
}

func TestFlattenActionTriggerRejectsUnknownOperationType(t *testing.T) {
	value := "Linux servers"
	action := zabbix.Action{
		Name:   "Notify Linux failures",
		Status: 0,
		Filter: zabbix.ActionFilter{
			EvalType: 0,
			Conditions: zabbix.ActionConditions{
				{ConditionType: 0, Operator: 0, Value: &value},
			},
		},
		Operations: zabbix.ActionOperations{{OperationType: 99}},
	}

	_, err := flattenActionTrigger(action)
	if err == nil || !strings.Contains(err.Error(), "unsupported problem operation type 99") {
		t.Fatalf("error = %v, want unsupported operation rejection", err)
	}
}

func TestFlattenActionTriggerRejectsUnsupportedBooleanState(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*zabbix.Action)
		wantErr string
	}{
		{name: "pause suppressed", mutate: func(action *zabbix.Action) { action.PauseSuppressed = 2 }, wantErr: "unsupported pause_suppressed value 2"},
		{name: "pause symptoms", mutate: func(action *zabbix.Action) { action.PauseSymptoms = -1 }, wantErr: "unsupported pause_symptoms value -1"},
		{name: "notify if canceled", mutate: func(action *zabbix.Action) { action.NotifyIfCanceled = 3 }, wantErr: "unsupported notify_if_canceled value 3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action := zabbix.Action{Status: 0, Filter: zabbix.ActionFilter{EvalType: 0}}
			test.mutate(&action)
			_, err := flattenActionTrigger(action)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestFlattenActionTriggerRejectsUnsupportedMessageBooleanState(t *testing.T) {
	tests := []struct {
		name      string
		operation func(*zabbix.Action)
		wantErr   string
	}{
		{
			name: "problem message",
			operation: func(action *zabbix.Action) {
				action.Operations = zabbix.ActionOperations{{
					OperationType: 0,
					EvalType:      0,
					OpMessage:     &zabbix.ActionOperationMessage{UseDefault: 2},
					MessageUsers:  []zabbix.ActionMessageUser{{UserID: "1"}},
				}}
			},
			wantErr: "problem operation 1: unsupported operation default_msg value 2",
		},
		{
			name: "recovery notify all message",
			operation: func(action *zabbix.Action) {
				action.RecoveryOperations = zabbix.ActionOperations{{
					OperationType: 11,
					OpMessage:     &zabbix.ActionOperationMessage{UseDefault: 2},
				}}
			},
			wantErr: "recovery operation 1: unsupported operation default_msg value 2",
		},
		{
			name: "update notify all message",
			operation: func(action *zabbix.Action) {
				action.UpdateOperations = zabbix.ActionOperations{{
					OperationType: 12,
					OpMessage:     &zabbix.ActionOperationMessage{UseDefault: 2},
				}}
			},
			wantErr: "update operation 1: unsupported operation default_msg value 2",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action := zabbix.Action{Status: 0, Filter: zabbix.ActionFilter{EvalType: 0}}
			test.operation(&action)
			_, err := flattenActionTrigger(action)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateActionFormula(t *testing.T) {
	labels := map[string]bool{"A": true, "AA": true}
	tests := []struct {
		name    string
		formula string
		wantErr string
	}{
		{name: "multi letter label", formula: "A and (AA)", wantErr: "label \"AA\" must be \"B\""},
		{name: "unknown label", formula: "A and B", wantErr: "configured label"},
		{name: "noncanonical first appearance", formula: "AA and A", wantErr: "label \"AA\" must be \"A\""},
		{name: "braces", formula: "{A} and AA", wantErr: "must not use braces"},
		{name: "missing operator", formula: "A AA", wantErr: "expected and"},
		{name: "unused label", formula: "A", wantErr: "does not reference"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateActionFormula(test.formula, labels)
			if test.wantErr == "" && err != nil {
				t.Fatalf("validate formula: %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestActionFormulaID(t *testing.T) {
	tests := map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 701: "ZZ", 702: "AAA"}
	for index, want := range tests {
		if got := actionFormulaID(index); got != want {
			t.Errorf("actionFormulaID(%d) = %q, want %q", index, got, want)
		}
	}
}

func TestAlignActionConditionOrderUsesConfiguredSemantics(t *testing.T) {
	hostGroup := "42"
	triggerName := "problem"
	server := zabbix.ActionConditions{
		{ConditionType: 3, Operator: 2, Value: &triggerName, FormulaID: "B"},
		{ConditionType: 0, Operator: 0, Value: &hostGroup, FormulaID: "A"},
	}
	configured := []interface{}{map[string]interface{}{
		"evaluation_type": "custom_expression",
		"formula":         "A and B",
		"condition": []interface{}{
			map[string]interface{}{"condition_type": "host_group", "operator": "equals", "value": hostGroup, "label": "A"},
			map[string]interface{}{"condition_type": "trigger_name", "operator": "like", "value": triggerName, "label": "B"},
		},
	}}

	aligned := alignActionConditionOrder(server, configured)
	if aligned[0].ConditionType != 0 || aligned[1].ConditionType != 3 {
		t.Fatalf("condition types = [%d, %d], want [0, 3]", aligned[0].ConditionType, aligned[1].ConditionType)
	}
}

func TestAlignActionConditionOrderUsesLabelsToDisambiguateDuplicates(t *testing.T) {
	value := "same"
	server := zabbix.ActionConditions{
		{ConditionType: 3, Operator: 2, Value: &value, FormulaID: "A"},
		{ConditionType: 3, Operator: 2, Value: &value, FormulaID: "B"},
	}
	configured := []interface{}{map[string]interface{}{
		"evaluation_type": "custom_expression",
		"formula":         "A or B",
		"condition": []interface{}{
			map[string]interface{}{"condition_type": "trigger_name", "operator": "like", "value": value, "label": "B"},
			map[string]interface{}{"condition_type": "trigger_name", "operator": "like", "value": value, "label": "A"},
		},
	}}

	aligned := alignActionConditionOrder(server, configured)
	if aligned[0].FormulaID != "B" || aligned[1].FormulaID != "A" {
		t.Fatalf("formula IDs = [%s, %s], want [B, A]", aligned[0].FormulaID, aligned[1].FormulaID)
	}
}

func TestBuildActionFilterAllowsBlockOrderIndependentOfFormulaOrder(t *testing.T) {
	configured := []interface{}{map[string]interface{}{
		"evaluation_type": "custom_expression",
		"formula":         "A and B",
		"condition": []interface{}{
			map[string]interface{}{"condition_type": "trigger_name", "operator": "like", "value": "second block", "label": "B"},
			map[string]interface{}{"condition_type": "trigger_name", "operator": "like", "value": "first formula", "label": "A"},
		},
	}}

	if _, err := buildActionFilter(configured); err != nil {
		t.Fatalf("build filter: %v", err)
	}
}

func TestActionTriggerSchemaAllowsMatchAllFilter(t *testing.T) {
	filter := resourceActionTrigger().Schema["filter"].Elem.(*schema.Resource)
	conditions := filter.Schema["condition"]
	if !conditions.Optional || conditions.Required || conditions.MinItems != 0 {
		t.Fatalf("condition schema = optional %t, required %t, min items %d; want optional with no minimum", conditions.Optional, conditions.Required, conditions.MinItems)
	}
}

func TestActionFilterAllowsNoConditions(t *testing.T) {
	configured := []interface{}{map[string]interface{}{
		"evaluation_type": "and_or",
		"formula":         "",
		"condition":       []interface{}{},
	}}

	filter, err := buildActionFilter(configured)
	if err != nil {
		t.Fatalf("build filter: %v", err)
	}
	if len(filter.Conditions) != 0 {
		t.Fatalf("built conditions = %#v, want none", filter.Conditions)
	}

	flattened, err := flattenActionFilter(filter)
	if err != nil {
		t.Fatalf("flatten filter: %v", err)
	}
	if conditions := flattened["condition"].([]interface{}); len(conditions) != 0 {
		t.Fatalf("flattened conditions = %#v, want none", conditions)
	}
}

func validActionTriggerConfig() map[string]interface{} {
	return map[string]interface{}{
		"name": "Run diagnostics",
		"filter": []interface{}{
			map[string]interface{}{
				"evaluation_type": "and_or",
				"condition": []interface{}{
					map[string]interface{}{
						"condition_type": "host_group",
						"operator":       "equals",
						"value":          "2",
					},
				},
			},
		},
		"operations": []interface{}{
			map[string]interface{}{
				"remote_command": []interface{}{
					map[string]interface{}{
						"current_host": true,
						"global_script": []interface{}{
							map[string]interface{}{"script_id": "57"},
						},
					},
				},
			},
		},
	}
}
