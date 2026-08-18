# 10. 2.0 实施状态

基线日期：2026-08-19；后端版本：`2.0.0`。

## 已实现

| 能力 | 状态 |
|---|---|
| VLESS + RAW/TCP + REALITY + Vision + XUDP | 保留 |
| 会话、握手、用户/全局限制与限速 | 保留 |
| 用户、凭据和策略定向撤销 | 保留 |
| DNS 最终地址、端口和来源策略 | 保留 |
| `sort=15` 从 `ss_node.server` 加载 | 2.0 已实现 |
| 不新增/修改数据库表 | 2.0 已实现 |
| V2 模式现有表流量事务 | 2.0 已实现 |
| 独立 `?vless=1` | 保留 |
| 显式 Mihomo、sing-box、第一方混合订阅 | overlay 已整理 |
| `mu=2`/`mu=4` 不加入 VLESS | 保留 |

## 2.0 已移除

- `vless_reality_node_config` 模型与查询；
- `vless_traffic_batches` 查询与迁移；
- 本地流量 outbox、容量配置和重放；
- COMMIT 响应丢失测试代理；
- 1.0 的 migration SQL。

删除这些组件是项目负责人明确确认的取舍：流量基本充足，允许数据库异常期间少量漏记或重复，以换取与 V2 后端一致的零迁移部署和更短实现。

## 仍需真实环境验证

- 实际生产 schema 的只读兼容性；
- v2rayN、v2rayNG、Mihomo 和 sing-box 的 TCP/XUDP；
- 数据库短断、恢复和重启时的实际流量误差；
- Debian 11 及以上宿主机运行；
- 72 小时和 7 天长稳；
- 将 overlay 合并到项目负责人提供的实际 SSPanel 文件。

本轮自动化、敏感信息扫描、GitHub 和 Docker 发布结果记录在 `docs/18-vle-2.0-v2-compatible-mode.md`。
