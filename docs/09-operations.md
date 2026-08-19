# 09. 构建、安装与运行

## 2.2 推荐入口

2.2 内置 Caddy并固定 REALITY target。管理员先为 SNI创建指向节点或中转入口的 DNS记录，确保公网 TCP/80 最终到达节点容器 8080，然后运行：

```bash
docker volume create vle-node-data
docker run --rm -it --read-only -v vle-node-data:/data sadno/vle:2.2 setup
```

向导生成的面板字符串不包含 `target`：

```text
public-host;public-port;0;tcp;reality;sni=...|pbk=...|sid=...
```

公网 VLESS端口为2053时，正式运行至少映射：

```bash
docker run -d \
  --name vle-node \
  --restart unless-stopped \
  --stop-timeout 120 \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  -p 80:8080/tcp \
  -p 2053:8443/tcp \
  -v vle-node-data:/data \
  sadno/vle:2.2
```

80只用于 ACME HTTP-01；2053可替换为任意未占用且非80的公网 TCP端口。Caddy HTTPS不发布，REALITY在容器内部转发到 `127.0.0.1:9443`。如果SNI解析到中转IP，中转还必须把 TCP/80原样转发到节点的80入口。

## 2.1 兼容入口

2.1 首次部署流程继续记录在 [19-vle-2.1-secure-setup.md](19-vle-2.1-secure-setup.md)。它仍要求面板字符串带 `target=`，不启用 Caddy。

```bash
docker volume create vle-node-data
docker run --rm -it --read-only -v vle-node-data:/data sadno/vle:2.1 setup
```

完成面板保存和校验后，向导会按选择的公开端口输出最终 `docker run` 命令。下面的手工配置方式继续供 2.0 和 2.1 高级部署使用。

## 构建

```bash
go test -mod=vendor ./...
go build -mod=vendor -trimpath -o dist/vlesshappy ./cmd/vlesshappy
docker build --platform linux/amd64 -t sadno/vle:2.0 .
```

固定 Xray-core 位于 `vendor/`，构建不下载外部 Xray 二进制。

## 数据库

2.0 不执行数据库迁移。只需确认面板现有表可用，不要运行 1.0 的旧 migration。

如果测试环境已经存在以下旧表，可以保留或在完成备份后另行清理；2.0 不读写：

```text
vless_reality_node_config
vless_traffic_batches
```

## 节点参数

面板节点类型选择 `sort=15`。管理员表单会自动生成 `ss_node.server`：

```text
public-host;public-port;0;tcp;reality;sni=...|pbk=...|sid=...|target=...|minver=...
```

2.2 受管模式删除 `target=`；面板 overlay 需按单独交付移除该输入和必填校验。后端仍兼容旧字符串，但旧字符串必须使用没有 `caddy_dir` 的手工运行配置。

不要手工把 REALITY 私钥放入该字段。私钥只在节点生成：

```bash
vlesshappy keygen -private-key-file /run/secrets/vlesshappy_reality_private_key
```

命令只向终端显示公钥，私钥文件权限为 `0600`。

## 配置与校验

从 `packaging/config.example.json` 建立配置。数据库密码使用独立 secret 文件。生产数据库建议 `verify_ca`；`disabled` 只用于可信测试网络。

```bash
vlesshappy validate -config /etc/vlesshappy/config.json
```

校验会连接数据库、解析 `ss_node.server`、读取授权，并验证本地私钥与公开公钥匹配，但不监听端口。

## Docker

状态目录仍用于单实例锁：

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
  sadno/vle:2.0
```

XUDP 在同一 TCP/REALITY 会话内承载，不开放 UDP 入站端口。

## SSPanel overlay

```bash
sh panel-overlay/apply.sh --check
```

`--apply` 固定拒绝。部署前按 `panel-overlay/merged/README.md` 将完整文件和补丁合并到实际面板，重点验证：

- `/link/<token>?vless=1`；
- `?mu=2`、`?mu=4` 不出现 VLESS；
- 明确支持的 Clash/Mihomo、sing-box 和第一方混合订阅；
- 管理员保存后 `ss_node.server` 不超过 255 字节。

## 故障边界

- 流量写入失败：恢复进程内计数并继续服务，数据库恢复后再报；
- 数据库故障期间重启：未上报流量可能丢失；
- 提交成功但响应丢失：下一次可能重复一批流量；
- 授权读取失败：最大陈旧时间内使用旧快照，超时后停止；
- 节点、REALITY 或 secret 无效：拒绝启动或停止服务；
- SIGTERM：尝试最后一次流量事务，失败时记录错误后退出。

这些计费误差只影响面板统计，不改变 VLESS、REALITY、TCP、Mux 或 XUDP 协议行为。
