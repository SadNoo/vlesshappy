# SSPanel overlay

本目录是可审查的面板集成成品，当前没有写入父项目。

包含：

- `app/Services/VlessReality.php`：节点过滤、标准 URI、独立订阅和 Mihomo 节点；
- `app/Models/VlessRealityNodeConfig.php`：公开 REALITY 配置模型；
- `resources/.../vless_reality_fields.tpl`：管理员节点公开参数表单；
- `patches/sspanel-vless-reality.patch`：独立 `?vless=1`、第一方混合 `links`/`mihomo`、capabilities 与用户中心；`mu=2`/`mu=4` 不注入 VLESS；
- `patches/sspanel-vless-reality-admin.patch`：`sort=15` 管理、心跳和节点表单；
- `test/vless_reality.php`：无需真实数据库的字段、URI、Mihomo 和订阅测试。

只检查兼容性，不写文件：

```bash
sh vlesshappy/panel-overlay/apply.sh --check
```

`--apply` 目前会明确拒绝执行。项目负责人将另行提供实际 SSPanel 文件；收到文件和新授权后，才以这些文件为基线重新生成最终 patch。随后单独备份数据库并人工应用：

```text
vlesshappy/migrations/001_vless_reality.up.sql
```

当前脚本只做兼容性检查，不写父项目。数据库迁移和私钥生成不会由 overlay 自动执行。
