#!/usr/bin/env bash
# Adds a Brave Search MCP server to the 5 research/strategy agents, on top of
# their existing memory + sequential-thinking + fetch servers.
#
# You supply the key — it never leaves your machine except into the agent's
# MCP config on your own box. Get a free key at https://brave.com/search/api
#
# Usage:
#   export BRAVE_API_KEY="your-brave-search-key"
#   bash infra/ec2-orynx/add-search-mcp.sh
set -euo pipefail

: "${BRAVE_API_KEY:?Set BRAVE_API_KEY first: export BRAVE_API_KEY=\"...\"}"

PROFILE="ec2"
ALIST="$(multica --profile "$PROFILE" agent list 2>/dev/null)"
UUID='[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'

for name in "Market Research" "Investor Analyst" "Compliance" "Scientist" "Growth & Marketing"; do
  id="$(echo "$ALIST" | grep -F "$name" | grep -oE "^$UUID" | head -1)"
  [ -z "$id" ] && { echo "SKIP $name (not found)"; continue; }
  slug="$(echo "$name" | tr '[:upper:]' '[:lower:]' | tr ' ' '-' | tr -cd 'a-z0-9-')"

  printf '{"mcpServers":{
    "memory":{"command":"npx","args":["-y","@modelcontextprotocol/server-memory"],"env":{"MEMORY_FILE_PATH":"/home/ubuntu/orynx-memory/%s.json"}},
    "sequential-thinking":{"command":"npx","args":["-y","@modelcontextprotocol/server-sequential-thinking"]},
    "fetch":{"command":"/home/ubuntu/.local/bin/uvx","args":["mcp-server-fetch"]},
    "brave-search":{"command":"npx","args":["-y","@modelcontextprotocol/server-brave-search"],"env":{"BRAVE_API_KEY":"%s"}}
  }}' "$slug" "$BRAVE_API_KEY" \
    | multica --profile "$PROFILE" agent update "$id" --mcp-config-stdin >/dev/null \
    && echo "OK  $name  (+ brave-search)"
done
echo "Done. Research agents can now do open web search."
