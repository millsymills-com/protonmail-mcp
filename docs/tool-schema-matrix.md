<!-- GENERATED FILE — do not edit by hand.
     Regenerate: go test ./internal/tools -run TestSchemaMatrix -update-matrix
     Source of truth: the tools registered in internal/tools (MCP tools/list). -->

# Tool ↔ schema coverage matrix

Every tool the server advertises, with its input/output schema fields and the
env gate that registers it. Required fields are marked `*`. `TestSchemaMatrixNoDrift`
asserts this file matches the live schema; it fails if a tool or field drifts.

Tools: 32 total — 16 read, 13 write, 3 dangerous.

| Tool | Mode | Gate | Input fields | Output fields |
| --- | --- | --- | --- | --- |
| `proton_add_custom_domain` | write | `WRITES` | `domain_name*`:string | `domain*`:object |
| `proton_create_address` | write | `WRITES` | `display_name`:string, `domain_id*`:string, `local_part*`:string, `signature`:string | `email*`:string, `id*`:string |
| `proton_create_draft` | write | `WRITES` | `bcc`:array<string>, `body`:string, `cc`:array<string>, `from_address_id`:string, `mime_type`:string, `subject`:string, `to`:array<string> | `message*`:object |
| `proton_delete_address` | dangerous | `WRITES`+`DANGEROUS` | `id*`:string | `ok*`:boolean |
| `proton_delete_messages` | dangerous | `WRITES`+`DANGEROUS` | `message_ids*`:array<string> | `message_ids*`:array<string>, `ok*`:boolean |
| `proton_disable_catchall` | write | `WRITES` | `domain_id*`:string | `ok*`:boolean |
| `proton_get_address` | read | — | `id*`:string | `address*`:object |
| `proton_get_catchall` | read | — | `domain_id*`:string | `destination_address_id`:string, `destination_email`:string, `domain_id*`:string, `enabled*`:boolean |
| `proton_get_core_settings` | read | — | — | `settings*`:object |
| `proton_get_custom_domain` | read | — | `id*`:string | `domain*`:object |
| `proton_get_event` | read | — | `calendar_id*`:string, `event_id*`:string | `event*`:object |
| `proton_get_mail_settings` | read | — | — | `settings*`:object |
| `proton_get_message` | read | — | `id*`:string, `include_body`:boolean, `include_headers`:boolean | `body`:string, `message*`:object, `parsed_headers`:object, `raw_headers`:string |
| `proton_label_messages` | write | `WRITES` | `action*`:string, `label_id*`:string, `message_ids*`:array<string> | `message_ids*`:array<string>, `ok*`:boolean |
| `proton_list_address_keys` | read | — | `address_id*`:string | `keys*`:array<object> |
| `proton_list_addresses` | read | — | — | `addresses*`:array<object> |
| `proton_list_calendars` | read | — | — | `calendars*`:array<object> |
| `proton_list_custom_domains` | read | — | — | `domains*`:array<object> |
| `proton_list_events` | read | — | `calendar_id`:string, `end*`:string, `start*`:string | `events*`:array<object>, `skipped`:integer, `truncated`:boolean |
| `proton_list_labels` | read | — | — | `labels*`:array<object> |
| `proton_mark_messages` | write | `WRITES` | `message_ids*`:array<string>, `read*`:boolean | `message_ids*`:array<string>, `ok*`:boolean |
| `proton_remove_custom_domain` | dangerous | `WRITES`+`DANGEROUS` | `id*`:string | `ok*`:boolean |
| `proton_search_messages` | read | — | `address_id`:string, `label_id`:string, `limit`:integer, `page`:integer, `query`:string | `messages*`:array<object> |
| `proton_session_status` | read | — | — | `email`:string, `keyring_unlock`:string, `logged_in*`:boolean, `persist_degraded`:boolean, `persist_error`:string |
| `proton_set_address_status` | write | `WRITES` | `enabled*`:boolean, `id*`:string | `ok*`:boolean |
| `proton_set_catchall` | write | `WRITES` | `destination_address_id*`:string, `domain_id*`:string | `ok*`:boolean |
| `proton_update_address` | write | `WRITES` | `display_name`:string, `id*`:string, `signature`:string | `ok*`:boolean |
| `proton_update_core_settings` | write | `WRITES` | `crash_reports`:boolean, `telemetry`:boolean | `settings*`:object |
| `proton_update_draft` | write | `WRITES` | `bcc`:array<string>, `body`:string, `cc`:array<string>, `from_address_id`:string, `id*`:string, `mime_type`:string, `subject`:string, `to`:array<string> | `message*`:object |
| `proton_update_mail_settings` | write | `WRITES` | `display_name`:string, `signature`:string | `settings*`:object |
| `proton_verify_custom_domain` | write | `WRITES` | `id*`:string | `domain*`:object |
| `proton_whoami` | read | — | — | `email*`:string, `max_space_bytes*`:integer, `name`:string, `persist_degraded`:boolean, `persist_error`:string, `used_space_bytes*`:integer |
