variable "operations_user_group_id" {
  type        = string
  description = "Zabbix user group ID that receives problem notifications."
}

variable "diagnostic_script_id" {
  type        = string
  description = "Zabbix global script ID to run on the host that generated the event."
}

resource "zabbix_action_trigger" "database_problem" {
  name              = "Database problem response"
  escalation_period = "10m"
  pause_symptoms    = true

  filter {
    evaluation_type = "custom_expression"
    formula         = "A and B"

    condition {
      condition_type = "event_tag_value"
      operator       = "equals"
      value          = "database"
      value2         = "service"
      label          = "A"
    }

    condition {
      condition_type = "trigger_severity"
      operator       = "greater_or_equals"
      value          = "3"
      label          = "B"
    }
  }

  operations {
    escalation_step_from = 1
    escalation_step_to   = 0

    send_message {
      user_group_ids = [var.operations_user_group_id]
    }
  }

  operations {
    escalation_step_from = 2
    escalation_step_to   = 2

    condition {
      acknowledged = false
    }

    remote_command {
      current_host = true

      global_script {
        script_id = var.diagnostic_script_id
      }
    }
  }

  recovery_operations {
    notify_all_involved = true

    notify_all_message {
      use_default_message = false
      subject             = "Database problem recovered"
      message             = "The database problem on {HOST.NAME} has recovered."
    }
  }

  update_operations {
    notify_all_involved = true
  }
}