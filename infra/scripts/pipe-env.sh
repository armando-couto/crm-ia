#!/usr/bin/env bash
# Leva o .env de produção para a pipeline: valida e grava o segredo
# ENV_PRODUCTION no GitHub (o arquivo inteiro, linhas CHAVE=valor).
#   bash infra/scripts/pipe-env.sh              # confere
#   bash infra/scripts/pipe-env.sh --gravar     # grava o segredo
set -euo pipefail
cd "$(dirname "$0")/../.."
ARQUIVO="${ARQUIVO:-.env.production}"; MODO="${1:-}"
command -v gh >/dev/null || { echo "precisa do gh (brew install gh)"; exit 1; }
[ -f "$ARQUIVO" ] || { echo "não achei $ARQUIVO — gere com: bash infra/scripts/gerar-env.sh $ARQUIVO"; exit 1; }
valor() { { grep -E "^$1=" "$ARQUIVO" || true; } | head -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//'; }
faltando=()
for chave in DOMAIN ACME_EMAIL PGADMIN_PASSWORD PAINEL_DB_PASSWORD PAINEL_SESSION_KEY PAINEL_CSRF_KEY PROVISIONER_TOKEN; do
  [ -n "$(valor "$chave")" ] || faltando+=("$chave")
done
if [ ${#faltando[@]} -gt 0 ]; then echo "✗ vazias: ${faltando[*]}"; exit 1; fi
echo "→ obrigatórias ok"
for chave in FIXPAY_TOKEN_API FIXPAY_SIGN_GRUPO_ID MANDRILL_API_KEY; do
  [ -n "$(valor "$chave")" ] || echo "  aviso: $chave vazia (recurso desligado em produção)"
done
if [ "$MODO" = "--gravar" ]; then gh secret set ENV_PRODUCTION < "$ARQUIVO" && echo "✓ ENV_PRODUCTION gravado"; else echo "(use --gravar para enviar ao GitHub)"; fi
