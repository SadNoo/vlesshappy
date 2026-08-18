-- vlesshappy VLESS REALITY node configuration and idempotent accounting.
-- This migration never stores the REALITY private key.

CREATE TABLE IF NOT EXISTS `vless_reality_node_config` (
  `node_id` INT NOT NULL,
  `public_host` VARCHAR(253) NOT NULL,
  `public_port` INT UNSIGNED NOT NULL,
  `server_name` VARCHAR(253) NOT NULL,
  `target` VARCHAR(300) NOT NULL,
  `reality_public_key` VARCHAR(128) NOT NULL,
  `short_id` VARCHAR(16) NOT NULL,
  `fingerprint` VARCHAR(32) NOT NULL DEFAULT 'chrome',
  `flow` VARCHAR(64) NOT NULL DEFAULT 'xtls-rprx-vision',
  `transport` VARCHAR(16) NOT NULL DEFAULT 'raw',
  `min_client_version` VARCHAR(32) NOT NULL DEFAULT '',
  `config_version` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
    ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`node_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `vless_traffic_batches` (
  `batch_id` CHAR(36) NOT NULL,
  `node_id` INT NOT NULL,
  `payload_sha256` BINARY(32) NOT NULL,
  `created_at` BIGINT NOT NULL,
  `applied_at` BIGINT NOT NULL,
  PRIMARY KEY (`batch_id`),
  KEY `idx_vless_traffic_batches_node_applied` (`node_id`, `applied_at`)
) ENGINE=InnoDB DEFAULT CHARSET=ascii;

-- A one-minute high-throughput batch can exceed signed INT. Existing panel
-- readers already handle numeric values, so widening is backward compatible.
ALTER TABLE `user_traffic_log`
  MODIFY `u` BIGINT NOT NULL,
  MODIFY `d` BIGINT NOT NULL;
