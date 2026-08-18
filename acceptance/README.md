# 验收工具

本目录只保存无敏感信息的版本矩阵、结果模板和执行说明。本轮按项目负责人要求没有运行验收。

## 输入边界

- 节点 URI、UUID、REALITY 材料和订阅 token 必须写入操作者创建的 `0600` 临时文件；不得放进命令行、环境变量、仓库或结果日志。
- `clients.json` 中的 `status` 只有真实设备执行后才能改成 `passed` 或 `failed`。
- 第一方客户端文件尚未由项目负责人提供，因此该行保持阻塞，不以其他客户端代替。

## 每个客户端必须执行

1. 导入独立 `?vless=1`，确认 `mu=2`、`mu=4` 没有 VLESS。
2. TCP：小请求、双向大流量、half-close。
3. XUDP：DNS、普通 UDP echo、IPv4、IPv6，payload 为 1、64、512、1200 字节。
4. Mux：并发 TCP 与 XUDP，确认用户 TCP/XUDP 上限不可绕过。
5. 修改用户禁用、密码、`disconnect_ip`、forbidden IP/port、限速和连接器参数，确认定向撤销。
6. 记录 GUI 版本、实际内核版本、平台、日期、结果和脱敏日志摘要。

结果使用 `result-template.md` 复制生成，禁止覆盖历史结果。
