# vlesshappy

- 状态：`0.2.0-rc.1 IMPLEMENTED / 本轮按要求未测试`
- 文档基线日期：`2026-08-19`
- 实施版本：`0.2.0-rc.1`
- 对应远程仓库：`https://github.com/SadNoo/vlesshappy`

`vlesshappy` 是 VLESS + RAW/TCP + REALITY + Vision + XUDP 多用户单端口后端，直接兼容本项目的 SSPanel 用户、节点、策略、倍率计费和状态表。本目录包含可构建 Go 后端、SQL migration、容器、测试、运维手册，以及未直接应用到父项目的完整面板 overlay。

## 已实现

- 固定并 vendoring Xray-core `v1.260327.0`，只增加可重放的最小会话 hook，不重写协议或密码学；
- `sort=15` 多真实用户 VLESS REALITY 入站；
- 与 `User::getUuid()` 一致的 UUIDv3 身份；
- 用户启用、到期、额度、等级、分组和节点带宽授权，用户差异增量应用；
- TCP 每用户/全局、XUDP 每用户/全局、并发握手精确上限，Mux 子流不可绕过；
- 用户与节点共享令牌桶实际限速，受控流量关闭 splice 快捷路径；
- 用户、凭据和策略变化定向撤销，`disconnect_ip` 只撤销对应来源，XUDP 迁移时原子更新来源；
- forbidden IP/CIDR、所有 DNS 候选检查、已检查 IP 固定、端口范围和 disconnect source IP；
- 活跃 IP、真实在线用户、节点 info 和 heartbeat；
- 用户原始上下行计数、冻结倍率、本地原子 outbox 和数据库 batch 幂等事务；
- REALITY 私钥与数据库密码 secret 文件、严格配置、密钥配对检查；
- 独立 `?vless=1` 与第一方混合 `links`/`mihomo` overlay；`mu=2`/`mu=4` 不注入 VLESS；
- 非 root、无 shell 的 scratch 容器构建。

## 后续验收入口（本轮未执行）

```bash
cd vlesshappy
go test -mod=vendor ./...
go test -mod=vendor -race ./...
go vet -mod=vendor ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -trimpath ./cmd/vlesshappy
sh panel-overlay/apply.sh --check
```

完整安装、迁移、密钥生成、容器运行和故障恢复见 [运行手册](docs/09-operations.md)，已完成项目和仍需真实环境验证的门禁见 [实施状态](docs/10-implementation-status.md)。

## 工作区硬边界

1. 后续新增的源码、测试、容器文件、SQL、面板 overlay 和发布材料全部放在 `vlesshappy/` 内。
2. 未经再次明确授权，不直接修改父项目的 `app/`、`config/`、`resources/`、`sql/`、`test/` 或其他目录。
3. 面板集成先在 `vlesshappy/panel-overlay/` 中按父项目相对路径交付；是否实际应用到父项目，由项目负责人单独确认。
4. 不覆盖或吸收父项目当前未提交的 SS2022 改动。
5. 目录内全部工作还必须遵守 [AGENTS.md](AGENTS.md) 的范围、敏感信息和简化实现约束。

## 已冻结的核心决策

| 编号 | 决策 |
|---|---|
| D-001 | VLESS REALITY 节点使用 `ss_node.sort = 15`；`sort=14` 保留给 SS2022。 |
| D-002 | 后端基于固定版本的当前 Xray-core，采用单一 Go 进程和最小维护补丁；不基于旧 v2lite。 |
| D-003 | 首版仅支持 `VLESS + RAW/TCP + REALITY + xtls-rprx-vision`，UDP 使用 XUDP。 |
| D-004 | 一个节点端口承载多个真实面板用户，不使用共享承运用户。 |
| D-005 | 首版用户 ID 与父项目 `User::getUuid()` 完全一致，保持现有 V2 身份兼容。 |
| D-006 | REALITY 私钥只存在于节点本地 secret 文件；数据库和订阅只保存、输出客户端所需公开材料。 |
| D-007 | 使用直接 MySQL 控制面、授权快照、持久化流量 outbox 和数据库批次幂等。 |
| D-008 | 独立订阅为 `?vless=1`；`mu=2`/`mu=4` 不加入 VLESS；第一方混合 `links`、`mihomo` 显式聚合 VLESS。 |
| D-009 | `?mu=2` 继续保持 VMess-only，不在首版静默改变旧订阅语义。 |
| D-010 | VLESS 的 UDP 能力定义为 XUDP 端到端 UDP 代理，不宣称是原生 UDP 入站。 |
| D-011 | 能用优先；采用满足当前范围的最短直接实现，不建设通用协议平台，不堆叠未获批准的功能。 |

## 文档清单

1. [AGENTS.md](AGENTS.md)：所有后续工作的目录边界、授权、敏感信息和简化实现硬约束。
2. [01-scope-and-decisions.md](docs/01-scope-and-decisions.md)：范围、术语、冻结决策、待确认项。
3. [02-architecture.md](docs/02-architecture.md)：总体架构、组件、状态机和代码目录。
4. [03-panel-database-contract.md](docs/03-panel-database-contract.md)：父项目数据合同、新表和事务语义。
5. [04-subscription-client-contract.md](docs/04-subscription-client-contract.md)：`?vless=1`、Mihomo、第一方客户端及字段映射。
6. [05-runtime-reliability.md](docs/05-runtime-reliability.md)：授权同步、会话撤销、可靠计费和故障恢复。
7. [06-security-threat-model.md](docs/06-security-threat-model.md)：秘密边界、REALITY target 风险、数据库和容器安全。
8. [07-test-release-acceptance.md](docs/07-test-release-acceptance.md)：测试矩阵、长稳指标和上线门槛。
9. [08-implementation-plan.md](docs/08-implementation-plan.md)：后续直接实施顺序、每阶段交付物和停止条件。
10. [09-operations.md](docs/09-operations.md)：构建、密钥、配置、容器和故障恢复手册。
11. [10-implementation-status.md](docs/10-implementation-status.md)：当前实现、未执行验证和外部阻塞项。
12. [11-validation-report-20260819.md](docs/11-validation-report-20260819.md)：只适用于 0.1.0 的历史验证证据。
13. [12-session-control-and-limits.md](docs/12-session-control-and-limits.md)：最小 Xray hook、会话、撤销、DNS 与限速实现。
14. [13-acceptance-and-fault-gates.md](docs/13-acceptance-and-fault-gates.md)：故障注入、客户端和长稳执行入口。
15. [14-release-gates.md](docs/14-release-gates.md)：SBOM、漏洞、签名、许可证和正式发布门禁。
16. [15-change-manifest-020rc1.md](docs/15-change-manifest-020rc1.md)：本次全部变更清单。
17. [16-sspanel-deferred-integration.md](docs/16-sspanel-deferred-integration.md)：SSPanel 延后集成边界与订阅契约。
18. [REFERENCES.md](docs/REFERENCES.md)：父项目代码依据和上游官方资料。

架构决策均已由项目负责人确认。面板实际文件尚待项目负责人另行提供；数据库迁移、远程 Git、镜像发布、签名和上线仍分别需要明确授权。
