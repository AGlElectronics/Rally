#!/usr/bin/env bash
# Rally dev convenience script. Same commands as scripts/dev.ps1.
set -euo pipefail

cmd="${1:-up}"
cd "$(dirname "$0")/.."

case "$cmd" in
  up)       docker compose -f infra/docker-compose.yml up --build ;;
  down)     docker compose -f infra/docker-compose.yml down -v ;;
  logs)     docker compose -f infra/docker-compose.yml logs -f backend frontend ;;
  migrate)  docker compose -f infra/docker-compose.yml run --rm migrate up ;;
  status)   docker compose -f infra/docker-compose.yml run --rm migrate status ;;
  psql)     docker compose -f infra/docker-compose.yml exec postgres psql -U rally -d rally ;;
  *)        echo "Unknown command: $cmd (use: up|down|logs|migrate|status|psql)" ; exit 2 ;;
esac
