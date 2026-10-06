#!/usr/bin/env bash
# Gera (ou completa) um .env a partir do .env.example, sorteando os segredos.
#   bash infra/scripts/gerar-env.sh                      # cria .env (dev)
#   bash infra/scripts/gerar-env.sh .env.production      # cria o de produção
#   bash infra/scripts/gerar-env.sh .env.production --completar
set -euo pipefail
cd "$(dirname "$0")/../.."
ARQUIVO="${1:-.env}"; MODO="${2:-}"
if [ -f "$ARQUIVO" ] && [ "$MODO" != "--completar" ]; then
  echo "já existe $ARQUIVO — use: bash infra/scripts/gerar-env.sh $ARQUIVO --completar"; exit 1
fi
if [ ! -f "$ARQUIVO" ]; then cp .env.example "$ARQUIVO"; else
  faltantes=$(grep -oE '^[A-Z_]+=' .env.example | tr -d '=' | while read -r c; do grep -qE "^$c=" "$ARQUIVO" || echo "$c"; done)
  if [ -n "$faltantes" ]; then
    { echo ""; echo "# -- acrescentado por gerar-env.sh --completar --------------------------------"; } >> "$ARQUIVO"
    for c in $faltantes; do grep -E "^$c=" .env.example >> "$ARQUIVO"; done
    echo "→ chaves novas: $(echo "$faltantes" | tr '\n' ' ')"
  fi
fi
gerar_hex() { openssl rand -hex "$1"; }
gerar_senha() { openssl rand -base64 24 | tr -d '/+=' | cut -c1-"$1"; }
substituir() {
  local chave="$1" valor="$2" atual
  grep -qE "^${chave}=" "$ARQUIVO" || { echo "  ! $chave não existe em $ARQUIVO" >&2; return 0; }
  atual=$({ grep -E "^${chave}=" "$ARQUIVO" || true; } | head -1 | cut -d= -f2-)
  [ -n "$atual" ] && return 0
  sed -i.bak "s|^${chave}=.*|${chave}=${valor}|" "$ARQUIVO" && rm -f "$ARQUIVO.bak"
  echo "  sorteado $chave"
}
substituir PGADMIN_PASSWORD   "$(gerar_senha 28)"
substituir PAINEL_DB_PASSWORD "$(gerar_senha 28)"
substituir PAINEL_SESSION_KEY "$(gerar_hex 32)"
substituir PAINEL_CSRF_KEY    "$(gerar_hex 32)"
substituir PAINEL_ADMIN_SENHA "$(gerar_senha 14)"
substituir PROVISIONER_TOKEN  "$(gerar_hex 32)"
echo "$ARQUIVO pronto. Falta preencher à mão:"
echo "  DOMAIN / ACME_EMAIL            → domínio público e e-mail do Let's Encrypt"
echo "  FIXPAY_TOKEN_API / FIXPAY_SIGN_GRUPO_ID → credenciais da CRM IA na recorrência (sem elas, só cobrança manual)"
echo "  MANDRILL_API_KEY               → e-mail transacional da plataforma (sem ela, envio simulado)"
echo ""
echo "Senha inicial do painel (PAINEL_ADMIN_SENHA): $({ grep '^PAINEL_ADMIN_SENHA=' "$ARQUIVO" || true; } | cut -d= -f2-)"
