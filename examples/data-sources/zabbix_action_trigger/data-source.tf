data "zabbix_action_trigger" "example" {
  name = "Notify infrastructure failures"
}

output "action_id" {
  value = data.zabbix_action_trigger.example.id
}