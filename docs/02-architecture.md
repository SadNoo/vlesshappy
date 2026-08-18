# 02. 总体架构

## 系统关系

```text
客户端 -> VLESS/REALITY 数据面 -> 目标 TCP/XUDP
                    |
SSPanel -> MySQL <- vlesshappy 控制面
```

一个进程服务一个 `node_id`，包含一个 RAW/TCP 监听端口、一个 REALITY 私钥、多名真实面板用户、一个 MySQL 连接池和一个会话注册表。不开放公网管理 API。

## 数据库模型

2.0 与现有 V2 后端采用相同思路：

- `ss_node.sort=15` 区分 VLESS REALITY；
- 节点公开参数编码在现有 `ss_node.server`；
- 用户从现有 `user` 表读取；
- 流量事务写入 `user`、`user_traffic_log` 和 `ss_node`；
- 在线与节点状态写入 `alive_ip`、`ss_node_online_log`、`ss_node_info` 和 `ss_node.node_heartbeat`；
- 不新增 VLESS 专用表，不执行 `ALTER TABLE`。

## 核心组件

### 配置与秘密

普通运行参数来自严格 JSON。数据库密码和 REALITY 私钥分别从权限受限的绝对路径文件读取；未知字段、重复字段、宽松类型和不安全 secret 文件均拒绝。

### 数据库适配器

从 `ss_node.server` 解析：

```text
public-host;public-port;0;tcp;reality;sni=...|pbk=...|sid=...|target=...|minver=...
```

`chrome`、`xtls-rprx-vision` 和 `raw` 是固定值。完整字符串最多 255 字节，未知、重复或缺失选项均拒绝。

### 授权与数据面

授权快照包含 UUID、额度、等级、分组、限速、连接器、禁止目标和禁止来源。Xray-core 负责 VLESS、REALITY、Vision、Mux 和 XUDP；本项目的最小 hook 负责会话索引、限制、限速和定向撤销。

### 流量

每个报告周期交换 Xray 用户计数器并在一个 MySQL 事务中：

1. 增量更新 `user.u/d/t`；
2. 写入原始 `user_traffic_log`；
3. 增加 `ss_node.node_bandwidth` 并更新心跳；
4. 提交成功后保留新计数基线。

事务失败时把计数恢复到进程内 Xray counter，下一周期重试。该模型不提供跨重启持久化或提交结果不明时的严格去重，因此数据库异常期间允许少量漏记或重复；这项取舍已经由项目负责人确认。

## 生命周期

```text
BOOTSTRAP -> SERVING -> DEGRADED_AUTH -> DRAINING -> STOPPED
```

- 首次启动必须连接数据库、取得完整快照并验证本地私钥与面板公钥匹配后才监听。
- 普通流量写入失败只告警并恢复进程内计数，不主动中断用户。
- 授权读取失败时保留最后快照；超过 `auth_stale_seconds` 后停止服务。
- 节点隐藏、类型错误、带宽耗尽或 REALITY 配置无效时 fail-closed。
- 状态目录只用于单实例文件锁，不保存流量或秘密。
