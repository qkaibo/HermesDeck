#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Hermes Agent venv
HERMES_AGENT_DIR="$HOME/.hermes/hermes-agent"
if [ -f "$HERMES_AGENT_DIR/venv/bin/python3" ]; then
    PYTHON_CMD="$HERMES_AGENT_DIR/venv/bin/python3"
elif [ -f "$HERMES_AGENT_DIR/venv/bin/python" ]; then
    PYTHON_CMD="$HERMES_AGENT_DIR/venv/bin/python"
else
    PYTHON_CMD="python3"
fi
echo "Using Python: $($PYTHON_CMD --version 2>&1)"

# Node >= 22
if [ -f "$HOME/.n/bin/node" ]; then
    NODE_CMD="$HOME/.n/bin/node"
else
    NODE_CMD="node"
fi
echo "Using Node: $($NODE_CMD --version 2>&1)"

# Cleanup
PID_FILE="$PROJECT_DIR/.pids"
if [ -f "$PID_FILE" ]; then
    while read pid; do kill "$pid" 2>/dev/null || true; done < "$PID_FILE"
    rm -f "$PID_FILE"
fi

echo "=== Starting HermesDeck ==="

# 1. HTTP Bridge (port 29553) — OpenAI /v1/chat/completions → Hermes AIAgent
cd "$PROJECT_DIR/src/python"
PILOT_HOME="$HOME/.hermesdeck" $PYTHON_CMD -m hermesdeck_sidecar.http_bridge &
PY_PID=$!
echo $PY_PID >> "$PID_FILE"
echo "HTTP Bridge PID=$PY_PID (port 29553)"

sleep 5

# 2. Gateway (port 28788) — pure PilotDeck, no patches
cd "$PROJECT_DIR"
$NODE_CMD src/hermesdeck-gateway.mjs &
GW_PID=$!
echo $GW_PID >> "$PID_FILE"
echo "Gateway PID=$GW_PID (port 28788)"

sleep 3

# 3. UI Express Server (port 28789)
cd "$PROJECT_DIR/ui"
PILOT_HOME="$HOME/.hermesdeck" SERVER_PORT=28789 PILOTDECK_GATEWAY_URL=ws://127.0.0.1:28788/ws $NODE_CMD --import tsx server/index.js &
UI_PID=$!
echo $UI_PID >> "$PID_FILE"
echo "UI Server PID=$UI_PID (port 28789)"

echo ""
echo "✓ HermesDeck 已启动: http://127.0.0.1:28789"
echo "  停止: kill \$(cat $PID_FILE) && rm -f $PID_FILE"

wait -n
