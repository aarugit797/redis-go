#!/usr/bin/env bash
# Phase 8 benchmark: the same server, same network code, two execution models.
#   locked: every connection goroutine executes under one global mutex
#   single: one goroutine executes everything, connections only do I/O
#
# Usage: ./bench.sh            (needs redis-benchmark: sudo apt install redis-tools)
# Env:   RUNS=3 PORT=6390 CLIENTS=50
set -euo pipefail

RUNS=${RUNS:-3}
PORT=${PORT:-6390}
CLIENTS=${CLIENTS:-50}
cd "$(dirname "$0")"
go build -o redis-go .

# name | redis-benchmark arguments
SCENARIOS=(
  "no pipeline, random keys|-n 200000 -r 1000000"
  "pipeline 16, random keys|-n 1000000 -P 16 -r 1000000"
  "pipeline 16, one hot key|-n 1000000 -P 16"
)

median() { sort -n | awk '{a[NR]=$1} END {print a[int((NR+1)/2)]}'; }

# run_mode MODE SCENARIO_ARGS TEST -> median requests/sec over RUNS
run_mode() {
  local mode=$1 args=$2 test=$3 dir pid
  dir=$(mktemp -d)
  ./redis-go -port "$PORT" -mode "$mode" -save "" -dir "$dir" >/dev/null 2>&1 &
  pid=$!
  for _ in $(seq 50); do redis-cli -p "$PORT" ping >/dev/null 2>&1 && break; sleep 0.1; done
  for _ in $(seq "$RUNS"); do
    # shellcheck disable=SC2086
    redis-benchmark -p "$PORT" -c "$CLIENTS" -t "$test" $args --csv 2>/dev/null \
      | awk -F'"' 'NR==2 {print $4}'
  done | median
  kill "$pid"; wait "$pid" 2>/dev/null || true
  rm -rf "$dir"
}

echo "machine: $(nproc) cores, $(go version | awk '{print $3}'), $CLIENTS clients, median of $RUNS runs"
printf '\n%-28s %-4s %14s %14s %10s\n' "scenario" "cmd" "locked rps" "single rps" "single vs locked"
for sc in "${SCENARIOS[@]}"; do
  name=${sc%%|*}; args=${sc#*|}
  for test in set get; do
    locked=$(run_mode locked "$args" "$test")
    single=$(run_mode single "$args" "$test")
    diff=$(awk -v l="$locked" -v s="$single" 'BEGIN {printf "%+.1f%%", (s-l)/l*100}')
    printf '%-28s %-4s %14.0f %14.0f %10s\n' "$name" "$test" "$locked" "$single" "$diff"
  done
done
