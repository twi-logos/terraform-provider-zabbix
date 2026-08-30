# requires Zabbix 7.4 or later
resource "zabbix_templategroup" "applications" {
  name = "Templates/Applications"
}

resource "zabbix_template" "example" {
  host   = "example-template"
  groups = [zabbix_templategroup.applications.id]
}
