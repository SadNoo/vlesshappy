#!/bin/sh
set -eu

[ "$#" -eq 2 ] || { echo "usage: fault-gate.sh <absolute-config> <absolute-output-dir>" >&2; exit 2; }
config="$1"
output="$2"
case "$config:$output" in /*:/*) ;; *) echo "config and output must be absolute" >&2; exit 2 ;; esac
[ -f "$config" ] || { echo "config file is missing" >&2; exit 1; }
[ ! -e "$output" ] || { echo "refusing to overwrite $output" >&2; exit 1; }
mkdir -p "$output"

go test -mod=vendor ./internal/database ./internal/lifecycle -count=1 > "$output/database.log" 2>&1
echo "V2-compatible accounting intentionally has no durable outbox or batch deduplication. Test a temporary database outage, recovery, and container restart, then record the accepted traffic variance." > "$output/NEXT.txt"
echo "Do not modify the supplied secret-bearing config in place: $config" >> "$output/NEXT.txt"
