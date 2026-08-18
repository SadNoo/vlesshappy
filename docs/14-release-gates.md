# 14. 发布门禁

`scripts/release-gate.sh <version> <absolute-new-output-dir>` 是正式发布入口。它会拒绝覆盖结果目录，并要求本机已有 `go`、`syft`、`trivy`、`cosign`，随后执行测试、race、vet、Linux amd64/arm64 构建、SPDX SBOM、HIGH/CRITICAL 漏洞否决、SHA-256 和 Sigstore bundle。

在执行脚本前还必须完成：

1. `docs/13` 的客户端、故障和长稳结果全部通过。
2. 最终 SSPanel 文件集成和回归通过。
3. 项目负责人选择 `vlesshappy` 自身许可证；Xray 修改文件继续受 MPL-2.0 约束。
4. 固定最终镜像 digest，检查非 root、只读、无 shell、无 secret。
5. 人工审查 SBOM、第三方许可证、漏洞例外和全部结果。
6. 项目负责人分别授权 tag、签名、镜像推送和 GitHub Release。

脚本生成签名不等于自动发布，不包含 `git push`、Docker push、数据库迁移或部署命令。许可证未决定时不得发布源码包或正式镜像。
