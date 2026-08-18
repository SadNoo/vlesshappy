# 参考依据

文档基线：2026-08-19。实现开始时必须重新验证上游版本和字段是否变化，并固定精确 commit/tag。

## 父项目代码依据

- `app/Controllers/LinkController.php`：现有订阅入口、`?ss2022=1`、`mu=2/4` 和 Clash 聚合。
- `app/Services/ClientConfig.php`：第一方客户端 `links`、`mihomo` 聚合。
- `app/Services/SS2022.php`：`sort=14`、节点过滤、独立订阅模式。
- `app/Controllers/Client/ClientApiV1Controller.php`：iOS、Android、macOS、Windows、Linux 客户端能力和配置输出。
- `app/Models/User.php`：`User::getUuid()` UUIDv3 DNS 映射。
- `app/Controllers/Mod_Mu/UserController.php`：现有用户过滤、流量倍率、alive IP 和节点状态语义。
- `app/Command/Job.php`：设备 IP 数和 `disconnect_ip` 处理。
- `sql/glzjin_all.sql`：`user`、`ss_node`、`alive_ip`、`user_traffic_log`、节点信息和在线日志 schema。

## Xray/REALITY 官方资料

- Xray-core：<https://github.com/XTLS/Xray-core>
- VLESS inbound：<https://xtls.github.io/en/config/inbounds/vless.html>
- REALITY：<https://xtls.github.io/en/config/transports/reality.html>
- Transport：<https://xtls.github.io/en/config/transport.html>
- Statistics：<https://xtls.github.io/en/config/stats.html>
- API：<https://xtls.github.io/config/api.html>
- VLESS 分享链接提案：<https://github.com/XTLS/Xray-core/discussions/716>

实现注意：当前 Xray 文档中 REALITY 客户端字段使用 `password`，标准 URI 仍使用 `pbk`。不得依靠记忆或旧版示例映射字段。

## 客户端官方资料

- v2rayN 订阅说明：<https://github.com/2dust/v2rayN/wiki/Description-of-subscription>
- v2rayNG：<https://github.com/2dust/v2rayNG>
- Mihomo VLESS：<https://wiki.metacubex.one/en/config/proxies/vless/>
- sing-box VLESS：<https://sing-box.sagernet.org/configuration/outbound/vless/>
- sing-box TLS/Reality：<https://sing-box.sagernet.org/configuration/shared/tls/>

## 相关协议对照

- Hysteria2 protocol：<https://hy2.app/docs/developers/Protocol/>
- TUIC v5：<https://github.com/tuic-protocol/tuic>
- AnyTLS protocol：<https://github.com/anytls/anytls-go/blob/main/docs/protocol.md>

这些协议只作为数据面和测试设计参考，不进入 VLESS REALITY 首版实现范围。
