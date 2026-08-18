# 15. 0.2.0-rc.1 变更清单

本次只修改 `vlesshappy/`，未修改父项目、相邻项目、父仓库 Git 状态、数据库或远程服务。

## 八项交付映射

| 项目 | 本轮交付 | 主要位置 |
|---|---|---|
| 1. 固定 core 与最小 hook | vendored Xray、唯一可重放补丁、离线构建 | `vendor/`、`core/`、`Dockerfile`、`Makefile` |
| 2. 精确会话与握手限制 | TCP、Mux 子流、XUDP、握手和连接器统一注册表 | `internal/session/`、`internal/config/` |
| 3. 实际共享限速 | 用户与节点双层令牌桶，禁用受控链路 splice 旁路 | `internal/session/registry.go`、core patch |
| 4. 定向撤销与增量重载 | 用户/凭据/策略/来源撤销、Xray 用户差异同步和回滚 | `internal/xrayadapter/`、`internal/lifecycle/` |
| 5. 计费故障闭环 | retired 尾流量、outbox 分阶段故障缝、COMMIT 响应丢失代理 | `internal/accounting/`、`cmd/vlesshappy-mysql-faultproxy/` |
| 6. 真实客户端与长稳准备 | 固定客户端矩阵、验收模板、72/168 小时采集器 | `acceptance/`、`scripts/soak.sh` |
| 7. 发布门禁 | test/race/vet、双架构、OCI、SBOM、漏洞、校验和和签名入口 | `scripts/release-gate.sh` |
| 8. 订阅与 SSPanel 边界 | 独立 `?vless=1`；`mu=2`/`mu=4` 不加入；第一方混合加入；实际集成延期 | `panel-overlay/`、`docs/16-sspanel-deferred-integration.md` |

以上是源码与工具交付映射，不代表验收通过；本轮按要求没有执行任何测试或构建。

## 后端源码

- `internal/session/registry.go`：新增会话权威注册表、握手/TCP/XUDP/连接器上限、限速、DNS pin、XUDP 来源迁移、定向撤销、指标。
- `internal/xrayadapter/engine.go`：注册 hook、获取 Xray 用户管理器、增量 add/remove/update、失败回滚、节点变化分流和已移除用户尾计数保留。
- `internal/lifecycle/run.go`：增量 reload 前按旧快照结算、会话指标、节点不可用立即停服、80% outbox 告警和硬上限行为。
- `internal/config/config.go`、`packaging/config.example.json`：新增严格 `limits`。
- `internal/database/mysql.go`：删除非零限速隔离，允许注册表实际执行；用高精度加法过滤配额，避免无符号减法下溢。
- `internal/policy/policy.go`：新增 IP/CIDR 与端口运行时匹配。
- `internal/accounting/outbox.go`：新增不可导出的测试故障注入缝。
- `cmd/vlesshappy-mysql-faultproxy/main.go`：新增 COMMIT 响应丢失工具。
- `cmd/vlesshappy/main.go`：版本更新为 `0.2.0-rc.1`。

## 固定 core 与构建

- `vendor/`：纳入固定依赖源码和许可证。
- `core/patches/0001-vlesshappy-session-control.patch`：唯一 Xray 补丁。
- `core/README.md`：来源、校验、许可和补丁重放方法。
- `go.mod`：`x/time/rate` 改为直接依赖；`go.sum` 版本不变。
- `Dockerfile`、`Makefile`：固定 `-mod=vendor`，容器构建不下载依赖。
- `THIRD_PARTY_NOTICES.md`：披露 vendored MPL 源码和修改范围。

## 未执行的测试与工具

- 新增 `internal/session/registry_test.go`、`internal/accounting/fault_injection_test.go`；删除已经失效的 lifecycle 临时 connector 测试。
- 新增 `acceptance/clients.json`、`acceptance/README.md`、`acceptance/result-template.md`。
- 新增 `scripts/fault-gate.sh`、`scripts/soak.sh`、`scripts/release-gate.sh`。
- 所有上述测试和脚本本轮均未执行。
- 仅执行 `gofmt`、静态文本/范围检查和 core patch 反向 `dry-run`；没有编译或启动任何程序。

## 面板草案与文档

- `panel-overlay/patches/sspanel-vless-reality.patch`：移除旧 `mu=4` 注入，保留独立 VLESS 和第一方混合聚合。
- `panel-overlay/apply.sh`：`--apply` 固定拒绝，只允许 check。
- `panel-overlay/README.md`：声明等待实际面板文件。
- 更新 README、运维、订阅、测试、状态和历史验证边界；新增 `docs/12` 至 `docs/16`。
