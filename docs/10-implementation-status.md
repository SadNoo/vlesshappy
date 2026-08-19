# 10. 2.2 实施状态

基线日期：2026-08-19；后端版本：`2.2.0`。

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
| 2.1镜像内安全初始化 | 保留 |
| 2.2固定内部 target与受管 Caddy | 已实现，等待公网域名签发验收 |
| 自定义非443公网 VLESS端口 | 已实现 |
| 固定纯 TCP中转入口 | 节点公开地址/端口合同已支持 |

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
- 使用真实 DNS和公网 TCP/80完成 Caddy首次签发、续期与中转80透传验收；

2.0 验收记录见 `docs/18-vle-2.0-v2-compatible-mode.md`；2.2 自动化、敏感信息和发布验证记录见 `docs/22-vle-2.2-validation.md` 及本次发布报告。
