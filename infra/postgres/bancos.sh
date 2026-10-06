#!/bin/sh
# Cria (ou sincroniza) o banco do painel, seu dono e a extensão pgcrypto, a
# partir do ambiente já exportado pelo entrypoint. Idempotente.
set -eu
i=0
until pg_isready -h 127.0.0.1 -p 5432 -q; do
  i=$((i + 1)); [ "$i" -gt 300 ] && { echo "crmia: PostgreSQL não respondeu em 5 min" >&2; exit 1; }
  sleep 1
done
export PGPASSWORD="$POSTGRES_PASSWORD"
psql_admin() { psql -h 127.0.0.1 -p 5432 -U "$POSTGRES_USER" -v ON_ERROR_STOP=1 -qAt "$@"; }
preparar() {
  banco="$1"; usuario="$2"; senha="$3"
  [ -n "$senha" ] || { echo "crmia:   $banco sem senha (PAINEL_DB_PASSWORD), pulando"; return; }
  senha_sql=$(printf '%s' "$senha" | sed "s/'/''/g")
  psql_admin -d postgres -c "DO \$\$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='${usuario}') THEN CREATE ROLE ${usuario} LOGIN PASSWORD '${senha_sql}'; ELSE ALTER ROLE ${usuario} LOGIN PASSWORD '${senha_sql}'; END IF; END \$\$;"
  if [ "$(psql_admin -d postgres -c "SELECT 1 FROM pg_database WHERE datname='${banco}'")" != "1" ]; then
    psql_admin -d postgres -c "CREATE DATABASE ${banco} OWNER ${usuario} ENCODING 'UTF8'"
  else
    psql_admin -d postgres -c "ALTER DATABASE ${banco} OWNER TO ${usuario}"
  fi
  psql_admin -d postgres -c "REVOKE CONNECT ON DATABASE ${banco} FROM PUBLIC"
  psql_admin -d postgres -c "GRANT CONNECT ON DATABASE ${banco} TO ${usuario}"
  psql_admin -d "$banco" -c "CREATE EXTENSION IF NOT EXISTS pgcrypto"
  echo "crmia:   ok $banco (dono $usuario)"
}
echo "crmia: preparando o banco do painel"
preparar "${PAINEL_DB_NAME:-crmia_painel}" "${PAINEL_DB_USER:-crmia_painel}" "${PAINEL_DB_PASSWORD:-}"
echo "crmia: banco do painel pronto"
