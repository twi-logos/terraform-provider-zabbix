package zabbix

import (
	"reflect"
	"testing"
)

func stringPointer(value string) *string {
	return &value
}

func TestActionGlobalScriptWriteParamsContainOnlyScriptID(t *testing.T) {
	operation := ActionOperation{
		OperationType: 1,
		OpCommand:     &ActionOperationCommand{ScriptID: "57"},
		CommandHosts:  ActionCommandHosts{{HostID: "0"}, {HostID: "10705"}},
		CommandGroups: ActionCommandGroups{{GroupID: "12"}},
	}

	for _, test := range []struct {
		name  string
		group actionOperationGroup
	}{
		{name: "problem", group: actionProblemOperations},
		{name: "recovery", group: actionRecoveryOperations},
		{name: "update", group: actionUpdateOperations},
	} {
		t.Run(test.name, func(t *testing.T) {
			params := actionOperationWriteParams(operation, test.group)
			wantCommand := Params{"scriptid": "57"}
			if !reflect.DeepEqual(params["opcommand"], wantCommand) {
				t.Fatalf("opcommand = %#v, want %#v", params["opcommand"], wantCommand)
			}
			if got := params["opcommand"].(Params); len(got) != 1 {
				t.Fatalf("opcommand has %d keys, want exactly scriptid", len(got))
			}
			wantHosts := []Params{{"hostid": "0"}, {"hostid": "10705"}}
			if !reflect.DeepEqual(params["opcommand_hst"], wantHosts) {
				t.Errorf("opcommand_hst = %#v, want %#v", params["opcommand_hst"], wantHosts)
			}
			wantGroups := []Params{{"groupid": "12"}}
			if !reflect.DeepEqual(params["opcommand_grp"], wantGroups) {
				t.Errorf("opcommand_grp = %#v, want %#v", params["opcommand_grp"], wantGroups)
			}
		})
	}
}

func TestActionProblemSuppressedWriteOmitsValues(t *testing.T) {
	for _, operator := range []int{10, 11} {
		filter := ActionFilter{
			Conditions: ActionConditions{{
				ConditionType: 16,
				Operator:      operator,
				Value:         stringPointer(""),
				Value2:        stringPointer("unexpected"),
			}},
		}

		condition := actionFilterWriteParams(filter)["conditions"].([]Params)[0]
		if _, present := condition["value"]; present {
			t.Fatalf("operator %d contains value: %#v", operator, condition)
		}
		if _, present := condition["value2"]; present {
			t.Fatalf("operator %d contains value2: %#v", operator, condition)
		}
	}
}

func TestActionEventTagValueWritePreservesFieldOrder(t *testing.T) {
	filter := ActionFilter{
		Conditions: ActionConditions{{
			ConditionType: 26,
			Operator:      0,
			Value:         stringPointer("availability"),
			Value2:        stringPointer("Ephemeral Ports"),
		}},
	}

	condition := actionFilterWriteParams(filter)["conditions"].([]Params)[0]
	if condition["value"] != "availability" || condition["value2"] != "Ephemeral Ports" {
		t.Fatalf("event-tag-value fields reversed: %#v", condition)
	}
}

func TestActionNotifyAllWriteKeepsGroupAndMessageState(t *testing.T) {
	recovery := ActionOperation{
		OperationType: 11,
		OpMessage: &ActionOperationMessage{
			UseDefault:  0,
			Subject:     "Recovered",
			Message:     "Problem recovered",
			MediaTypeID: "5",
		},
	}
	update := ActionOperation{
		OperationType: 12,
		OpMessage: &ActionOperationMessage{
			UseDefault:  0,
			Subject:     "Updated",
			Message:     "Problem updated",
			MediaTypeID: "5",
		},
	}

	recoveryParams := actionOperationWriteParams(recovery, actionRecoveryOperations)
	if recoveryParams["operationtype"] != 11 {
		t.Fatalf("recovery operation type = %v, want 11", recoveryParams["operationtype"])
	}
	recoveryMessage := recoveryParams["opmessage"].(Params)
	if _, present := recoveryMessage["mediatypeid"]; present {
		t.Fatalf("recovery notify-all must not contain mediatypeid: %#v", recoveryMessage)
	}
	if recoveryMessage["subject"] != "Recovered" || recoveryMessage["message"] != "Problem recovered" {
		t.Fatalf("recovery message lost: %#v", recoveryMessage)
	}

	updateParams := actionOperationWriteParams(update, actionUpdateOperations)
	if updateParams["operationtype"] != 12 {
		t.Fatalf("update operation type = %v, want 12", updateParams["operationtype"])
	}
	updateMessage := updateParams["opmessage"].(Params)
	if updateMessage["mediatypeid"] != "5" {
		t.Fatalf("update mediatypeid = %v, want 5", updateMessage["mediatypeid"])
	}
	if updateMessage["subject"] != "Updated" || updateMessage["message"] != "Problem updated" {
		t.Fatalf("update message lost: %#v", updateMessage)
	}
}

func TestActionReadGlobalScriptWithoutInnerType(t *testing.T) {
	in := actionOperationResponse{
		OperationType: 1,
		OpCommand:     &actionOperationCommandResponse{ScriptID: "57"},
		CommandHosts:  []actionCommandHostResponse{{HostID: "10705"}},
	}

	operation := actionOperationsFromResponse([]actionOperationResponse{in})[0]
	if operation.OpCommand == nil || operation.OpCommand.ScriptID != "57" {
		t.Fatalf("global script not preserved: %#v", operation.OpCommand)
	}
	if len(operation.CommandHosts) != 1 || operation.CommandHosts[0].HostID != "10705" {
		t.Fatalf("command targets not preserved: %#v", operation.CommandHosts)
	}
}

func TestActionReadPreservesPauseAndOperationConditions(t *testing.T) {
	action := actionFromResponse(actionResponse{
		PauseSuppressed: 1,
		PauseSymptoms:   1,
		Operations: []actionOperationResponse{{
			EvalType: 1,
			OpConditions: []actionOperationConditionResponse{
				{ConditionType: 14, Operator: 0, Value: "0"},
				{ConditionType: 14, Operator: 0, Value: "1"},
			},
		}},
	}, V64)
	if action.PauseSuppressed != 1 || action.PauseSymptoms != 1 {
		t.Fatalf("pause fields = suppressed:%d symptoms:%d", action.PauseSuppressed, action.PauseSymptoms)
	}
	if len(action.Operations) != 1 || len(action.Operations[0].OpConditions) != 2 {
		t.Fatalf("operation conditions lost: %#v", action.Operations)
	}
}

func TestActionPauseSymptomsVersionHandling(t *testing.T) {
	oldVersionAction := Action{PauseSymptoms: 1}
	params, err := actionWriteParamsForVersion(oldVersionAction, 60000)
	if err != nil {
		t.Fatalf("write 6.0 default: %v", err)
	}
	if _, present := params["pause_symptoms"]; present {
		t.Fatalf("6.0 write contains unsupported pause_symptoms: %#v", params)
	}

	oldVersionAction.PauseSymptoms = 0
	if _, err := actionWriteParamsForVersion(oldVersionAction, 60000); err == nil {
		t.Fatal("6.0 write accepted pause_symptoms = false")
	}

	params, err = actionWriteParamsForVersion(Action{PauseSymptoms: 0}, V64)
	if err != nil {
		t.Fatalf("write 6.4 value: %v", err)
	}
	if params["pause_symptoms"] != 0 {
		t.Fatalf("6.4 pause_symptoms = %#v, want 0", params["pause_symptoms"])
	}

	oldRead := actionFromResponse(actionResponse{PauseSymptoms: 0}, 60000)
	if oldRead.PauseSymptoms != 1 {
		t.Fatalf("6.0 read pause_symptoms = %d, want default 1", oldRead.PauseSymptoms)
	}
	newRead := actionFromResponse(actionResponse{PauseSymptoms: 0}, V64)
	if newRead.PauseSymptoms != 0 {
		t.Fatalf("6.4 read pause_symptoms = %d, want server value 0", newRead.PauseSymptoms)
	}
}

func TestActionWritePreservesPauseAndOperationConditions(t *testing.T) {
	params := actionWriteParams(Action{
		PauseSuppressed: 1,
		PauseSymptoms:   1,
		Operations: ActionOperations{{
			EvalType: 1,
			OpConditions: ActionOperationConditions{
				{ConditionType: 14, Operator: 0, Value: "0"},
				{ConditionType: 14, Operator: 0, Value: "1"},
			},
		}},
	})
	if params["pause_suppressed"] != 1 || params["pause_symptoms"] != 1 {
		t.Fatalf("pause fields = suppressed:%v symptoms:%v", params["pause_suppressed"], params["pause_symptoms"])
	}
	operation := params["operations"].([]Params)[0]
	want := []Params{
		{"conditiontype": 14, "operator": 0, "value": "0"},
		{"conditiontype": 14, "operator": 0, "value": "1"},
	}
	if operation["evaltype"] != 1 || !reflect.DeepEqual(operation["opconditions"], want) {
		t.Fatalf("operation condition params = %#v", operation)
	}
}

func TestActionDefaultNotifyAllMessageRoundTrip(t *testing.T) {
	operation := actionOperationsFromResponse([]actionOperationResponse{{
		OperationType: 12,
		OpMessage:     &actionOperationMessageResponse{UseDefault: 1, MediaTypeID: "5"},
	}})[0]
	params := actionOperationWriteParams(operation, actionUpdateOperations)
	want := Params{"default_msg": 1, "mediatypeid": "5"}
	if !reflect.DeepEqual(params["opmessage"], want) {
		t.Fatalf("default notify-all message = %#v, want %#v", params["opmessage"], want)
	}
}

func TestActionResponseToWriteOmitsRecipientsForNonMessageOperations(t *testing.T) {
	tests := []struct {
		name      string
		group     actionOperationGroup
		operation actionOperationResponse
	}{
		{
			name:  "problem global script",
			group: actionProblemOperations,
			operation: actionOperationResponse{
				OperationType: 1,
				OpCommand:     &actionOperationCommandResponse{ScriptID: "57"},
				CommandHosts:  []actionCommandHostResponse{{HostID: "0"}},
			},
		},
		{
			name:  "recovery notify all",
			group: actionRecoveryOperations,
			operation: actionOperationResponse{
				OperationType: 11,
				OpMessage:     &actionOperationMessageResponse{UseDefault: 1},
			},
		},
		{
			name:  "update notify all",
			group: actionUpdateOperations,
			operation: actionOperationResponse{
				OperationType: 12,
				OpMessage:     &actionOperationMessageResponse{UseDefault: 1},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := actionOperationsFromResponse([]actionOperationResponse{test.operation})[0]
			params := actionOperationWriteParams(operation, test.group)
			if _, present := params["opmessage_grp"]; present {
				t.Fatalf("opmessage_grp must be absent: %#v", params)
			}
			if _, present := params["opmessage_usr"]; present {
				t.Fatalf("opmessage_usr must be absent: %#v", params)
			}
		})
	}
}

func TestActionIDsFromResponseRejectsMalformedSuccess(t *testing.T) {
	tests := []Response{
		{Result: []interface{}{}},
		{Result: map[string]interface{}{}},
		{Result: map[string]interface{}{"actionids": []interface{}{float64(1)}}},
		{Result: map[string]interface{}{"actionids": []interface{}{"1", "2"}}},
	}
	for i, response := range tests {
		if _, err := actionIDsFromResponse(response, 1); err == nil {
			t.Errorf("case %d: expected malformed response error", i)
		}
	}
}
