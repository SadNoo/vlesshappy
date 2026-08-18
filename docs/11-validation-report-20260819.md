# 11. 本地验收报告（2026-08-19）

> 历史报告：以下结果只对应 0.1.0。0.2.0-rc.1 引入 vendored Xray hook、会话注册表、实际限速和定向 reload；项目负责人要求本轮暂不测试，因此本报告不得作为新版本通过证据。

## Go 门禁

- `go test ./...`：通过；
- `go test -race ./...`：通过；
- `go vet ./...`：通过；
- Linux/amd64 静态、strip 构建：通过；
- Linux/arm64 静态、strip 构建：通过；
- 固定 Xray-core 真实 JSON 配置编译：通过。

本地二进制：

| 文件 | SHA-256 |
|---|---|
| `dist/vlesshappy-linux-amd64` | `354badc2d9067102dc3adb526f05f4af4361c164696839f22035007963c76a51` |
| `dist/vlesshappy-linux-arm64` | `9559ba8b84bc3fdad753cee71f5046d7002f40adc08ce30f9b1fcc7cf2d7dae5` |

`dist/` 是本地忽略生成物，不属于源码提交内容。

## MySQL 集成

使用临时 MySQL 8.4 镜像：

```text
mysql@sha256:b3b90af2a6552ae30c266fdb7d5dd55f3afb72404bb78d37fe8a23eb857fd3fb
```

已验证：

- migration 在父项目最小 `user_traffic_log` 基表上连续执行两次均成功，两个新表存在且 `u/d` 为 BIGINT；
- `sort=15` 节点与用户过滤；
- UUID/策略快照读取；
- 1.5 倍率上下行入账；
- 同一个 batch 重放两次只入账一次；
- 用户 raw 日志、节点 raw bandwidth；
- alive IP、online、node info 和 heartbeat。

测试过程中发现 MySQL 8.4 把 `load` 作为保留字，已修正 SQL 为反引号字段并重新通过。临时容器和密码文件均已删除。

## 面板 overlay

- 两个 patch 对当前父项目 `git apply --check`：通过；
- `VlessReality.php` 与模型 PHP 7.4 语法：通过；
- 标准 URI、IPv6、REALITY query、Mihomo XUDP、Base64 订阅、禁用用户和严格端口测试：通过；
- 测试镜像：`php@sha256:620a6b9f4d4feef2210026172570465e9d0c1de79766418d3affd09190a7fda5`；
- overlay 未写入父项目。

## 容器

- 本地标签：`vlesshappy:local-test`；
- 本地清单摘要：`sha256:77fff61a7fb5266da3484daf63886c7bfb89fe1f00ba2c3729e4f39041e9e2da`；
- 平台：Linux/amd64；
- 运行用户：`65532:65532`；
- 镜像大小：约 12.6 MB（Docker inspect）；
- `version` 与非 root tmpfs keygen：通过；
- `/bin/sh`：不存在，符合 scratch 最终层；
- 未 push、未 tag 远程版本。

## 尚不能由本机短验收代替

真实客户端 TCP/XUDP 矩阵、精确连接/会话 hook、真实限速、MySQL 生产 TLS/故障注入、72 小时与 7 天长稳仍按 `10-implementation-status.md` 作为正式发布门禁。
