# Third-party notices

`vlesshappy` embeds [XTLS/Xray-core](https://github.com/XTLS/Xray-core) at
`v1.260327.0` (`d2758a023cd7f4174a5a5fa4ff66e487d4342ba0`). Xray-core is
licensed under Mozilla Public License 2.0. The fixed source is vendored for a
reproducible patched build. `core/patches/0001-vlesshappy-session-control.patch`
changes only session hooks and release wiring; protocol and cryptographic code
is unchanged. The vendored Xray license and source are shipped with this tree.

The MySQL connection uses [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql)
`v1.10.0`, licensed under the Mozilla Public License 2.0.

Transitive dependency versions and checksums are fixed by `go.mod`, `go.sum`
and `vendor/modules.txt`; dependency license files remain in `vendor/`.

The 2.2 container includes [Caddy](https://github.com/caddyserver/caddy)
`v2.11.4`, copied from the pinned Docker Official Image
`caddy:2.11.4-alpine@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648`.
Caddy is licensed under Apache License 2.0 and is used only for the local
managed HTTPS fallback and ACME HTTP-01 certificate lifecycle.
