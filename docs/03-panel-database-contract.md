# 03. 面板与数据库合同

## 零迁移原则

`vle:2.0` 不创建表、不增加列、不修改列类型。只复用当前 SSPanel 已有表：

- `ss_node`
- `user`
- `user_traffic_log`
- `alive_ip`
- `ss_node_online_log`
- `ss_node_info`

1.0 测试期曾设计的 `vless_reality_node_config` 和 `vless_traffic_batches` 不再使用。数据库中如果已经存在，可先保留；2.0 不读写它们。

## `ss_node` 节点合同

VLESS 节点必须满足：

```text
id = node_id
sort = 15
type = 1
node_bandwidth_limit = 0 或 node_bandwidth < node_bandwidth_limit
```

`ss_node.server` 固定格式：

```text
public-host;public-port;0;tcp;reality;sni=<SNI>|pbk=<PUBLIC_KEY>|sid=<SHORT_ID>|target=<HOST:PORT>|minver=<VERSION>
```

约束：

- 完整字符串 1..255 字节；
- 必须恰好六个分号字段；
- 第三至第五字段固定为 `0;tcp;reality`；
- `sni`、`pbk`、`sid`、`target` 必须存在，`minver` 可省略；
- 选项不可重复，不接受未知选项；
- 公钥是 32 字节无填充 Base64URL；
- short ID 是最多 16 位的偶数长度十六进制，可为空；
- `minver` 只能包含数字和点，最长 32 字节；
- `chrome`、`xtls-rprx-vision`、`raw` 固定在代码中。

面板和 Go 后端分别实现同一严格解析器，禁止依赖数据库静默截断。REALITY 私钥只存在节点 secret 文件。

## `user` 授权合同

```text
enable = 1
AND expire_in > NOW()
AND transfer_enable > u + d
AND (
  is_admin = 1
  OR (class >= node_class AND (node_group = 0 OR user.node_group = node_group))
)
```

UUID 必须逐字节匹配父项目：

```text
UUIDv3(namespace=DNS, name="<user.id>|<user.passwd>")
```

用户 `passwd` 变化视为凭据轮换，并撤销旧会话。

## 流量事务

每次报告使用一个事务：

1. 按节点倍率计算用户 `u/d` 增量；
2. 更新 `user.u/d/t`；
3. 写 `user_traffic_log`，其中 `u/d` 保留原始字节；
4. 增加 `ss_node.node_bandwidth` 原始字节并更新心跳；
5. `COMMIT`。

事务内任一 SQL 失败都会整体回滚。与 V2 后端相同，本模式不使用持久化 batch ID：数据库中断后进程仍存活时会重试；容器重启或提交结果不明时可能漏记或重复一小批流量。

## 在线与状态

- `alive_ip`：按用户和规范化来源地址上报；
- `ss_node_online_log`：至少有一个活跃会话的用户数；
- `ss_node_info`：uptime 与短 load 字符串；
- `ss_node.node_heartbeat`：成功流量或遥测事务时更新。

遥测失败不回滚已经成功提交的流量事务。
