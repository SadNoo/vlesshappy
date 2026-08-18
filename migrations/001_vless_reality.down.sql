-- Destructive rollback; review retained accounting before running manually.
DROP TABLE IF EXISTS `vless_traffic_batches`;
DROP TABLE IF EXISTS `vless_reality_node_config`;

-- user_traffic_log.u/d intentionally remain BIGINT: narrowing could lose data.
