#!/usr/bin/env bash
# Backup de todos os bancos: clientes (pelo provisionador) e painel.
set -euo pipefail
cd "$(dirname "$0")/../.."
[ -f .env ] || { echo "sem .env"; exit 1; }
TOKEN="$(grep -E '^PROVISIONER_TOKEN=' .env | cut -d= -f2-)"
DESTINO="${DESTINO:-./backups}"; mkdir -p "$DESTINO"
echo "→ clientes"
for slug in $(docker exec crmia-provisioner wget -qO- --header="Authorization: Bearer $TOKEN" http://127.0.0.1:7898/tenants | python3 -c "import sys,json;print(' '.join(t['slug'] for t in json.load(sys.stdin)))"); do
  echo "  $slug"; docker exec crmia-provisioner wget -qO- --method=POST --header="Authorization: Bearer $TOKEN" --header="Content-Type: application/json" --body-data='{}' "http://127.0.0.1:7898/tenants/$slug/backup" >/dev/null || echo "  falhou: $slug"
done
echo "→ painel"
docker exec crmia-painel true 2>/dev/null && docker run --rm --network crmia_interna -e PGPASSWORD="$(grep -E '^PAINEL_DB_PASSWORD=' .env | cut -d= -f2-)" postgres:16-alpine \
  pg_dump -h "${PGHOST:-postgres}" -U "$(grep -E '^PAINEL_DB_USER=' .env | cut -d= -f2-)" "$(grep -E '^PAINEL_DB_NAME=' .env | cut -d= -f2-)" | gzip > "$DESTINO/painel-$(date +%Y%m%d-%H%M%S).sql.gz"
echo "✓ backups em $DESTINO e no volume crmia_backups"
