#!/bin/sh
set -eu

[ "$#" -eq 3 ] || { echo "usage: fault-gate.sh <absolute-config> <mysql-upstream> <absolute-output-dir>" >&2; exit 2; }
config="$1"
upstream="$2"
output="$3"
case "$config:$output" in /*:/*) ;; *) echo "config and output must be absolute" >&2; exit 2 ;; esac
[ -f "$config" ] || { echo "config file is missing" >&2; exit 1; }
[ ! -e "$output" ] || { echo "refusing to overwrite $output" >&2; exit 1; }
mkdir -p "$output"

go test -mod=vendor ./internal/accounting -run 'TestOutboxFaultInjectionStages|TestOutboxRejectsTamperingAndIncompleteWrite' -count=1 > "$output/outbox.log" 2>&1
go build -mod=vendor -o "$output/mysql-faultproxy" ./cmd/vlesshappy-mysql-faultproxy
echo "outbox fault cases prepared; start mysql-faultproxy against $upstream, point a temporary config copy at 127.0.0.1:13306, and execute one report/replay cycle" > "$output/NEXT.txt"
echo "Do not modify the supplied secret-bearing config in place: $config" >> "$output/NEXT.txt"
