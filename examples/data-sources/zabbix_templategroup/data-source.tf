# requires Zabbix 7.4 or later
data "zabbix_templategroup" "applications" {
  name = "Templates/Applications"
}

resource "zabbix_template" "example" {
  host   = "example-template"
  groups = [data.zabbix_templategroup.applications.id]
}
