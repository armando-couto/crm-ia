#!/bin/sh
# Se houver um /.env.production montado (segredo do Swarm), exporta-o sem
# sobrescrever o que já veio no ambiente; traduz PGADMIN_* para o que a imagem
# oficial espera e dispara, em segundo plano, a preparação do banco do painel —
# a CADA subida, não só na criação do volume.
set -eu
if [ -f /.env.production ]; then
  while IFS= read -r linha || [ -n "$linha" ]; do
    linha=$(printf '%s' "$linha" | tr -d '\r')
    case "$linha" in ''|'#'*) continue ;; esac
    case "$linha" in *=*) ;; *) continue ;; esac
    chave=${linha%%=*}; valor=${linha#*=}
    chave=$(printf '%s' "$chave" | sed 's/^export //; s/^ *//; s/ *$//')
    printf '%s' "$chave" | grep -Eq '^[A-Za-z_][A-Za-z0-9_]*$' || continue
    valor=$(printf '%s' "$valor" | sed 's/^ *//; s/ *$//')
    case "$valor" in \"*\") valor=${valor#\"}; valor=${valor%\"} ;; \'*\') valor=${valor#\'}; valor=${valor%\'} ;; esac
    [ -n "$valor" ] || continue
    eval "atual=\${$chave:-}"
    [ -n "$atual" ] && continue
    export "$chave=$valor"
  done < /.env.production
fi
: "${POSTGRES_USER:=${PGADMIN_USER:-postgres}}"
: "${POSTGRES_PASSWORD:=${PGADMIN_PASSWORD:-}}"
[ -n "$POSTGRES_PASSWORD" ] || { echo "PGADMIN_PASSWORD (ou POSTGRES_PASSWORD) não definida" >&2; exit 1; }
export POSTGRES_USER POSTGRES_PASSWORD
if [ "${1:-}" = "postgres" ]; then crmia-bancos.sh & fi
exec docker-entrypoint.sh "$@"
