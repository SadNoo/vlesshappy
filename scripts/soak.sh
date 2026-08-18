#!/bin/sh
set -eu

usage() {
    echo "usage: soak.sh --hours <72|168> --output <absolute-dir> --pid <vlesshappy-pid> -- <probe-command> [args...]" >&2
    exit 2
}

hours=""
output=""
service_pid=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        --hours) [ "$#" -ge 2 ] || usage; hours="$2"; shift 2 ;;
        --output) [ "$#" -ge 2 ] || usage; output="$2"; shift 2 ;;
        --pid) [ "$#" -ge 2 ] || usage; service_pid="$2"; shift 2 ;;
        --) shift; break ;;
        *) usage ;;
    esac
done

[ "$hours" = "72" ] || [ "$hours" = "168" ] || usage
case "$output" in /*) ;; *) usage ;; esac
case "$service_pid" in ''|*[!0-9]*) usage ;; esac
[ "$#" -gt 0 ] || usage

mkdir -p "$output"
started=$(date +%s)
deadline=$((started + hours * 3600))
metrics="$output/resources.csv"
results="$output/probes.log"
[ ! -e "$metrics" ] || { echo "refusing to overwrite $metrics" >&2; exit 1; }
[ ! -e "$results" ] || { echo "refusing to overwrite $results" >&2; exit 1; }
echo "unix_time,elapsed_seconds,pid,rss_kib,vsz_kib,fd_count,probe_status" > "$metrics"

while [ "$(date +%s)" -lt "$deadline" ]; do
    now=$(date +%s)
    if ! kill -0 "$service_pid" 2>/dev/null; then
        echo "$now,$((now-started)),$service_pid,0,0,0,service_exited" >> "$metrics"
        exit 1
    fi
    rss=$(ps -o rss= -p "$service_pid" | tr -d ' ')
    vsz=$(ps -o vsz= -p "$service_pid" | tr -d ' ')
    if [ -d "/proc/$service_pid/fd" ]; then
        fds=$(find "/proc/$service_pid/fd" -mindepth 1 -maxdepth 1 2>/dev/null | wc -l | tr -d ' ')
    else
        fds=0
    fi
    if "$@" >> "$results" 2>&1; then status=ok; else status=failed; fi
    echo "$now,$((now-started)),$service_pid,${rss:-0},${vsz:-0},$fds,$status" >> "$metrics"
    [ "$status" = "ok" ] || exit 1
    sleep 60
done
