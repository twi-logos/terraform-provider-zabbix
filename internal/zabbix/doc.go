/*
Package zabbix is the Zabbix JSON-RPC API client used by terraform-provider-zabbix.

It began life as the standalone go-zabbix-api library, vendored here as a git
submodule and merged into the provider in v2. It is deliberately an internal
package: the provider is its only consumer, and it is free to change shape whenever
the provider needs it to.

# Supported versions

Zabbix 7.4 and above. NewAPI rejects older servers after probing APIInfo.version.
Config.Version uses major*10000 + minor*100 + patch (7.4.13 is 70413). Use named
version constants rather than bare integers.

# Authentication

NewAPI probes APIInfo.version unauthenticated. After that, Login stores a session token
in API.Auth, or the caller may set API.Auth directly to a pre-created API token. The
token travels in an Authorization: Bearer header.

# Testing

This package has focused unit tests and is covered end to end by the provider's
acceptance suite against live Zabbix 7.4. Zabbix 8.0 trunk runs as non-blocking
early warning. See TESTING.md.
*/
package zabbix
