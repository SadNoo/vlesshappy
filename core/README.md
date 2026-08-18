# Xray-core 固定源与最小补丁

- 上游：`github.com/xtls/xray-core v1.260327.0`
- Go module 校验：`h1:g4TzxMwyPrxslZh6uD+FiG3lXKTrnNO+b4ky2OhogHE=`
- 上游许可证：MPL-2.0；完整许可证随 `vendor/github.com/xtls/xray-core/LICENSE` 保留。
- 构建输入：仓库内 `vendor/`，构建时不在线替换上游源码。

`patches/0001-vlesshappy-session-control.patch` 是唯一 Xray 修改。它增加握手和逻辑会话 hook、逻辑会话释放/XUDP 来源迁移回调，并在受控流量上包裹限速等待；没有修改 VLESS、REALITY、Vision、TLS、XUDP 编解码或密码学算法。

重新生成 vendor 时必须在干净工作树人工执行：

```bash
go mod vendor
patch -d vendor/github.com/xtls/xray-core -p1 < core/patches/0001-vlesshappy-session-control.patch
gofmt -w vendor/github.com/xtls/xray-core/common/sessioncontrol \
  vendor/github.com/xtls/xray-core/app/dispatcher/sessioncontrol.go \
  vendor/github.com/xtls/xray-core/proxy/vless/inbound/sessioncontrol.go
```

随后必须审查补丁、运行全部门禁并更新上游校验；不得对新版本盲目使用 fuzz patch。
