# 最新 SSPanel 文件与 VLESS 合并预览

本目录以“SS 订阅格式”任务在 2026-08-15 的最终文件为基线，只用于审查和后续生成最终部署文件，不会自动写入父项目。

## 已合并的完整文件

| 文件 | VLESS 修改 |
|---|---|
| `app/Controllers/LinkController.php` | 严格 `?vless=1`、显式 `?clash=1` 聚合、`?sing-box=1` 聚合、订阅地址 |
| `app/Controllers/UserController.php` | `sort=15` 节点地址、在线人数、心跳和安全展示 |
| `app/Controllers/VueController.php` | Vue 节点列表识别 `sort=15`，使用公开入口 |
| `resources/views/material/user/index.tpl` | General 区域增加 VLESS REALITY 独立订阅 |

这些文件已经保留基线中的独立 `?ss2022=1`、Clash、Surge v5、sing-box、Loon 和最新用户中心入口。

## 新增文件

以下文件仍以 `panel-overlay/` 下的路径为唯一副本，部署时复制到面板同名路径：

- `app/Services/VlessReality.php`
- `resources/views/material/admin/node/vless_reality_fields.tpl`

管理员节点控制器、节点模型和 create/edit 模板继续查看：

- `patches/sspanel-vless-reality-admin.patch`
- `patches/sspanel-vless-reality.patch` 中的 `app/Models/Node.php` 部分

引用任务没有提供这些管理员文件的线上最新版，因此在收到实际文件前不生成可能覆盖线上改动的完整副本。

## 无需修改的最新版文件

VLESS 不进入统一 `URL::getNew_AllItems()`，而是在明确支持它的订阅方法中由 `VlessReality` 服务追加。因此以下最新版文件不需要 VLESS 改动：

- `app/Utils/Tools.php`：继续负责 SS2022 的 `后端;端口;server_key;||中转;端口` 格式；
- `app/Utils/URL.php`；
- `app/Utils/AppURI.php`；
- `app/Controllers/ConfController.php`；
- `app/Services/AppsProfiles.php`；
- `resources/conf/loon.conf`；
- `resources/conf/sing-box.json`。

这样可以避免 VLESS 自动流入 Loon、Surge、SSR、SSD 或旧兼容订阅。

## 订阅参数

| 参数 | 行为 |
|---|---|
| `?vless=1` | 仅 VLESS REALITY，标准 URI 按行排列后 Base64；重复或非 `1` 值不进入 VLESS |
| `?clash=1` | 显式混合配置，追加 Mihomo VLESS 节点 |
| `?sing-box=1` | 显式混合配置，追加 sing-box VLESS outbound |
| `?ss2022=1` | 只输出 SS2022，不加入 VLESS |
| `?mu=2` | 保持 VMess-only |
| `?mu=4` | 保持旧 Clash 兼容语义，不加入 VLESS |
| `?loon=1`、`?surge=*` | 保持原内容，不加入 VLESS |

第一方客户端 API 如已部署，则 `format=links` 和 `format=mihomo` 继续按 `patches/sspanel-vless-reality.patch` 合并 VLESS；未部署时不新增该 API。

## 节点参数

面板节点类型固定为 `sort=15`，管理员只填写公开材料：

- `vless_public_host`：客户端直连地址；使用中转时填写中转域名或 IP；
- `vless_public_port`：客户端直连端口；使用中转时填写中转入口端口；
- `vless_server_name`：REALITY SNI；
- `vless_target`：REALITY 伪装目标 `host:port`，不是中转地址；
- `vless_reality_public_key`：32 字节无填充 Base64URL 公钥；
- `vless_short_id`：不超过 16 位、偶数长度的十六进制；
- `vless_min_client_version`：可空。

保存时由 `VlessReality::encodeServer()` 编码进现有 `ss_node.server`：

```text
public-host;public-port;0;tcp;reality;sni=...|pbk=...|sid=...|target=...|minver=...
```

完整字符串不得超过现有 `VARCHAR(255)`；超长时面板明确拒绝，不允许数据库截断。编辑节点时再由同一服务解析回表单字段。

固定值不做成面板开关：`chrome`、`xtls-rprx-vision`、`raw`、`reality`、`none`、`xudp`。REALITY 私钥只放节点本地 secret 文件。

## 数据库边界

- 不新增 `vless_reality_node_config`；
- 不新增 `vless_traffic_batches`；
- 不执行 `ALTER TABLE`；
- 后端沿用 V2 模式，直接使用 `ss_node`、`user`、`user_traffic_log`、`alive_ip`、`ss_node_online_log` 和 `ss_node_info`；
- 数据库中如残留 1.0 测试表，可以暂时保留，2.0 不会读取或写入。

## 尚需线上文件

正式生成管理员端完整文件前，需要线上最新版：

1. `app/Controllers/Admin/NodeController.php`
2. `app/Models/Node.php`
3. `resources/views/material/admin/node/create.tpl`
4. `resources/views/material/admin/node/edit.tpl`

如果线上已部署第一方客户端 API，还需要其最新版：

1. `app/Services/ClientConfig.php`
2. `app/Controllers/Client/ClientApiV1Controller.php`

收到这些文件后再生成最终完整副本，不直接覆盖父项目。
