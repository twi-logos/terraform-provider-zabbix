package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/twi-logos/terraform-provider-zabbix/internal/zabbix"
)

func TestDataActionTriggerReadByName(t *testing.T) {
	reads := 0
	api := actionDataSourceTestAPI(t, []interface{}{actionDataSourceTestResult("117", "Notify failures")}, func(params map[string]interface{}) {
		reads++
		filter := params["filter"].(map[string]interface{})
		names := filter["name"].([]interface{})
		if len(names) != 1 || names[0] != "Notify failures" {
			t.Errorf("name filter = %#v", filter["name"])
		}
		if _, present := params["selectUpdateOperations"]; !present {
			t.Error("action.get did not request update operations")
		}
	})
	data := schema.TestResourceDataRaw(t, dataActionTrigger().Schema, map[string]interface{}{"name": "Notify failures"})

	if err := dataActionTriggerRead(data, api); err != nil {
		t.Fatalf("read data source: %v", err)
	}
	if data.Id() != "117" {
		t.Fatalf("ID = %q", data.Id())
	}
	if data.Get("status") != "enabled" || data.Get("escalation_period") != "1m" {
		t.Fatalf("computed state = status:%#v escalation_period:%#v", data.Get("status"), data.Get("escalation_period"))
	}
	if err := dataActionTriggerRead(data, api); err != nil {
		t.Fatalf("refresh data source: %v", err)
	}
	if reads != 2 {
		t.Fatalf("name lookup requests = %d, want 2", reads)
	}
}

func TestDataActionTriggerReadByID(t *testing.T) {
	api := actionDataSourceTestAPI(t, []interface{}{actionDataSourceTestResult("117", "Notify failures")}, func(params map[string]interface{}) {
		ids := params["actionids"].([]interface{})
		if len(ids) != 1 || ids[0] != "117" {
			t.Errorf("actionids = %#v", params["actionids"])
		}
	})
	data := schema.TestResourceDataRaw(t, dataActionTrigger().Schema, map[string]interface{}{"action_id": "117"})

	if err := dataActionTriggerRead(data, api); err != nil {
		t.Fatalf("read data source: %v", err)
	}
	if data.Get("name") != "Notify failures" {
		t.Fatalf("name = %#v", data.Get("name"))
	}
}

func TestDataActionTriggerReadPreservesOperations(t *testing.T) {
	result := actionDataSourceTestResult("117", "Notify failures")
	result["operations"] = []interface{}{
		map[string]interface{}{
			"operationtype": "0", "esc_period": "5m", "esc_step_from": "2", "esc_step_to": "3", "evaltype": "1",
			"opconditions":  []interface{}{map[string]interface{}{"conditiontype": "14", "operator": "0", "value": "1"}},
			"opmessage":     map[string]interface{}{"default_msg": "0", "subject": "Problem", "message": "Problem body", "mediatypeid": "7"},
			"opmessage_grp": []interface{}{map[string]interface{}{"usrgrpid": "8"}},
			"opmessage_usr": []interface{}{map[string]interface{}{"userid": "9"}},
		},
		map[string]interface{}{
			"operationtype": "1", "esc_period": "0", "esc_step_from": "1", "esc_step_to": "0", "evaltype": "0",
			"opconditions": []interface{}{}, "opcommand": map[string]interface{}{"scriptid": "57"},
			"opcommand_hst": []interface{}{map[string]interface{}{"hostid": "0"}, map[string]interface{}{"hostid": "10"}},
			"opcommand_grp": []interface{}{map[string]interface{}{"groupid": "11"}},
		},
	}
	result["recovery_operations"] = []interface{}{map[string]interface{}{
		"operationtype": "11", "opmessage": map[string]interface{}{"default_msg": "0", "subject": "Recovered", "message": "Recovery body", "mediatypeid": "0"},
	}}
	result["update_operations"] = []interface{}{map[string]interface{}{
		"operationtype": "12", "opmessage": map[string]interface{}{"default_msg": "1", "subject": "", "message": "", "mediatypeid": "7"},
	}}

	api := actionDataSourceTestAPI(t, []interface{}{result}, nil)
	data := schema.TestResourceDataRaw(t, dataActionTrigger().Schema, map[string]interface{}{"action_id": "117"})
	if err := dataActionTriggerRead(data, api); err != nil {
		t.Fatalf("read data source: %v", err)
	}

	checks := map[string]interface{}{
		"operations.0.send_message.0.subject":                          "Problem",
		"operations.1.remote_command.0.current_host":                   true,
		"operations.1.remote_command.0.global_script.0.script_id":      "57",
		"recovery_operations.0.notify_all_involved":                    true,
		"recovery_operations.0.notify_all_message.0.subject":           "Recovered",
		"update_operations.0.notify_all_involved":                      true,
		"update_operations.0.notify_all_message.0.use_default_message": true,
		"update_operations.0.notify_all_message.0.media_type_id":       "7",
	}
	for key, want := range checks {
		if got := data.Get(key); got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
	setChecks := map[string]interface{}{
		"operations.0.condition":                       map[string]interface{}{"acknowledged": true},
		"operations.0.send_message.0.user_group_ids":   "8",
		"operations.0.send_message.0.user_ids":         "9",
		"operations.1.remote_command.0.host_ids":       "10",
		"operations.1.remote_command.0.host_group_ids": "11",
	}
	for key, want := range setChecks {
		values, ok := data.Get(key).(*schema.Set)
		if !ok {
			t.Errorf("%s = %#v, want a set", key, data.Get(key))
			continue
		}
		if !values.Contains(want) {
			t.Errorf("%s = %#v, want member %#v", key, values.List(), want)
		}
	}
}

func TestDataActionTriggerReadRejectsMissingAndAmbiguousNames(t *testing.T) {
	for _, test := range []struct {
		name    string
		results []interface{}
		wantErr string
	}{
		{name: "missing", wantErr: "no trigger action found"},
		{
			name: "ambiguous",
			results: []interface{}{
				actionDataSourceTestResult("117", "Duplicate"),
				actionDataSourceTestResult("118", "Duplicate"),
			},
			wantErr: "use action_id to disambiguate",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := actionDataSourceTestAPI(t, test.results, nil)
			data := schema.TestResourceDataRaw(t, dataActionTrigger().Schema, map[string]interface{}{"name": "Duplicate"})
			err := dataActionTriggerRead(data, api)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestAccDataSourceActionTrigger(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test: set TF_ACC=1 to run")
	}
	testAccPreCheck(t)
	prerequisites := testAccPrepareActionPrerequisites(t)
	config := testAccActionConfigForVersion(t, testAccActionConfigA(prerequisites)) + `
data "zabbix_action_trigger" "by_name" {
	name = zabbix_action_trigger.testaction.name
}

data "zabbix_action_trigger" "by_id" {
	action_id = zabbix_action_trigger.testaction.id
}
`

	resource.Test(t, resource.TestCase{
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckAllDestroyed,
		Steps: []resource.TestStep{{
			Config: config,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrPair("data.zabbix_action_trigger.by_name", "id", "zabbix_action_trigger.testaction", "id"),
				resource.TestCheckResourceAttr("data.zabbix_action_trigger.by_name", "operations.#", "2"),
				resource.TestCheckResourceAttr("data.zabbix_action_trigger.by_name", "filter.0.condition.#", "2"),
				resource.TestCheckResourceAttrPair("data.zabbix_action_trigger.by_id", "id", "zabbix_action_trigger.testaction", "id"),
				resource.TestCheckResourceAttr("data.zabbix_action_trigger.by_id", "name", "test-action-trigger-a"),
			),
		}},
	})
}

func actionDataSourceTestResult(id, name string) map[string]interface{} {
	return map[string]interface{}{
		"actionid": id, "name": name, "status": "0", "esc_period": "1m",
		"pause_suppressed": "1", "pause_symptoms": "1", "notify_if_canceled": "1",
		"filter":     map[string]interface{}{"evaltype": "0", "formula": "", "conditions": []interface{}{}},
		"operations": []interface{}{}, "recovery_operations": []interface{}{}, "update_operations": []interface{}{},
	}
}

func actionDataSourceTestAPI(t *testing.T, results []interface{}, checkParams func(map[string]interface{})) *zabbix.API {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string                 `json:"method"`
			Params map[string]interface{} `json:"params"`
			ID     int32                  `json:"id"`
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
			if checkParams != nil {
				checkParams(request.Params)
			}
			result = results
		default:
			t.Errorf("unexpected method %q", request.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0", "id": request.ID, "result": result,
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	api, err := zabbix.NewAPI(zabbix.Config{Url: server.URL})
	if err != nil {
		t.Fatalf("create API client: %v", err)
	}
	return api
}
