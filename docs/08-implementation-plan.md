# 08. 2.0 实施计划

## 已批准目标

把 1.0 的专用配置表和批次幂等设计改为与现有 V2 后端相同的数据库模式，同时保留 VLESS REALITY 数据面、会话安全、`sort=15` 和独立 `?vless=1`。

## 实施阶段

1. `ss_node.server`：实现 Go/PHP 同格式严格解析，固定 `chrome + Vision + RAW/TCP`。
2. 数据库：查询只读 `ss_node` 和 `user`，删除两张专用表和全部迁移依赖。
3. 计费：用单事务更新现有用户、日志、节点表；失败恢复进程内计数。
4. 生命周期：删除 outbox、重放、容量门禁和故障代理，保留授权陈旧 fail-closed。
5. 面板：管理员表单编码 `ss_node.server`，订阅从同一字段解析；不新增 Model。
6. 订阅：继续保持 `?vless=1` 独立，`mu=2`/`mu=4` 不加入；显式支持的 Mihomo、sing-box 和第一方混合入口加入。
7. 文档：明确零迁移、SSPanel 文件清单、可接受计费误差、升级和回滚。
8. 验证与发布：Go/PHP/overlay/Docker 测试、三层敏感信息扫描、GitHub 草稿 PR 和 `sadno/vle:2.0`。

## 完成条件

- 后端源码中不存在专用 VLESS 表查询；
- `migrations/` 只声明“不需要迁移”；
- `ss_node.server` 的 Go/PHP 固定向量一致；
- MySQL 集成证明只创建并使用父项目既有表；
- 流量事务失败不产生部分入账；
- 面板清单可明确区分完整文件、补丁文件和无需修改文件；
- 所有变更都在 `vlesshappy/`，敏感信息扫描无未解释命中；
- GitHub 和 Docker Hub 发布物都能回查到同一源码提交。
