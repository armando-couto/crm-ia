#!/usr/bin/env bash
# (dev) Cria o banco/role do painel no PostgreSQL que JÁ roda na máquina
# (padrão 127.0.0.1:5432, postgres/postgres). Idempotente.
set -euo pipefail
cd "$(dirname "$0")/../.."
[ -f .env ] || { echo "sem .env — rode make env"; exit 1; }
valor() { { grep -E "^$1=" .env || true; } | head -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//'; }
HOST="${DEV_PGHOST_LOCAL:-127.0.0.1}"; PORT="${DEV_PGPORT:-5432}"
ADMIN="${PGADMIN_USER:-$(valor PGADMIN_USER)}"; ADMIN="${ADMIN:-postgres}"
ADMIN_SENHA="${PGADMIN_PASSWORD:-$(valor PGADMIN_PASSWORD)}"; ADMIN_SENHA="${ADMIN_SENHA:-postgres}"
PAINEL_SENHA="$(valor PAINEL_DB_PASSWORD)"; PAINEL_SENHA="${PAINEL_SENHA:-dev}"
if command -v psql >/dev/null 2>&1; then
  PSQL=(env PGPASSWORD="$ADMIN_SENHA" psql -h "$HOST" -p "$PORT" -U "$ADMIN" -v ON_ERROR_STOP=1 -qAt)
else
  PSQL=(docker run --rm -i -e PGPASSWORD="$ADMIN_SENHA" --add-host=host.docker.internal:host-gateway postgres:16-alpine psql -h host.docker.internal -p "$PORT" -U "$ADMIN" -v ON_ERROR_STOP=1 -qAt)
fi
if ! "${PSQL[@]}" -d postgres -c "select 1" >/dev/null 2>&1 && [ "$ADMIN_SENHA" != "postgres" ]; then
  ADMIN_SENHA=postgres
  for i in "${!PSQL[@]}"; do case "${PSQL[$i]}" in PGPASSWORD=*) PSQL[$i]="PGPASSWORD=postgres";; esac; done
fi
"${PSQL[@]}" -d postgres -c "select 1" >/dev/null || { echo "não consegui falar com o PostgreSQL em $HOST:$PORT como $ADMIN"; exit 1; }
sql="$(printf "DO \$\$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='crmia_painel') THEN CREATE ROLE crmia_painel LOGIN PASSWORD '%s'; ELSE ALTER ROLE crmia_painel PASSWORD '%s'; END IF; END \$\$;" "$PAINEL_SENHA" "$PAINEL_SENHA")"
"${PSQL[@]}" -d postgres -c "$sql"
if [ "$("${PSQL[@]}" -d postgres -c "select 1 from pg_database where datname='crmia_painel'")" != "1" ]; then
  "${PSQL[@]}" -d postgres -c "CREATE DATABASE crmia_painel OWNER crmia_painel ENCODING 'UTF8'"; echo "  banco crmia_painel criado"
else
  "${PSQL[@]}" -d postgres -c "ALTER DATABASE crmia_painel OWNER TO crmia_painel"
fi
"${PSQL[@]}" -d crmia_painel -c "CREATE EXTENSION IF NOT EXISTS pgcrypto" >/dev/null
echo "→ banco de dev crmia_painel pronto em $HOST:$PORT"
