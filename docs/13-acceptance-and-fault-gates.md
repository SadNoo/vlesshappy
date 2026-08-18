# 13. 验收与故障门禁

本轮只交付工具，没有执行。任何结果文件必须由真实运行产生，禁止提前填写“通过”。

## Outbox 与数据库

- `internal/accounting/fault_injection_test.go` 注入短写、文件 fsync、rename、目录 fsync；目录 fsync 失败必须标记“批次已发布”，防止恢复相同计数。
- 既有测试覆盖截断 `.tmp`、hash 篡改、容量和 batch 幂等。
- `cmd/vlesshappy-mysql-faultproxy` 转发 MySQL `COMMIT` 后丢弃响应，模拟“数据库已提交、客户端收到 EOF”；只允许用于临时、TLS-disabled 的测试数据库。
- `scripts/fault-gate.sh` 生成不覆盖的结果目录和后续步骤，不修改输入配置。

仍需在一次性 MySQL 8.4 容器执行：只读、锁等待、死锁、重启、长断网、TLS 正确/错误 CA、提交响应丢失重放。禁止在生产数据库切换 `read_only` 或执行故障注入。

## 真实协议与客户端

`acceptance/clients.json` 固定 v2rayN 7.24.2、v2rayNG 2.2.6（Xray-core v26.6.27）和 Mihomo 1.19.28；第一方客户端等待项目负责人提供。每个客户端按 `acceptance/README.md` 执行 TCP half-close/大流量、XUDP DNS/echo/IPv4/IPv6/1/64/512/1200、Mux、限制和定向撤销。

订阅必须同时证明：

- 独立 `?vless=1` 只输出 VLESS；
- `mu=2` 和 `mu=4` 不出现 VLESS；
- 第一方混合 `links` 和 `mihomo` 包含 VLESS 且旧协议不回归。

## 72 小时和 7 天

`scripts/soak.sh` 只接受 72 或 168 小时、绝对新结果目录、目标 PID 和不含秘密的 probe 命令。每分钟记录 RSS、VSZ、FD 和探针结果；探针首次失败立即结束。真实 probe 必须从权限为 `0600` 的文件读取连接材料，不能把 URI/UUID 放在命令行或日志中。

先完成 72 小时，再执行 168 小时；任何计费差异、资源单调增长、会话未撤销、TCP/XUDP 失败或 outbox 未清空都重置观察窗口。
