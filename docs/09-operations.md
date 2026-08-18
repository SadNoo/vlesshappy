# 09. 构建、安装与运行

## 1. 构建

要求 Go 1.26：

```bash
go test -mod=vendor ./...
go build -mod=vendor -trimpath -o dist/vlesshappy ./cmd/vlesshappy
docker build --platform linux/amd64 -t vlesshappy:0.2.0-rc.1 .
```

本项目不下载或启动外部 Xray 二进制；固定 Xray-core 源码和最小 hook 位于 `vendor/`，构建不在线替换源码。

## 2. 数据库迁移

先备份生产数据库，再人工审查并执行：

```text
migrations/001_vless_reality.up.sql
```

迁移创建公开节点配置表和幂等批次表，并把 `user_traffic_log.u/d` 扩为 BIGINT。迁移不包含私钥，不会自动创建节点记录。

## 3. 生成 REALITY 密钥

在节点上运行本项目二进制：

```bash
vlesshappy keygen \
  -private-key-file /run/secrets/vlesshappy_reality_private_key
```

命令只把私钥写入新建的 `0600` 文件，终端只显示公钥。把公钥、SNI、short ID、target、公开地址和端口填入面板 `sort=15` 节点；私钥不得复制到面板。

数据库密码同样使用权限为 `0400` 或 `0600` 的独立绝对路径文件。运行 UID 必须拥有读取 secret 和写入状态目录的权限。

## 4. 配置

从 `packaging/config.example.json` 建立 `/etc/vlesshappy/config.json`。生产数据库建议使用 `verify_ca`；`disabled` 仅适合可信隔离测试网。执行：

```bash
vlesshappy validate -config /etc/vlesshappy/config.json
```

`validate` 会连接数据库、读取首份授权、校验 schema 和节点类型，并确认节点本地私钥与面板公钥匹配，但不会监听代理端口。

## 5. 容器运行

先建立仅 UID 65532 可写的宿主状态目录，再运行：

```bash
docker run -d \
  --name vlesshappy-15 \
  --restart unless-stopped \
  --stop-timeout 120 \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --user 65532:65532 \
  -p 443:8443/tcp \
  -v /srv/vlesshappy/15/state:/var/lib/vlesshappy \
  -v /srv/vlesshappy/15/config.json:/etc/vlesshappy/config.json:ro \
  -v /srv/vlesshappy/15/secrets:/run/secrets:ro \
  vlesshappy:0.2.0-rc.1
```

VLESS UDP 由 XUDP 在同一 TCP/REALITY 会话内承载，不开放 UDP 入站端口。

## 6. 面板 overlay

当前 overlay 只在本目录：

```bash
sh panel-overlay/apply.sh --check
```

`--apply` 当前固定拒绝。项目负责人另行提供实际 SSPanel 文件并给出新授权后，再重新生成最终集成并检查：

- `/link/<token>?vless=1`；
- `?mu=2`、`?mu=4` 中不出现 VLESS 节点；
- 第一方 API 的 `format=links` 和 `format=mihomo`；
- `?mu=2` 仍为 VMess-only。

## 7. 故障处理

- 数据库写入失败：流量批次保留在 `state/outbox/`，数据库恢复后顺序重放。
- outbox 出现 `.tmp`、hash 错误或未知文件：进程拒绝启动，保留现场，禁止直接删除。
- 授权读取失败：在最大陈旧时间内继续使用最后快照；超时后关闭 Xray 并结算。
- 用户、凭据或策略变化：增量更新 Xray 用户并只撤销目标用户；新增 `disconnect_ip` 只撤销对应来源。REALITY/SNI/target 等节点数据面变化才执行全 core 重启。
- 域名目标：后端解析所有候选地址，任一命中 forbidden IP 即拒绝；通过后固定到一个已检查地址，防止二次解析 rebinding。
- 非零用户/节点 `node_speedlimit`：由共享令牌桶实际执行；受控会话关闭 splice 快捷路径以防旁路。
- SIGTERM：停止监听，落盘最终计数，尝试重放后退出；数据库仍不可用时保留 outbox。

恢复 outbox 前先复制整个状态目录并核对 batch ID、node ID 和 hash；不要手工修改 JSON。
