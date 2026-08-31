package provider

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/twi-logos/terraform-provider-zabbix/internal/zabbix"
)

type testAccActionPrerequisites struct {
	userID       string
	userGroupID  string
	mediaTypeIDs [2]string
	scriptIDs    [2]string
}

func TestActionTriggerAcceptanceFixturesParse(t *testing.T) {
	prerequisites := testAccActionPrerequisites{
		userID:       "1",
		userGroupID:  "2",
		mediaTypeIDs: [2]string{"3", "4"},
		scriptIDs:    [2]string{"5", "6"},
	}
	fixtures := map[string]string{
		"update before":   testAccActionConfigA(prerequisites),
		"update after":    testAccActionConfigB(prerequisites),
		"defaults before": testAccActionDefaultsConfig(prerequisites, true),
		"defaults after":  testAccActionDefaultsConfig(prerequisites, false),
		"match all":       testAccActionMatchAllConfig(prerequisites),
	}
	for name, config := range fixtures {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(config, "%!") {
				t.Fatalf("fixture contains an unresolved format directive:\n%s", config)
			}
			_, diagnostics := hclparse.NewParser().ParseHCL([]byte(config), name+".tf")
			if diagnostics.HasErrors() {
				t.Fatalf("fixture is not valid HCL: %s", diagnostics.Error())
			}
		})
	}
}

func TestActionTriggerExampleParses(t *testing.T) {
	const path = "../examples/resources/zabbix_action_trigger/resource.tf"
	config, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trigger-action example: %v", err)
	}
	_, diagnostics := hclparse.NewParser().ParseHCL(config, path)
	if diagnostics.HasErrors() {
		t.Fatalf("trigger-action example is not valid HCL: %s", diagnostics.Error())
	}
}

func testAccPrepareActionPrerequisites(t *testing.T) testAccActionPrerequisites {
	t.Helper()
	api := testAccAdminAPI(t)

	var users []struct {
		UserID string `json:"userid"`
		Groups []struct {
			UserGroupID string `json:"usrgrpid"`
		} `json:"usrgrps"`
	}
	if err := api.CallWithErrorParse("user.get", zabbix.Params{
		"output":        []string{"userid"},
		"selectUsrgrps": []string{"usrgrpid"},
		"filter":        zabbix.Params{"username": os.Getenv("ZABBIX_USER")},
	}, &users); err != nil {
		t.Fatalf("discovering the acceptance user: %s", err)
	}
	if len(users) != 1 || len(users[0].Groups) == 0 {
		t.Fatalf("acceptance user discovery returned %d users and %d groups", len(users), func() int {
			if len(users) == 0 {
				return 0
			}
			return len(users[0].Groups)
		}())
	}

	var mediaTypes []struct {
		MediaTypeID string `json:"mediatypeid"`
	}
	if err := api.CallWithErrorParse("mediatype.get", zabbix.Params{
		"output":    []string{"mediatypeid"},
		"sortfield": "mediatypeid",
	}, &mediaTypes); err != nil {
		t.Fatalf("discovering media types: %s", err)
	}
	if len(mediaTypes) < 2 {
		t.Fatalf("need two media types for update coverage, found %d", len(mediaTypes))
	}

	leftoverActions, err := api.ActionsGet(zabbix.Params{
		"output":      []string{"actionid"},
		"search":      zabbix.Params{"name": "test-action-trigger-"},
		"startSearch": true,
		"filter":      zabbix.Params{"eventsource": []int{0}},
	})
	if err != nil {
		t.Fatalf("finding leftover trigger actions: %s", err)
	}
	if len(leftoverActions) > 0 {
		ids := make([]string, len(leftoverActions))
		for i, action := range leftoverActions {
			ids[i] = action.ActionID
		}
		if err := api.ActionsDeleteByIDs(ids); err != nil {
			t.Fatalf("deleting leftover trigger actions: %s", err)
		}
	}

	var existing []struct {
		ScriptID string `json:"scriptid"`
	}
	if err := api.CallWithErrorParse("script.get", zabbix.Params{
		"output":      []string{"scriptid"},
		"search":      zabbix.Params{"name": "test-action-trigger-script-"},
		"startSearch": true,
	}, &existing); err != nil {
		t.Fatalf("finding leftover action scripts: %s", err)
	}
	if len(existing) > 0 {
		ids := make([]string, len(existing))
		for i, script := range existing {
			ids[i] = script.ScriptID
		}
		if _, err := api.CallWithError("script.delete", ids); err != nil {
			t.Fatalf("deleting leftover action scripts: %s", err)
		}
	}

	var created struct {
		ScriptIDs []string `json:"scriptids"`
	}
	if err := api.CallWithErrorParse("script.create", []zabbix.Params{
		{"name": "test-action-trigger-script-a", "type": 0, "scope": 1, "command": "echo action-a", "execute_on": 1},
		{"name": "test-action-trigger-script-b", "type": 0, "scope": 1, "command": "echo action-b", "execute_on": 1},
	}, &created); err != nil {
		t.Fatalf("creating action-scope global scripts: %s", err)
	}
	if len(created.ScriptIDs) != 2 {
		t.Fatalf("script.create returned %d ids, want 2", len(created.ScriptIDs))
	}
	t.Cleanup(func() {
		if _, err := api.CallWithError("script.delete", created.ScriptIDs); err != nil {
			t.Logf("could not remove action scripts: %s", err)
		}
	})

	return testAccActionPrerequisites{
		userID:      users[0].UserID,
		userGroupID: users[0].Groups[0].UserGroupID,
		mediaTypeIDs: [2]string{
			mediaTypes[0].MediaTypeID,
			mediaTypes[1].MediaTypeID,
		},
		scriptIDs: [2]string{created.ScriptIDs[0], created.ScriptIDs[1]},
	}
}

var getServerActionTrigger = serverObject("action.get", "actionids", zabbix.Params{
	"selectFilter":             "extend",
	"selectOperations":         "extend",
	"selectRecoveryOperations": "extend",
	"selectUpdateOperations":   "extend",
	"filter":                   zabbix.Params{"eventsource": []int{0}},
})

func serverActionTrigger(api *zabbix.API, id string) (map[string]interface{}, error) {
	action, err := getServerActionTrigger(api, id)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"operations", "recovery_operations", "update_operations"} {
		operations, _ := action[key].([]interface{})
		sort.SliceStable(operations, func(i, j int) bool {
			left, _ := strconv.Atoi(serverString(operations[i].(map[string]interface{})["operationtype"]))
			right, _ := strconv.Atoi(serverString(operations[j].(map[string]interface{})["operationtype"]))
			return left < right
		})
	}
	return action, nil
}

func testAccCheckActionReferences(actionAddr string, paths map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		api, ok := testAccProvider.Meta().(*zabbix.API)
		if !ok || api == nil {
			return fmt.Errorf("testAccCheckActionReferences: provider not configured")
		}
		actionID, err := testAccStateID(s, actionAddr)
		if err != nil {
			return err
		}
		action, err := serverActionTrigger(api, actionID)
		if err != nil {
			return err
		}
		for path, targetAddr := range paths {
			targetID, err := testAccStateID(s, targetAddr)
			if err != nil {
				return err
			}
			got, found := serverValue(action, path)
			if !found || serverString(got) != targetID {
				return fmt.Errorf("%s server path %s = %q, want %s id %q", actionAddr, path, serverString(got), targetAddr, targetID)
			}
		}
		return nil
	}
}

func testAccCheckActionConditionCount(actionAddr string, want int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		api, ok := testAccProvider.Meta().(*zabbix.API)
		if !ok || api == nil {
			return fmt.Errorf("testAccCheckActionConditionCount: provider not configured")
		}
		actionID, err := testAccStateID(s, actionAddr)
		if err != nil {
			return err
		}
		action, err := serverActionTrigger(api, actionID)
		if err != nil {
			return err
		}
		rawConditions, found := serverValue(action, "filter.conditions")
		if !found {
			return fmt.Errorf("%s server filter.conditions is missing", actionAddr)
		}
		conditions, ok := rawConditions.([]interface{})
		if !ok {
			return fmt.Errorf("%s server filter.conditions has type %T, want array", actionAddr, rawConditions)
		}
		if len(conditions) != want {
			return fmt.Errorf("%s server filter.conditions has %d entries, want %d", actionAddr, len(conditions), want)
		}
		return nil
	}
}

func testAccActionTargetFixture(body string) string {
	return `
resource "zabbix_hostgroup" "testactiongroupa" {
	name = "test-action-trigger-group-a"
}
resource "zabbix_hostgroup" "testactiongroupb" {
	name = "test-action-trigger-group-b"
}
resource "zabbix_host" "testactionhosta" {
	host   = "test-action-trigger-host-a"
	groups = [zabbix_hostgroup.testactiongroupa.id]
}
resource "zabbix_host" "testactionhostb" {
	host   = "test-action-trigger-host-b"
	groups = [zabbix_hostgroup.testactiongroupb.id]
}
resource "zabbix_action_trigger" "testaction" {
` + body + `
}
`
}

func testAccActionConfigForVersion(t *testing.T, config string) string {
	if testAccVersion(t) >= zabbix.V64 {
		return config
	}
	return strings.ReplaceAll(config, "pause_symptoms     = false", "pause_symptoms     = true")
}

func testAccActionConfigA(p testAccActionPrerequisites) string {
	return fmt.Sprintf(testAccActionTargetFixture(`
	name               = "test-action-trigger-a"
	status             = "enabled"
	escalation_period  = "5m"
	pause_suppressed   = true
	pause_symptoms     = true
	notify_if_canceled = true

	filter {
		evaluation_type = "custom_expression"
		formula         = "A and B"
		condition {
			condition_type = "event_tag_value"
			operator       = "equals"
			value          = "value-a"
			value2         = "tag-a"
			label          = "A"
		}
		condition {
			condition_type = "host_group"
			operator       = "equals"
			value          = zabbix_hostgroup.testactiongroupa.id
			label          = "B"
		}
	}

	operations {
		escalation_period         = "1m"
		escalation_step_from      = 1
		escalation_step_to        = 2
		condition_evaluation_type = "and"
		condition { acknowledged = false }
		send_message {
			use_default_message = false
			subject             = "problem-a"
			message             = "problem message a"
			media_type_id       = %q
			user_group_ids      = [%q]
		}
	}
	operations {
		escalation_period         = "2m"
		escalation_step_from      = 3
		escalation_step_to        = 0
		condition_evaluation_type = "or"
		condition { acknowledged = true }
		remote_command {
			current_host   = true
			host_ids       = [zabbix_host.testactionhosta.id]
			host_group_ids = [zabbix_hostgroup.testactiongroupa.id]
			global_script { script_id = %q }
		}
	}

	recovery_operations {
		notify_all_involved = true
		notify_all_message {
			use_default_message = false
			subject             = "recovery-a"
			message             = "recovery message a"
		}
	}
	recovery_operations {
		send_message {
			use_default_message = false
			subject             = "recovery direct a"
			message             = "recovery direct message a"
			media_type_id       = %q
			user_group_ids      = [%q]
		}
	}
	recovery_operations {
		remote_command {
			current_host   = true
			host_ids       = [zabbix_host.testactionhosta.id]
			host_group_ids = [zabbix_hostgroup.testactiongroupa.id]
			global_script { script_id = %q }
		}
	}

	update_operations {
		notify_all_involved = true
		notify_all_message {
			use_default_message = false
			subject             = "update-a"
			message             = "update message a"
			media_type_id       = %q
		}
	}
	update_operations {
		send_message {
			use_default_message = false
			subject             = "update direct a"
			message             = "update direct message a"
			media_type_id       = %q
			user_group_ids      = [%q]
		}
	}
	update_operations {
		remote_command {
			current_host   = true
			host_ids       = [zabbix_host.testactionhosta.id]
			host_group_ids = [zabbix_hostgroup.testactiongroupa.id]
			global_script { script_id = %q }
		}
	}
`), p.mediaTypeIDs[0], p.userGroupID, p.scriptIDs[0], p.mediaTypeIDs[0], p.userGroupID, p.scriptIDs[0], p.mediaTypeIDs[0], p.mediaTypeIDs[0], p.userGroupID, p.scriptIDs[0])
}

func testAccActionConfigB(p testAccActionPrerequisites) string {
	return fmt.Sprintf(testAccActionTargetFixture(`
	name               = "test-action-trigger-b"
	status             = "disabled"
	escalation_period  = "15m"
	pause_suppressed   = false
	pause_symptoms     = false
	notify_if_canceled = false

	filter {
		evaluation_type = "or"
		condition {
			condition_type = "trigger_name"
			operator       = "like"
			value          = "trigger-b"
		}
		condition {
			condition_type = "maintenance_status"
			operator       = "no"
		}
	}

	operations {
		escalation_period         = "3m"
		escalation_step_from      = 2
		escalation_step_to        = 0
		condition_evaluation_type = "and_or"
		condition { acknowledged = true }
		remote_command {
			host_ids       = [zabbix_host.testactionhostb.id]
			host_group_ids = [zabbix_hostgroup.testactiongroupb.id]
			global_script { script_id = %q }
		}
	}
	operations {
		escalation_period         = "4m"
		escalation_step_from      = 4
		escalation_step_to        = 5
		condition_evaluation_type = "and"
		condition { acknowledged = false }
		send_message {
			media_type_id = %q
			user_ids      = [%q]
		}
	}

	recovery_operations {
		send_message {
			media_type_id = %q
			user_ids      = [%q]
		}
	}
	recovery_operations {
		remote_command {
			host_ids       = [zabbix_host.testactionhostb.id]
			host_group_ids = [zabbix_hostgroup.testactiongroupb.id]
			global_script { script_id = %q }
		}
	}
	recovery_operations {
		notify_all_involved = true
	}

	update_operations {
		send_message {
			media_type_id = %q
			user_ids      = [%q]
		}
	}
	update_operations {
		remote_command {
			host_ids       = [zabbix_host.testactionhostb.id]
			host_group_ids = [zabbix_hostgroup.testactiongroupb.id]
			global_script { script_id = %q }
		}
	}
	update_operations {
		notify_all_involved = true
	}
`), p.scriptIDs[1], p.mediaTypeIDs[1], p.userID, p.mediaTypeIDs[1], p.userID, p.scriptIDs[1], p.mediaTypeIDs[1], p.userID, p.scriptIDs[1])
}

func TestAccUpdateActionTrigger(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test: set TF_ACC=1 to run")
	}
	testAccPreCheck(t)
	p := testAccPrepareActionPrerequisites(t)
	const addr = "zabbix_action_trigger.testaction"

	resource.Test(t, resource.TestCase{
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{Config: testAccActionConfigA(p)},
			{
				Config:           testAccActionConfigForVersion(t, testAccActionConfigB(p)),
				ConfigPlanChecks: expectUpdate(addr),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckServerAttrs(addr, serverActionTrigger, map[string]string{
						"name":                                        "test-action-trigger-b",
						"status":                                      "1",
						"esc_period":                                  "15m",
						"pause_suppressed":                            "0",
						"pause_symptoms":                              "0",
						"notify_if_canceled":                          "0",
						"filter.evaltype":                             "2",
						"filter.formula":                              "",
						"filter.conditions.0.conditiontype":           "3",
						"filter.conditions.0.operator":                "2",
						"filter.conditions.0.value":                   "trigger-b",
						"filter.conditions.1.conditiontype":           "16",
						"filter.conditions.1.operator":                "11",
						"operations.0.operationtype":                  "0",
						"operations.0.esc_period":                     "4m",
						"operations.0.esc_step_from":                  "4",
						"operations.0.esc_step_to":                    "5",
						"operations.0.evaltype":                       "1",
						"operations.0.opconditions.0.value":           "0",
						"operations.0.opmessage.default_msg":          "1",
						"operations.0.opmessage.mediatypeid":          p.mediaTypeIDs[1],
						"operations.0.opmessage_usr.0.userid":         p.userID,
						"operations.1.operationtype":                  "1",
						"operations.1.opcommand.scriptid":             p.scriptIDs[1],
						"recovery_operations.0.operationtype":         "0",
						"recovery_operations.1.operationtype":         "1",
						"recovery_operations.1.opcommand.scriptid":    p.scriptIDs[1],
						"recovery_operations.2.operationtype":         "11",
						"recovery_operations.2.opmessage.default_msg": "1",
						"update_operations.0.operationtype":           "0",
						"update_operations.0.opmessage.mediatypeid":   p.mediaTypeIDs[1],
						"update_operations.1.operationtype":           "1",
						"update_operations.1.opcommand.scriptid":      p.scriptIDs[1],
						"update_operations.2.operationtype":           "12",
						"update_operations.2.opmessage.default_msg":   "1",
						"update_operations.2.opmessage.mediatypeid":   "0",
					}),
					testAccCheckActionReferences(addr, map[string]string{
						"operations.1.opcommand_hst.0.hostid":           "zabbix_host.testactionhostb",
						"operations.1.opcommand_grp.0.groupid":          "zabbix_hostgroup.testactiongroupb",
						"recovery_operations.1.opcommand_hst.0.hostid":  "zabbix_host.testactionhostb",
						"recovery_operations.1.opcommand_grp.0.groupid": "zabbix_hostgroup.testactiongroupb",
						"update_operations.1.opcommand_hst.0.hostid":    "zabbix_host.testactionhostb",
						"update_operations.1.opcommand_grp.0.groupid":   "zabbix_hostgroup.testactiongroupb",
					}),
				),
			},
			{RefreshState: true},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccActionMatchAllConfig(p testAccActionPrerequisites) string {
	return fmt.Sprintf(`
resource "zabbix_action_trigger" "testaction" {
	name = "test-action-trigger-match-all"
	filter {
		evaluation_type = "and_or"
	}
	operations {
		send_message {
			user_ids = [%q]
		}
	}
}
`, p.userID)
}

func TestAccActionTriggerMatchAll(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test: set TF_ACC=1 to run")
	}
	testAccPreCheck(t)
	p := testAccPrepareActionPrerequisites(t)
	const addr = "zabbix_action_trigger.testaction"
	config := testAccActionMatchAllConfig(p)

	resource.Test(t, resource.TestCase{
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "filter.0.condition.#", "0"),
					testAccCheckActionConditionCount(addr, 0),
				),
			},
			{Config: config, PlanOnly: true},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccActionDefaultsConfig(p testAccActionPrerequisites, explicit bool) string {
	if explicit {
		return fmt.Sprintf(testAccActionTargetFixture(`
	name = "test-action-trigger-defaults"
	status             = "disabled"
	escalation_period  = "10m"
	pause_suppressed   = false
	pause_symptoms     = false
	notify_if_canceled = false
	filter {
		evaluation_type = "and_or"
		condition {
			condition_type = "trigger_name"
			operator       = "like"
			value          = "defaults"
		}
	}
	operations {
		escalation_period         = "2m"
		escalation_step_from      = 2
		escalation_step_to        = 3
		condition_evaluation_type = "or"
		send_message {
			use_default_message = false
			subject             = "default removal"
			message             = "default removal body"
			media_type_id       = %q
			user_group_ids      = [%q]
		}
	}
	operations {
		escalation_period         = "2m"
		escalation_step_from      = 2
		escalation_step_to        = 3
		condition_evaluation_type = "or"
		remote_command {
			current_host = true
			host_ids = [zabbix_host.testactionhosta.id]
			global_script { script_id = %q }
		}
	}
	recovery_operations {
		notify_all_involved = true
		notify_all_message {
			use_default_message = false
			subject             = "recovery removal"
			message             = "recovery removal body"
		}
	}
	recovery_operations {
		send_message {
			use_default_message = false
			subject             = "recovery direct removal"
			message             = "recovery direct removal body"
			media_type_id       = %q
			user_group_ids = [%q]
		}
	}
	recovery_operations {
		remote_command {
			current_host = true
			host_ids = [zabbix_host.testactionhosta.id]
			global_script { script_id = %q }
		}
	}
	recovery_operations {
		notify_all_involved = true
	}
	update_operations {
		notify_all_involved = true
		notify_all_message {
			use_default_message = false
			subject             = "update removal"
			message             = "update removal body"
			media_type_id       = %q
		}
	}
	update_operations {
		send_message {
			use_default_message = false
			subject             = "update direct removal"
			message             = "update direct removal body"
			media_type_id       = %q
			user_group_ids = [%q]
		}
	}
	update_operations {
		remote_command {
			current_host = true
			host_ids = [zabbix_host.testactionhosta.id]
			global_script { script_id = %q }
		}
	}
	update_operations {
		notify_all_involved = true
	}
`), p.mediaTypeIDs[0], p.userGroupID, p.scriptIDs[0], p.mediaTypeIDs[0], p.userGroupID, p.scriptIDs[0], p.mediaTypeIDs[0], p.mediaTypeIDs[0], p.userGroupID, p.scriptIDs[0])
	}

	return fmt.Sprintf(testAccActionTargetFixture(`
	name = "test-action-trigger-defaults"
	filter {
		evaluation_type = "and_or"
		condition {
			condition_type = "trigger_name"
			operator       = "like"
			value          = "defaults"
		}
	}
	operations {
		send_message {
			user_group_ids = [%q]
		}
	}
	operations {
		remote_command {
			host_ids = [zabbix_host.testactionhosta.id]
			global_script { script_id = %q }
		}
	}
	recovery_operations {
		send_message {
			user_group_ids = [%q]
		}
	}
	recovery_operations {
		send_message {
			user_group_ids = [%q]
		}
	}
	recovery_operations {
		remote_command {
			host_ids = [zabbix_host.testactionhosta.id]
			global_script { script_id = %q }
		}
	}
	recovery_operations {
		send_message {
			user_group_ids = [%q]
		}
	}
	update_operations {
		send_message {
			user_group_ids = [%q]
		}
	}
	update_operations {
		send_message {
			user_group_ids = [%q]
		}
	}
	update_operations {
		remote_command {
			host_ids = [zabbix_host.testactionhosta.id]
			global_script { script_id = %q }
		}
	}
	update_operations {
		send_message {
			user_group_ids = [%q]
		}
	}
`),
		p.userGroupID, p.scriptIDs[0],
		p.userGroupID, p.userGroupID, p.scriptIDs[0], p.userGroupID,
		p.userGroupID, p.userGroupID, p.scriptIDs[0], p.userGroupID,
	)
}

func TestAccRemoveActionTriggerDefaults(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test: set TF_ACC=1 to run")
	}
	testAccPreCheck(t)
	p := testAccPrepareActionPrerequisites(t)
	const addr = "zabbix_action_trigger.testaction"

	resource.Test(t, resource.TestCase{
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{Config: testAccActionConfigForVersion(t, testAccActionDefaultsConfig(p, true))},
			{
				Config:           testAccActionDefaultsConfig(p, false),
				ConfigPlanChecks: expectUpdate(addr),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "recovery_operations.#", "4"),
					resource.TestCheckResourceAttr(addr, "update_operations.#", "4"),
					testAccCheckServerAttrs(addr, serverActionTrigger, map[string]string{
						"status":                                      "0",
						"esc_period":                                  "1h",
						"pause_suppressed":                            "1",
						"pause_symptoms":                              "1",
						"notify_if_canceled":                          "1",
						"operations.0.esc_period":                     "0",
						"operations.0.esc_step_from":                  "1",
						"operations.0.esc_step_to":                    "1",
						"operations.0.evaltype":                       "0",
						"operations.0.opmessage.default_msg":          "1",
						"operations.0.opmessage.mediatypeid":          "0",
						"recovery_operations.0.opmessage.default_msg": "1",
						"recovery_operations.1.opmessage.default_msg": "1",
						"recovery_operations.1.opmessage.mediatypeid": "0",
						"recovery_operations.3.operationtype":         "1",
						"update_operations.0.opmessage.default_msg":   "1",
						"update_operations.0.opmessage.mediatypeid":   "0",
						"update_operations.1.opmessage.default_msg":   "1",
						"update_operations.1.opmessage.mediatypeid":   "0",
						"update_operations.3.operationtype":           "1",
					}),
					testAccCheckActionReferences(addr, map[string]string{
						"operations.1.opcommand_hst.0.hostid":          "zabbix_host.testactionhosta",
						"recovery_operations.3.opcommand_hst.0.hostid": "zabbix_host.testactionhosta",
						"update_operations.3.opcommand_hst.0.hostid":   "zabbix_host.testactionhosta",
					}),
				),
			},
			{RefreshState: true},
		},
	})
}
