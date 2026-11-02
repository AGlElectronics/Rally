# Rally dev convenience script. Usage from a PowerShell:
#
#   .\scripts\dev.ps1 up        # build + start the full compose stack
#   .\scripts\dev.ps1 down      # stop and remove containers/volumes
#   .\scripts\dev.ps1 logs      # tail backend + frontend logs
#   .\scripts\dev.ps1 migrate   # re-run the migrate container (`cmd/migrate up`)
#   .\scripts\dev.ps1 psql      # open an interactive psql against the dev DB
#
# This is a thin wrapper around `docker compose -f infra/docker-compose.yml`.

param(
    [Parameter(Position = 0)]
    [string]$Command = "up"
)

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Definition
$repo = Resolve-Path (Join-Path $here "..")

Push-Location $repo
try {
    switch ($Command) {
        "up"       { docker compose -f infra/docker-compose.yml up --build }
        "down"     { docker compose -f infra/docker-compose.yml down -v }
        "logs"     { docker compose -f infra/docker-compose.yml logs -f backend frontend }
        "migrate"  { docker compose -f infra/docker-compose.yml run --rm migrate up }
        "status"   { docker compose -f infra/docker-compose.yml run --rm migrate status }
        "psql"     { docker compose -f infra/docker-compose.yml exec postgres psql -U rally -d rally }
        default    { Write-Error "Unknown command: $Command (use: up|down|logs|migrate|status|psql)" }
    }
}
finally {
    Pop-Location
}
