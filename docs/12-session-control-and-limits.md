# 12. 会话控制与限制实现

## 最小 Xray hook

固定上游与补丁位于 `core/` 和 `vendor/`。补丁只提供四种能力：VLESS header 握手取得/释放名额、逻辑 dispatcher 会话打开、流量等待、Mux/XUDP 逻辑会话结束回调。业务规则全部在 `internal/session/registry.go`，上游 VLESS、REALITY、Vision、XUDP 编解码和密码学文件不改。

一个 direct VLESS TCP 请求算一个 TCP 会话；Mux 的每个目标子流分别经过 dispatcher，按 TCP 或 XUDP 计数；XUDP 在 Mux 重连的一分钟保留期内仍算同一个 XUDP 会话，真正过期或取消后释放。所有释放和撤销均为幂等。

XUDP 在保留期内从新来源重连时不新建会话，但会原子迁移该租约的来源计数；新来源仍需通过 `disconnect_ip` 和连接器上限。迁移被拒绝时关闭原逻辑 XUDP 会话，不能借重连绕过来源策略。

## 默认限制

严格配置 `limits` 默认值：

| 字段 | 默认 |
|---|---:|
| `tcp_per_user` | 800 |
| `tcp_global` | 3000 |
| `concurrent_handshakes` | 1024 |
| `xudp_per_user` | 128 |
| `xudp_global` | 2048 |
| `dns_resolve_timeout_seconds` | 10 |

检查顺序是：握手名额、认证、来源、端口、DNS/IP、连接器、用户会话上限、全局会话上限。注册表是唯一策略权威；Xray routing 不再复制用户 forbidden/disconnect 规则，因此解除策略也能在下一快照生效。拒绝原因只以固定类别和计数记录，不记录 UUID、完整目标、订阅或密钥。

## 限速

用户 `node_speedlimit` 是该用户所有上下行/会话共享的令牌桶；节点 `node_speedlimit` 是所有用户共享的第二个令牌桶。两者都非零时必须同时取得配额，因此自然执行更严格的组合限制。Mbps 按十进制 `1,000,000 bit/s` 换算，最小 burst 为 64 KiB。

受控会话把 `CanSpliceCopy` 设为禁用，确保 RAW/Vision 数据经过限速 reader/writer；这会牺牲 splice 零拷贝性能，但防止限速和计费旁路。协议与 Vision 能力不变。

## 定向撤销与动态用户

- 用户被删除、禁用、到期、超额或 UUID 变化：先从 Xray 用户验证器移除/替换，再取消该用户会话。
- forbidden、限速或连接器策略变化：取消该用户会话并用新策略接受后续请求。
- 新增 `disconnect_ip`：只取消该用户对应来源地址的会话。
- 连接器上限降低：保留最早出现的来源，取消超出的较新来源。
- REALITY key、SNI、target、short ID、flow 或最低客户端版本变化：完整重启嵌入 core；这是节点级数据面变化。

用户差异应用失败会回滚 Xray 用户操作；新快照只在用户操作成功后发布。每次快照切换前先按旧倍率抽取并持久化流量。被删除用户进入 retired 计数集合，直到其会话全部释放且计数再次归零才清除，避免定向撤销收尾流量丢失；节点倍率变化则执行全 core drain/restart，避免跨倍率计费。

## DNS 最终地址策略

IP 目标直接检查。域名目标在建立逻辑会话时解析全部 A/AAAA 候选；任一候选命中用户 forbidden IP/CIDR 时拒绝整个请求。全部通过后按稳定顺序选择一个地址并把 Xray destination 固定为该地址，因此 freedom outbound 不会二次解析到未经检查的 rebinding 地址。每个新逻辑会话重新解析，不跨策略缓存。
