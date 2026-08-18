# SSPanel overlay

本目录是可审查的面板集成成品，当前没有写入父项目。

`merged/` 是以“SS 订阅格式”任务的最新文件为基线生成的完整合并预览。优先从该目录查看 `LinkController.php`、`UserController.php`、`VueController.php` 和用户中心模板；它们保留最新 SS2022、Clash、Surge、sing-box、Loon 和中转语义，并加入 VLESS。原 `patches/` 继续保留，供管理员节点文件和历史差异对照。

包含：

- `app/Services/VlessReality.php`：节点过滤、`ss_node.server` 编解码、标准 URI、独立订阅、Mihomo 和 sing-box 节点，不新增模型或数据库表；
- `resources/.../vless_reality_fields.tpl`：管理员节点公开参数表单；
- `patches/sspanel-vless-reality.patch`：独立 `?vless=1`、第一方混合 `links`/`mihomo`、capabilities 与用户中心；`mu=2`/`mu=4` 不注入 VLESS；
- `patches/sspanel-vless-reality-admin.patch`：`sort=15` 管理、心跳和节点表单；
- `test/vless_reality.php`：无需真实数据库的字段、URI、Mihomo 和订阅测试。
- `merged/README.md`：最新版合并文件、无需修改文件、参数和部署边界清单。

只检查兼容性，不写文件：

```bash
sh vlesshappy/panel-overlay/apply.sh --check
```

`--apply` 目前会明确拒绝执行。项目负责人将另行提供实际 SSPanel 文件；收到文件和新授权后，才以这些文件为基线重新生成最终 patch。

当前脚本只做兼容性检查，不写父项目。2.0 不需要数据库迁移；私钥生成也不会由 overlay 自动执行。
