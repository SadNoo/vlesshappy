# 17. Docker 镜像 `sadno/vle:1.0`

交付日期：2026-08-19。

## 已完成

- 使用仓库内 `Dockerfile` 和固定 `vendor/` 离线依赖完成编译；
- 修正 `vendor/modules.txt`，声明补丁新增的 `common/sessioncontrol` 包；
- 推送 OCI 多架构镜像 `docker.io/sadno/vle:1.0`；
- 远端索引摘要：`sha256:f857810fe87bb9ebd4f016f4d2aa55d9df56906149919aff0b1c42b57a760ff9`；
- 目标架构：`linux/amd64`、`linux/arm64`；未创建 `latest` 标签；
- 运行层为非 root、无 shell 的 `scratch`，构建上下文未复制普通配置、数据库密码或 REALITY 私钥。

## Debian 兼容边界

镜像运行层不是 Debian 用户态，而是静态 Linux 二进制，因此不需要为 Debian 11、12、13 分别构建。设计目标是运行在装有兼容容器运行时的 Debian 11 及以上宿主机。本次只完成跨架构编译和远端清单核验，尚未在真实 Debian 11 主机执行运行时验收。

## 除 SSPanel 外仍需完成

1. 运行单元测试、race 和 vet，修复后重新生成可追溯镜像。
2. 在临时 MySQL 环境执行 migration、授权同步、流量批次幂等和崩溃恢复验收。
3. 执行 COMMIT 响应丢失、outbox 写入/fsync/rename 失败、数据库中断和授权陈旧等故障门禁。
4. 用固定版本的 v2rayN、v2rayNG、Mihomo 等真实客户端验证 REALITY、Vision、TCP、Mux 和 XUDP。
5. 在 Debian 11 及以上的 amd64/arm64 实机验证启动、健康检查、只读文件系统、非 root、重启恢复和资源限制。
6. 完成 72 小时预发布及 168 小时生产参数长稳，核对内存、CPU、文件描述符、会话与流量账单。
7. 确定项目自身许可证，生成并人工审查 SPDX SBOM、第三方许可证和 HIGH/CRITICAL 漏洞结果。
8. 固定最终版本与镜像摘要，生成校验和和 Sigstore 签名；经单独授权后发布正式 tag、Release 并部署，同时完成回滚演练。

在以上门禁通过前，`0.2.0-rc.1` 和 `sadno/vle:1.0` 都应视为候选交付物，不标记为生产正式版。
