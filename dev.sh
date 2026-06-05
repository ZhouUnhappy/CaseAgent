#!/bin/bash

# CaseAgent frontend & backend startup manager
# Usage:
#   ./dev.sh start [--vite_port <port>] [--go_port <port>] [--log <dir>]
#   ./dev.sh stop
#   ./dev.sh restart [--vite_port <port>] [--go_port <port>] [--log <dir>]

set -euo pipefail

COMMAND=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="$SCRIPT_DIR/.dev/logs"
VITE_PORT=40002
GO_PORT=40003

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_usage() {
    cat <<EOF
Usage:
  $0 start [--vite_port <port>] [--go_port <port>] [--log <dir>]
  $0 stop
  $0 restart [--vite_port <port>] [--go_port <port>] [--log <dir>]
EOF
}

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

check_port() {
    local port=$1
    local name=$2

    if lsof -Pi :"$port" -sTCP:LISTEN -t >/dev/null 2>&1; then
        print_error "$name port $port is already in use"
        echo "Run 'lsof -i :$port' to inspect the process."
        exit 1
    fi
}

kill_by_port() {
    local port=$1
    local name=$2
    local pids

    pids=$(lsof -Pi :"$port" -sTCP:LISTEN -t 2>/dev/null || true)
    if [ -n "$pids" ]; then
        print_info "Stopping $name on port $port (PIDs: $pids)"
        echo "$pids" | xargs kill 2>/dev/null || true
        sleep 1
    else
        print_warn "No $name process is listening on port $port"
    fi
}

start_dev() {
    mkdir -p "$LOG_DIR"

    check_port "$GO_PORT" "Backend"
    check_port "$VITE_PORT" "Frontend"

    local backend_log="$LOG_DIR/backend.log"
    local frontend_log="$LOG_DIR/frontend.log"

    print_info "Starting CaseAgent..."
    print_info "Backend:  http://localhost:$GO_PORT"
    print_info "Frontend: http://localhost:$VITE_PORT"
    print_info "Logs:     $LOG_DIR"

    print_info "Starting backend..."
    (
        cd "$SCRIPT_DIR/backend"
        nohup env SERVER_PORT="$GO_PORT" go run cmd/server/main.go >"$backend_log" 2>&1 &
    ) &
    print_info "Backend log: $backend_log"

    sleep 1

    print_info "Starting frontend..."
    (
        cd "$SCRIPT_DIR/frontend"
        nohup npm run dev -- --host 127.0.0.1 --port "$VITE_PORT" >"$frontend_log" 2>&1 &
    ) &
    print_info "Frontend log: $frontend_log"

    print_info "CaseAgent started in background. Use '$0 stop' to stop it."
}

stop_dev() {
    print_info "Stopping CaseAgent..."
    kill_by_port "$GO_PORT" "Backend"
    kill_by_port "$VITE_PORT" "Frontend"
    print_info "All processes stopped"
}

restart_dev() {
    stop_dev
    print_info "Waiting for ports to be released..."
    sleep 2
    start_dev
}

while [ $# -gt 0 ]; do
    case "$1" in
        start)
            COMMAND="start"
            shift
            ;;
        stop)
            COMMAND="stop"
            shift
            ;;
        restart)
            COMMAND="restart"
            shift
            ;;
        --go_port)
            GO_PORT="$2"
            shift 2
            ;;
        --vite_port)
            VITE_PORT="$2"
            shift 2
            ;;
        --log)
            LOG_DIR="$2"
            shift 2
            ;;
        *)
            print_error "Unknown argument: $1"
            print_usage
            exit 1
            ;;
    esac
done

case "$COMMAND" in
    start)
        start_dev
        ;;
    stop)
        stop_dev
        ;;
    restart)
        restart_dev
        ;;
    "")
        print_usage
        ;;
    *)
        print_error "Unknown command: $COMMAND"
        print_usage
        exit 1
        ;;
esac
