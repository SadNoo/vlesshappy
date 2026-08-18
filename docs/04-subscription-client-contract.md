# 04. 订阅与客户端合同

## 1. 独立订阅入口

目标入口：

```text
/link/<token>?vless=1
```

请求与响应规则：

- 只有参数值严格等于字符串 `1` 时进入 VLESS 分支。
- `?vless=0`、空值、重复参数或未知值不得进入该分支。
- 复用父项目现有订阅 token 鉴权。
- 成功响应为标准 Base64 编码的 UTF-8 文本；解码后每行一个 `vless://` URI，最后保留换行。
- 不进行 Base64 URL-safe 二次变体，除非兼容测试证明客户端要求。
- Header 至少包含 `Content-Type: application/octet-stream; charset=utf-8` 和 `Cache-Control: no-store`。
- 用户禁用、到期、额度耗尽或没有可用节点时返回空内容，不泄漏原因。
- 节点顺序按 `name`、`id` 稳定排序，避免无意义订阅变更。

## 2. 标准 VLESS URI

首版模板：

```text
vless://<uuid>@<host>:<port>?encryption=none&flow=xtls-rprx-vision&security=reality&sni=<sni>&fp=chrome&pbk=<reality-key>&sid=<short-id>&type=tcp#<node-name>
```

规则：

- UUID 使用本项目定义的 UUIDv3。
- IPv6 host 必须加 `[]`。
- query value 和 fragment 使用 RFC 3986 百分号编码。
- `encryption=none` 显式输出，首版不输出 VLESS Encryption。
- `security=reality`、`flow=xtls-rprx-vision`、`fp=chrome`、`pbk`、`type=tcp` 不得省略。
- `sid` 根据节点配置输出；允许空 short ID 时仍按固定客户端矩阵验证。
- `spx`、`pqv`、XHTTP、gRPC 等未启用字段不得输出空占位。
- 订阅中永远不出现 REALITY 私钥、数据库地址、节点内部监听地址或真实用户邮箱。

## 3. 字段映射

内部模型使用与客户端实现无关的名称，渲染时映射：

| 内部字段 | Xray 当前配置 | VLESS URI | Mihomo | sing-box |
|---|---|---|---|---|
| `reality_public_key` | `password` | `pbk` | `reality-opts.public-key` | `tls.reality.public_key` |
| `short_id` | `shortId` | `sid` | `reality-opts.short-id` | `tls.reality.short_id` |
| `server_name` | `serverName` | `sni` | `servername` | `tls.server_name` |
| `fingerprint` | `fingerprint` | `fp` | `client-fingerprint` | `tls.utls.fingerprint` |
| `flow` | `flow` | `flow` | `flow` | `flow` |

不得把某个客户端的字段名直接作为数据库 schema，避免上游重命名影响控制面。

## 4. Mihomo/Clash 输出

目标节点结构：

```yaml
- name: "<unique-name>"
  type: vless
  server: <public-host>
  port: <public-port>
  uuid: <uuid>
  udp: true
  network: tcp
  tls: true
  servername: <server-name>
  flow: xtls-rprx-vision
  packet-encoding: xudp
  client-fingerprint: chrome
  reality-opts:
    public-key: <reality-public-key>
    short-id: <short-id>
  encryption: ""
```

要求：

- `?mu=2` 和 `?mu=4` 均保持原有语义，不加入 VLESS。
- 只并入第一方混合订阅 API `format=mihomo`；该入口是明确的多协议聚合，不是 VLESS 独立入口。
- 节点名经过现有唯一化逻辑；同名节点不得覆盖。
- `skip-cert-verify` 不得设为 `true`。
- 不下发远程控制器、LAN 开放或其他本地客户端安全设置。

## 5. 第一方客户端聚合

现有第一方混合订阅 `links` 格式加入未 Base64 包裹的逐行标准 VLESS URI；现有第一方混合订阅 `mihomo` 格式加入上述代理对象。独立通用订阅仍只使用 `?vless=1`。

capabilities 建议增加：

```json
{
  "subscription": {
    "vless_reality": true,
    "vless_transport": "raw",
    "vless_flow": "xtls-rprx-vision",
    "vless_udp": "xudp"
  }
}
```

不得把 `vless_udp` 简写成 `native`。

## 6. 兼容性层级

### P0 必须通过

- v2rayN：标准 VLESS URI 订阅。
- v2rayNG：标准 VLESS URI 订阅。
- 当前项目生成的 Mihomo 配置：Linux、Windows、macOS 真实核心启动与 TCP/UDP 探测。
- 第一方客户端当前使用的 iOS、Android、macOS、Windows、Linux 配置消费链。

### P1 建议通过

- sing-box JSON 手工等价配置和标准 VLESS URI 导入。
- 至少一个 iOS 上支持 REALITY/XUDP 的当前客户端。

发布不能只写“最新版可用”。`07-test-release-acceptance.md` 的客户端矩阵必须记录具体客户端版本、内核版本、订阅格式和测试日期。

## 7. 兼容性保护

- `?mu=2` 保持 VMess-only。
- `?mu=4` 保持既有 Clash/Mihomo 语义，不由本 overlay 注入 VLESS；混合协议由第一方 `links`/`mihomo` 聚合承担。
- SSR、SSD、传统 SS 订阅不尝试表达 REALITY。
- 独立 VLESS 订阅失败不得影响现有 SSR、VMess、SS2022 和 Mihomo 中其他节点。
- 单个 VLESS 节点配置非法时跳过该节点并产生管理员可见告警；不得把私钥或完整 URI写入日志。
- 若所有 VLESS 节点均无效，独立订阅返回空内容；聚合订阅仍返回其他协议节点。
