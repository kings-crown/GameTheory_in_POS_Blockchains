#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CACHE_DIR="${CACHE_DIR:-"$ROOT_DIR/.gocache"}"
GOPATH_DIR="${GOPATH_DIR:-"$ROOT_DIR/.gopath"}"
ARTIFACT_DIR="${ARTIFACT_DIR:-"$ROOT_DIR/artifacts"}"

DEFAULT_VARIANTS=("Vic_gen" "Random")
DEFAULT_DURATION=300
DEFAULT_CLIENTS=10
DEFAULT_BALANCE=1000
DEFAULT_BASE_COST=15
DEFAULT_ROUND=60
DEFAULT_INTER_DELAY=1

variants=()
duration="$DEFAULT_DURATION"
clients="$DEFAULT_CLIENTS"
balance="$DEFAULT_BALANCE"
base_cost="$DEFAULT_BASE_COST"
round_interval="$DEFAULT_ROUND"
inter_delay="$DEFAULT_INTER_DELAY"

function usage() {
	cat <<'EOS'
Usage: ./run_experiments.sh [options]

Options:
  --variant NAME      Run a specific variant (Vic_gen, Vick, Random, Random_gen). Can be repeated.
  --duration SECONDS  Length of each experiment run (default: 300).
  --clients N         Number of simulated validators (default: 10).
  --balance TOKENS    Starting balance for each validator (default: 1000).
  --base-cost COST    Base bid cost used by simulated validators (default: 15).
  --round SECONDS     Interval between bids from each validator (default: 60).
  --inter-delay SEC   Delay between BPM and bid submissions (default: 1).
  --help              Show this help message.

Environment overrides:
  CACHE_DIR, GOPATH_DIR, ARTIFACT_DIR can be set to customize working directories.

The script sequentially starts each selected server, launches the Go-based client
simulator, waits for completion, and archives logs and exported chain snapshots
under ARTIFACT_DIR.
EOS
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--variant)
			if [[ $# -lt 2 ]]; then
				echo "--variant requires a value" >&2
				exit 1
			fi
			variants+=("$2")
			shift 2
			;;
		--duration)
			duration="$2"
			shift 2
			;;
		--clients)
			clients="$2"
			shift 2
			;;
		--balance)
			balance="$2"
			shift 2
			;;
		--base-cost)
			base_cost="$2"
			shift 2
			;;
		--round)
			round_interval="$2"
			shift 2
			;;
		--inter-delay)
			inter_delay="$2"
			shift 2
			;;
		--help)
			usage
			exit 0
			;;
		*)
			echo "Unknown argument: $1" >&2
			usage >&2
			exit 1
			;;
	esac
done

if [[ ${#variants[@]} -eq 0 ]]; then
	variants=(${DEFAULT_VARIANTS[@]})
fi

mkdir -p "$CACHE_DIR" "$GOPATH_DIR" "$ARTIFACT_DIR"

SERVER_PID=""
CLIENT_PID=""

cleanup() {
	if [[ -n "$CLIENT_PID" ]] && kill -0 "$CLIENT_PID" 2>/dev/null; then
		kill "$CLIENT_PID" 2>/dev/null || true
	fi
	if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
		kill "$SERVER_PID" 2>/dev/null || true
	fi
}

trap cleanup EXIT INT TERM

parse_port() {
	local env_file="$1"
	if [[ ! -f "$env_file" ]]; then
		echo 8080
		return
	fi
	local line
	line=$(grep -E '^[[:space:]]*PORT' "$env_file" | tail -n 1 || true)
	line=${line#*=}
	line=$(echo "$line" | tr -d '"' | tr -d "'" | tr -d ' ')
	if [[ -z "$line" ]]; then
		echo 8080
	else
		echo "$line"
	fi
}

is_port_available() {
	local port="$1"
	python3 - <<'PY' "$port" >/dev/null 2>&1
import socket, sys
port = int(sys.argv[1])
s = socket.socket()
try:
    s.bind(("", port))
except OSError:
    sys.exit(1)
finally:
    s.close()
sys.exit(0)
PY
}

find_free_port() {
	python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("", 0))
port = s.getsockname()[1]
s.close()
print(port)
PY
}

wait_for_server() {
	local log_file="$1"
	local timeout="${2:-30}"
	for ((i = 0; i < timeout; i++)); do
		if grep -q "TCP Server Listening" "$log_file" 2>/dev/null; then
			return 0
		fi
		sleep 1
		if [[ -n "$SERVER_PID" ]] && ! kill -0 "$SERVER_PID" 2>/dev/null; then
			return 1
		fi
	done
	return 1
}

run_variant() {
	local variant="$1"
	local variant_dir="$ROOT_DIR/$variant"
	if [[ ! -d "$variant_dir" ]]; then
		echo "Variant directory not found: $variant" >&2
		return 1
	fi

	local env_file="$variant_dir/.env"
	local port
	port=$(parse_port "$env_file")

	if ! is_port_available "$port"; then
		echo "Port $port is busy; selecting an available port automatically." >&2
		port=$(find_free_port)
	fi
	local host="127.0.0.1"
	local timestamp
	timestamp=$(date +%Y%m%d-%H%M%S)

	local server_log="$ARTIFACT_DIR/${timestamp}_${variant}_server.log"
	local client_log="$ARTIFACT_DIR/${timestamp}_${variant}_clients.log"
	local chain_snapshot="$ARTIFACT_DIR/${timestamp}_${variant}_blockchain.txt"

	echo "=== Running $variant on $host:$port ==="
	echo "Server log:   $server_log"
	echo "Client log:   $client_log"

	: >"$server_log"
	: >"$client_log"

	(
		cd "$variant_dir"
		PORT="$port" GOCACHE="$CACHE_DIR" GOPATH="$GOPATH_DIR" go run .
	) &>"$server_log" &
	SERVER_PID=$!

	if ! wait_for_server "$server_log" 45; then
		echo "Failed to detect server startup for $variant" >&2
		return 1
	fi

	(
		cd "$ROOT_DIR"
		GOCACHE="$CACHE_DIR" GOPATH="$GOPATH_DIR" go run ./tools/client \
			--host "$host" \
			--port "$port" \
			--clients "$clients" \
			--balance "$balance" \
			--base-cost "$base_cost" \
			--round "$round_interval" \
			--inter-delay "$inter_delay" \
			--duration "$duration"
	) &>"$client_log" &
	CLIENT_PID=$!

	if ! wait "$CLIENT_PID"; then
		echo "Client simulation exited with error for $variant" >&2
	fi
	CLIENT_PID=""

	sleep 2

	if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" || true
	fi
	SERVER_PID=""

	if [[ -f "$variant_dir/blockchain.xlsx" ]]; then
		cp "$variant_dir/blockchain.xlsx" "$chain_snapshot"
		echo "Snapshot stored at $chain_snapshot"
	else
		echo "Warning: no blockchain export found for $variant" >&2
	fi

	echo "=== Completed $variant ==="
}

for variant in "${variants[@]}"; do
	if ! run_variant "$variant"; then
		echo "Experiment failed for $variant" >&2
		exit 1
	fi
done

echo "Artifacts available in $ARTIFACT_DIR"
