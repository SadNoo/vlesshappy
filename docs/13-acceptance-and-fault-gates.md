# 13. 验收与故障门禁

## 数据库与计费

- 临时 MySQL 中只创建父项目既有表；
- 验证 `ss_node.server` 节点加载、用户过滤、倍率计费和遥测；
- 在事务中间制造 SQL 失败，确认用户、日志和节点带宽全部回滚；
- 短时断库后保持进程存活，确认 counter 恢复并在数据库恢复后继续上报；
- 断库期间重启一次，量化并记录允许的漏记；
- 模拟提交响应不明，量化并记录允许的重复；
- 计费误差不得导致进程崩溃、协议断流或数据库部分入账。

`scripts/fault-gate.sh` 只准备 V2 模式数据库测试说明，不修改 secret 配置，也不再生成 MySQL COMMIT fault proxy。

## 协议与客户端

按 `acceptance/README.md` 执行 TCP half-close/大流量、XUDP DNS/echo/IPv4/IPv6、Mux、限制和定向撤销。

订阅必须证明：

- `?vless=1` 只输出 VLESS；
- `mu=2` 和 `mu=4` 不出现 VLESS；
- 显式 Mihomo、sing-box 和第一方混合订阅包含 VLESS；
- 单坏 VLESS 节点不破坏其他协议。

## 72 小时和 7 天

记录 RSS、VSZ、FD、goroutine、探针、数据库错误和流量差异。先完成 72 小时，再执行 168 小时。资源持续增长、会话未撤销、TCP/XUDP 失败或数据库部分入账都必须修复并重置观察窗口。
