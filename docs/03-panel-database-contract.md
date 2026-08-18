# 03. 面板与数据库合同

## 1. 既有表读取合同

### 1.1 `ss_node`

VLESS 节点必须满足：

- `id = NODE_ID`；
- `sort = 15`；
- `type = 1`；
- `node_bandwidth_limit = 0` 或 `node_bandwidth < node_bandwidth_limit`。

使用字段：

- `traffic_rate`：用户计费倍率；
- `node_class`、`node_group`：用户可见性；
- `node_speedlimit`、`node_connector`：节点级限制；
- `node_bandwidth`、`node_bandwidth_limit`：原始节点流量和上限；
- `node_heartbeat`、`node_ip`：状态和来源识别。

`ss_node.server` 只保留父项目通用展示所需公开入口，建议格式：

```text
public-host;public-port
```

REALITY 完整配置不得塞入该 `varchar(255)` 字段。

### 1.2 `user`

用户可用条件：

```text
enable = 1
AND expire_in > database_now
AND transfer_enable > u + d
AND (
  is_admin = 1
  OR (
    class >= node.node_class
    AND (node.node_group = 0 OR user.node_group = node.node_group)
  )
)
```

必须读取：

- `id`、`passwd`；
- `u`、`d`、`transfer_enable`；
- `enable`、`expire_in`；
- `class`、`node_group`、`is_admin`；
- `node_speedlimit`、`node_connector`；
- `forbidden_ip`、`forbidden_port`、`disconnect_ip`。

时间判断使用数据库时间，避免节点与数据库时钟差导致授权分歧。

### 1.3 用户 UUID

首版必须逐字节匹配父项目：

```text
UUIDv3(namespace=DNS, name="<user.id>|<user.passwd>")
```

要求提供由真实 PHP 实现生成的固定测试向量，Go 实现不得自行猜测字符串编码或 UUID 字节序。用户 `passwd` 变化视为 VLESS 凭据轮换，旧 UUID 的既有会话必须撤销。

## 2. 新增节点配置表

迁移文件最终放在 `vlesshappy/migrations/`。草案：

```sql
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
```

约束由迁移、面板服务和后端三层验证：

- `public_port` 为 1..65535；
- `public_host/server_name/target` 严格解析；
- `short_id` 是长度不超过 16、偶数字符数的十六进制字符串，也可按明确配置允许空值；
- `fingerprint` 首版只允许 `chrome`；
- `flow` 只允许 `xtls-rprx-vision`；
- `transport` 只允许 `raw`；
- 公钥材料必须能被固定 Xray 版本解析；
- 表内绝不增加 REALITY 私钥字段。

首版每个节点只下发一个 `server_name` 和一个 `short_id`，避免客户端组合爆炸。多 SNI、short ID 轮换属于后续兼容变更。

## 3. 流量批次幂等表

```sql
CREATE TABLE IF NOT EXISTS `vless_traffic_batches` (
  `batch_id` CHAR(36) NOT NULL,
  `node_id` INT NOT NULL,
  `payload_sha256` BINARY(32) NOT NULL,
  `created_at` BIGINT NOT NULL,
  `applied_at` BIGINT NOT NULL,
  PRIMARY KEY (`batch_id`),
  KEY `idx_vless_traffic_batches_node_applied`
    (`node_id`, `applied_at`)
) ENGINE=InnoDB DEFAULT CHARSET=ascii;
```

幂等入账事务：

1. `BEGIN`。
2. 尝试插入 `batch_id`。
3. 若主键已存在，校验 `node_id` 和 `payload_sha256` 相同；相同则作为已提交成功返回，不再计费；不同则报严重一致性错误。
4. 对每个用户执行原子 `u/d` 增量。
5. 写入 `user_traffic_log`，其中 `u/d` 为原始流量、`rate` 为本批次冻结的节点倍率。
6. `ss_node.node_bandwidth` 增加所有用户原始上下行总和。
7. 更新 `user.t`，写入批次 `applied_at`。
8. `COMMIT`。

同一批次内若任一 SQL 失败，必须整体回滚，不允许出现“用户已扣流量但节点带宽未更新”等部分状态。

批次使用生成时冻结的倍率，重放时不得重新读取新倍率，否则数据库恢复后会改变历史账单。

## 4. 活跃 IP 与节点状态

- `alive_ip`：每 60 秒按 `user_id + canonical_ip` 去重后追加；IPv4-mapped IPv6 必须归一化。
- `ss_node_online_log`：记录当前至少一个活跃会话的真实用户数，不用流量批次条目数代替。
- `ss_node_info`：至少保存 uptime 和规范化 load 字符串；详细运行指标留在本地结构化日志/metrics。
- `ss_node.node_heartbeat`：每次成功状态事务后更新。

遥测失败不得导致流量批次丢失；流量计费与普通遥测使用不同重试队列和优先级。

## 5. 策略语义

- `forbidden_ip` 支持单 IP 和 CIDR；域名目标解析后对最终每个候选 IP 检查。
- `forbidden_port` 支持单端口和闭区间；非法规则使该用户 fail-closed。
- `disconnect_ip` 是规范化来源 IP 集合；匹配时拒绝新会话并关闭已存在的相同来源会话。
- `node_connector > 0` 时按最近 60 秒真实来源 IP 数量限制设备；超限后的处理要与父项目 `disconnect_ip` 机制一致。
- 用户额度或节点带宽达到上限时，下一次授权同步关闭相应既有会话，并立即拒绝新会话。

## 6. 面板 overlay 合同

后续面板集成文件全部先生成在：

```text
vlesshappy/panel-overlay/app/...
vlesshappy/panel-overlay/config/...
vlesshappy/panel-overlay/resources/...
vlesshappy/panel-overlay/test/...
```

overlay 必须包含来源路径清单和应用说明，但不得自动写入父项目。应用 overlay 前必须重新检查父项目 SS2022 未提交改动，避免覆盖同一控制器和模板。
