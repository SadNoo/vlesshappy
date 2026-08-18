# 16. SSPanel 延后集成合同

项目负责人确认：后端完成后会另行提交实际 SSPanel 文件。本轮不得修改父项目，也不得把现有 overlay 应用到父项目。因此：

- `panel-overlay/` 只是接口草案和未来 diff 参考。
- `panel-overlay/apply.sh --apply` 固定失败；`--check` 也只是对当前父项目的只读兼容性检查。
- 收到实际文件后，只修改项目负责人提交且明确授权的文件，先处理现有 SS2022/其他改动冲突，再生成新的最终 patch。
- 数据库 migration 不自动执行。

冻结的订阅语义：

1. `/link/<token>?vless=1` 是唯一 VLESS 独立通用订阅，严格只接受单个值 `1`。
2. `?mu=2` 不加入 VLESS。
3. `?mu=4` 不加入 VLESS。
4. “混合订阅”特指第一方客户端的 `format=links` 和 `format=mihomo` 聚合，它们显式加入 VLESS。
5. 独立 VLESS 失败不影响混合订阅中的其他协议；单坏节点跳过，不输出私钥或内部信息。

最终集成至少需要项目负责人提供 LinkController、第一方 ClientConfig/API、Node/User 控制器、用户模板和管理员节点页面的实际版本。收到前不推测文件结构、不写父项目。
