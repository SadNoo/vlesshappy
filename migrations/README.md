# Database migrations

`vle:2.0` does not require a database migration. VLESS REALITY uses the
existing `ss_node`, `user`, `user_traffic_log`, `alive_ip`,
`ss_node_online_log`, and `ss_node_info` tables in the same style as the V2
backend.

Do not run the removed 1.0 migration on a new installation. Existing
`vless_reality_node_config` or `vless_traffic_batches` tables from an earlier
test may be left in place while upgrading; 2.0 does not read or write them.
