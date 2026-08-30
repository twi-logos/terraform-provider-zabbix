package zabbix

import "fmt"

// Action is the provider's representation of a trigger action
// (eventsource = 0).
type Action struct {
	ActionID           string
	Name               string
	Status             int
	EscPeriod          string
	PauseSuppressed    int
	PauseSymptoms      int
	NotifyIfCanceled   int
	Filter             ActionFilter
	Operations         ActionOperations
	RecoveryOperations ActionOperations
	UpdateOperations   ActionOperations
}

type ActionFilter struct {
	EvalType   int
	Formula    string
	Conditions ActionConditions
}

type ActionCondition struct {
	ConditionType int
	Operator      int
	Value         *string
	Value2        *string
	FormulaID     string
}

type ActionConditions []ActionCondition

type ActionOperation struct {
	OperationType int
	EscPeriod     string
	EscStepFrom   int
	EscStepTo     int
	EvalType      int
	OpConditions  ActionOperationConditions
	OpMessage     *ActionOperationMessage
	MessageGroups ActionMessageGroups
	MessageUsers  ActionMessageUsers
	OpCommand     *ActionOperationCommand
	CommandHosts  ActionCommandHosts
	CommandGroups ActionCommandGroups
}

type ActionOperations []ActionOperation

type ActionOperationCondition struct {
	ConditionType int
	Operator      int
	Value         string
}

type ActionOperationConditions []ActionOperationCondition

type ActionOperationMessage struct {
	UseDefault  int
	Subject     string
	Message     string
	MediaTypeID string
}

type ActionMessageGroup struct {
	UserGroupID string
}

type ActionMessageGroups []ActionMessageGroup

type ActionMessageUser struct {
	UserID string
}

type ActionMessageUsers []ActionMessageUser

// ActionOperationCommand is a reference to a Zabbix global script. Script
// type, command text and credentials belong to the script object, not to the
// action operation.
type ActionOperationCommand struct {
	ScriptID string
}

type ActionCommandHost struct {
	HostID string
}

type ActionCommandHosts []ActionCommandHost

type ActionCommandGroup struct {
	GroupID string
}

type ActionCommandGroups []ActionCommandGroup

type actionResponse struct {
	ActionID           string                    `json:"actionid"`
	Name               string                    `json:"name"`
	Status             int                       `json:"status,string"`
	EscPeriod          string                    `json:"esc_period"`
	PauseSuppressed    int                       `json:"pause_suppressed,string"`
	PauseSymptoms      int                       `json:"pause_symptoms,string"`
	NotifyIfCanceled   int                       `json:"notify_if_canceled,string"`
	Filter             actionFilterResponse      `json:"filter"`
	Operations         []actionOperationResponse `json:"operations"`
	RecoveryOperations []actionOperationResponse `json:"recovery_operations"`
	UpdateOperations   []actionOperationResponse `json:"update_operations"`
}

type actionFilterResponse struct {
	EvalType   int                       `json:"evaltype,string"`
	Formula    string                    `json:"formula"`
	Conditions []actionConditionResponse `json:"conditions"`
}

type actionConditionResponse struct {
	ConditionType int     `json:"conditiontype,string"`
	Operator      int     `json:"operator,string"`
	Value         *string `json:"value"`
	Value2        *string `json:"value2"`
	FormulaID     string  `json:"formulaid"`
}

type actionOperationResponse struct {
	OperationType int                                `json:"operationtype,string"`
	EscPeriod     string                             `json:"esc_period"`
	EscStepFrom   int                                `json:"esc_step_from,string"`
	EscStepTo     int                                `json:"esc_step_to,string"`
	EvalType      int                                `json:"evaltype,string"`
	OpConditions  []actionOperationConditionResponse `json:"opconditions"`
	OpMessage     *actionOperationMessageResponse    `json:"opmessage"`
	MessageGroups []actionMessageGroupResponse       `json:"opmessage_grp"`
	MessageUsers  []actionMessageUserResponse        `json:"opmessage_usr"`
	OpCommand     *actionOperationCommandResponse    `json:"opcommand"`
	CommandHosts  []actionCommandHostResponse        `json:"opcommand_hst"`
	CommandGroups []actionCommandGroupResponse       `json:"opcommand_grp"`
}

type actionOperationConditionResponse struct {
	ConditionType int    `json:"conditiontype,string"`
	Operator      int    `json:"operator,string"`
	Value         string `json:"value"`
}

type actionOperationMessageResponse struct {
	UseDefault  int    `json:"default_msg,string"`
	Subject     string `json:"subject"`
	Message     string `json:"message"`
	MediaTypeID string `json:"mediatypeid"`
}

type actionMessageGroupResponse struct {
	UserGroupID string `json:"usrgrpid"`
}

type actionMessageUserResponse struct {
	UserID string `json:"userid"`
}

type actionOperationCommandResponse struct {
	ScriptID string `json:"scriptid"`
}

type actionCommandHostResponse struct {
	HostID string `json:"hostid"`
}

type actionCommandGroupResponse struct {
	GroupID string `json:"groupid"`
}

type actionOperationGroup int

const (
	actionProblemOperations actionOperationGroup = iota
	actionRecoveryOperations
	actionUpdateOperations
)

func actionFromResponse(in actionResponse, version int) Action {
	action := Action{
		ActionID:           in.ActionID,
		Name:               in.Name,
		Status:             in.Status,
		EscPeriod:          in.EscPeriod,
		PauseSuppressed:    in.PauseSuppressed,
		PauseSymptoms:      in.PauseSymptoms,
		NotifyIfCanceled:   in.NotifyIfCanceled,
		Filter:             actionFilterFromResponse(in.Filter),
		Operations:         actionOperationsFromResponse(in.Operations),
		RecoveryOperations: actionOperationsFromResponse(in.RecoveryOperations),
		UpdateOperations:   actionOperationsFromResponse(in.UpdateOperations),
	}
	if version < V64 {
		action.PauseSymptoms = 1
	}
	return action
}

func actionFilterFromResponse(in actionFilterResponse) ActionFilter {
	conditions := make(ActionConditions, len(in.Conditions))
	for i, condition := range in.Conditions {
		conditions[i] = ActionCondition{
			ConditionType: condition.ConditionType,
			Operator:      condition.Operator,
			Value:         condition.Value,
			Value2:        condition.Value2,
			FormulaID:     condition.FormulaID,
		}
	}
	return ActionFilter{EvalType: in.EvalType, Formula: in.Formula, Conditions: conditions}
}

func actionOperationsFromResponse(in []actionOperationResponse) ActionOperations {
	operations := make(ActionOperations, len(in))
	for i, operation := range in {
		out := ActionOperation{
			OperationType: operation.OperationType,
			EscPeriod:     operation.EscPeriod,
			EscStepFrom:   operation.EscStepFrom,
			EscStepTo:     operation.EscStepTo,
			EvalType:      operation.EvalType,
		}
		if operation.OpMessage != nil {
			out.OpMessage = &ActionOperationMessage{
				UseDefault:  operation.OpMessage.UseDefault,
				Subject:     operation.OpMessage.Subject,
				Message:     operation.OpMessage.Message,
				MediaTypeID: operation.OpMessage.MediaTypeID,
			}
		}
		if operation.OpCommand != nil {
			out.OpCommand = &ActionOperationCommand{ScriptID: operation.OpCommand.ScriptID}
		}
		out.OpConditions = make(ActionOperationConditions, len(operation.OpConditions))
		for j, condition := range operation.OpConditions {
			out.OpConditions[j] = ActionOperationCondition{
				ConditionType: condition.ConditionType,
				Operator:      condition.Operator,
				Value:         condition.Value,
			}
		}
		out.MessageGroups = make(ActionMessageGroups, len(operation.MessageGroups))
		for j, group := range operation.MessageGroups {
			out.MessageGroups[j] = ActionMessageGroup{UserGroupID: group.UserGroupID}
		}
		out.MessageUsers = make(ActionMessageUsers, len(operation.MessageUsers))
		for j, user := range operation.MessageUsers {
			out.MessageUsers[j] = ActionMessageUser{UserID: user.UserID}
		}
		out.CommandHosts = make(ActionCommandHosts, len(operation.CommandHosts))
		for j, host := range operation.CommandHosts {
			out.CommandHosts[j] = ActionCommandHost{HostID: host.HostID}
		}
		out.CommandGroups = make(ActionCommandGroups, len(operation.CommandGroups))
		for j, group := range operation.CommandGroups {
			out.CommandGroups[j] = ActionCommandGroup{GroupID: group.GroupID}
		}
		operations[i] = out
	}
	return operations
}

func actionWriteParams(action Action) Params {
	return Params{
		"name":                action.Name,
		"status":              action.Status,
		"esc_period":          action.EscPeriod,
		"pause_suppressed":    action.PauseSuppressed,
		"pause_symptoms":      action.PauseSymptoms,
		"notify_if_canceled":  action.NotifyIfCanceled,
		"filter":              actionFilterWriteParams(action.Filter),
		"operations":          actionOperationsWriteParams(action.Operations, actionProblemOperations),
		"recovery_operations": actionOperationsWriteParams(action.RecoveryOperations, actionRecoveryOperations),
		"update_operations":   actionOperationsWriteParams(action.UpdateOperations, actionUpdateOperations),
	}
}

func actionWriteParamsForVersion(action Action, version int) (Params, error) {
	params := actionWriteParams(action)
	if version >= V64 {
		return params, nil
	}
	delete(params, "pause_symptoms")
	if action.PauseSymptoms != 1 {
		return nil, fmt.Errorf("pause_symptoms = false requires Zabbix 6.4 or newer")
	}
	return params, nil
}

func actionFilterWriteParams(filter ActionFilter) Params {
	conditions := make([]Params, len(filter.Conditions))
	for i, condition := range filter.Conditions {
		params := Params{
			"conditiontype": condition.ConditionType,
			"operator":      condition.Operator,
		}
		if condition.ConditionType != 16 {
			if condition.Value != nil {
				params["value"] = *condition.Value
			}
			if condition.Value2 != nil {
				params["value2"] = *condition.Value2
			}
		}
		if filter.EvalType == 3 {
			params["formulaid"] = condition.FormulaID
		}
		conditions[i] = params
	}
	params := Params{"evaltype": filter.EvalType, "conditions": conditions}
	if filter.EvalType == 3 {
		params["formula"] = filter.Formula
	}
	return params
}

func actionOperationsWriteParams(operations ActionOperations, group actionOperationGroup) []Params {
	params := make([]Params, len(operations))
	for i, operation := range operations {
		params[i] = actionOperationWriteParams(operation, group)
	}
	return params
}

func actionOperationWriteParams(operation ActionOperation, group actionOperationGroup) Params {
	params := Params{"operationtype": operation.OperationType}
	if group == actionProblemOperations {
		params["esc_period"] = operation.EscPeriod
		params["esc_step_from"] = operation.EscStepFrom
		params["esc_step_to"] = operation.EscStepTo
		params["evaltype"] = operation.EvalType
		conditions := make([]Params, len(operation.OpConditions))
		for i, condition := range operation.OpConditions {
			conditions[i] = Params{
				"conditiontype": condition.ConditionType,
				"operator":      condition.Operator,
				"value":         condition.Value,
			}
		}
		params["opconditions"] = conditions
	}
	if operation.OpMessage != nil {
		message := Params{"default_msg": operation.OpMessage.UseDefault}
		if operation.OpMessage.UseDefault == 0 {
			message["subject"] = operation.OpMessage.Subject
			message["message"] = operation.OpMessage.Message
		}
		if operation.OpMessage.MediaTypeID != "" && !(group == actionRecoveryOperations && operation.OperationType == 11) {
			message["mediatypeid"] = operation.OpMessage.MediaTypeID
		}
		params["opmessage"] = message
	}
	if operation.OperationType == 0 && operation.MessageGroups != nil {
		groups := make([]Params, len(operation.MessageGroups))
		for i, recipient := range operation.MessageGroups {
			groups[i] = Params{"usrgrpid": recipient.UserGroupID}
		}
		params["opmessage_grp"] = groups
	}
	if operation.OperationType == 0 && operation.MessageUsers != nil {
		users := make([]Params, len(operation.MessageUsers))
		for i, recipient := range operation.MessageUsers {
			users[i] = Params{"userid": recipient.UserID}
		}
		params["opmessage_usr"] = users
	}
	if operation.OpCommand != nil {
		params["opcommand"] = Params{"scriptid": operation.OpCommand.ScriptID}
		hosts := make([]Params, len(operation.CommandHosts))
		for i, target := range operation.CommandHosts {
			hosts[i] = Params{"hostid": target.HostID}
		}
		params["opcommand_hst"] = hosts
		groups := make([]Params, len(operation.CommandGroups))
		for i, target := range operation.CommandGroups {
			groups[i] = Params{"groupid": target.GroupID}
		}
		params["opcommand_grp"] = groups
	}
	return params
}

func (api *API) ActionsGet(params Params) (res []Action, err error) {
	if _, present := params["output"]; !present {
		params["output"] = "extend"
	}
	var wire []actionResponse
	if err = api.CallWithErrorParse("action.get", params, &wire); err != nil {
		return
	}
	res = make([]Action, len(wire))
	for i, action := range wire {
		res[i] = actionFromResponse(action, api.Config.Version)
	}
	return
}

func (api *API) ActionGetByID(id string) (res *Action, err error) {
	actions, err := api.ActionsGet(Params{
		"actionids":                []string{id},
		"selectFilter":             "extend",
		"selectOperations":         "extend",
		"selectRecoveryOperations": "extend",
		"selectUpdateOperations":   "extend",
		"filter":                   Params{"eventsource": []int{0}},
	})
	if err != nil {
		return nil, err
	}
	if len(actions) == 0 {
		return nil, nil
	}
	if len(actions) != 1 {
		expected := ExpectedOneResult(len(actions))
		return nil, &expected
	}
	return &actions[0], nil
}

func (api *API) ActionsGetByName(name string) ([]Action, error) {
	return api.ActionsGet(Params{
		"filter": Params{
			"eventsource": []int{0},
			"name":        []string{name},
		},
		"selectFilter":             "extend",
		"selectOperations":         "extend",
		"selectRecoveryOperations": "extend",
		"selectUpdateOperations":   "extend",
	})
}

func (api *API) ActionsCreate(actions []Action) (err error) {
	params := make([]Params, len(actions))
	for i, action := range actions {
		params[i], err = actionWriteParamsForVersion(action, api.Config.Version)
		if err != nil {
			return err
		}
		params[i]["eventsource"] = 0
	}
	response, err := api.CallWithError("action.create", params)
	if err != nil {
		return err
	}
	ids, err := actionIDsFromResponse(response, len(actions))
	if err != nil {
		return fmt.Errorf("action.create: %w", err)
	}
	for i, id := range ids {
		actions[i].ActionID = id
	}
	return nil
}

func (api *API) ActionsUpdate(actions []Action) (err error) {
	params := make([]Params, len(actions))
	for i, action := range actions {
		params[i], err = actionWriteParamsForVersion(action, api.Config.Version)
		if err != nil {
			return err
		}
		params[i]["actionid"] = action.ActionID
	}
	_, err = api.CallWithError("action.update", params)
	return err
}

func (api *API) ActionsDeleteByIDs(ids []string) (err error) {
	response, err := api.CallWithError("action.delete", ids)
	if err != nil {
		return err
	}
	if _, err := actionIDsFromResponse(response, len(ids)); err != nil {
		return fmt.Errorf("action.delete: %w", err)
	}
	return nil
}

func actionIDsFromResponse(response Response, expected int) ([]string, error) {
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("result has type %T, want object", response.Result)
	}
	rawIDs, ok := result["actionids"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("actionids has type %T, want array", result["actionids"])
	}
	if len(rawIDs) != expected {
		return nil, &ExpectedMore{Expected: expected, Got: len(rawIDs)}
	}
	ids := make([]string, len(rawIDs))
	for i, rawID := range rawIDs {
		id, ok := rawID.(string)
		if !ok {
			return nil, fmt.Errorf("actionids[%d] has type %T, want string", i, rawID)
		}
		ids[i] = id
	}
	return ids, nil
}
