#!/usr/bin/env bash
# (dev) Cria grupo, cliente e licença de exemplo pelo HTTP do painel, como um
# humano faria, e provisiona o ambiente em https://crmia.localhost[:porta]/acme.
set -euo pipefail
cd "$(dirname "$0")/../.."
[ -f .env ] || { echo "sem .env — rode make dev antes"; exit 1; }
HOST=crmia.localhost
PORTA="$(grep -E '^HTTPS_PORT=' .env | cut -d= -f2- || true)"; PORTA="${PORTA:-443}"
SUFIXO=""; [ "$PORTA" = "443" ] || SUFIXO=":$PORTA"
BASE="https://$HOST$SUFIXO/painel"
EMAIL="$(grep -E '^PAINEL_ADMIN_EMAIL=' .env | cut -d= -f2-)"
SENHA="$(grep -E '^PAINEL_ADMIN_SENHA=' .env | cut -d= -f2-)"
SLUG="${SLUG:-acme}"; NOME="${NOME:-ACME Vendas}"; CNPJ="${CNPJ:-11222333000181}"
COOKIES="$(mktemp)"; trap 'rm -f "$COOKIES"' EXIT
c() { curl -sk --resolve "$HOST:$PORTA:127.0.0.1" -c "$COOKIES" -b "$COOKIES" "$@"; }
echo "→ aguardando o painel"
for i in $(seq 1 60); do c -o /dev/null -w '' "$BASE/saude" && break || sleep 2; done
echo "→ login como $EMAIL"
c -o /dev/null -X POST "$BASE/login" --data-urlencode "email=$EMAIL" --data-urlencode "senha=$SENHA"
CSRF="$(awk '$6=="crmia_painel_csrf"{print $7}' "$COOKIES")"
[ -n "$CSRF" ] || { echo "login falhou (confira PAINEL_ADMIN_EMAIL/SENHA no .env)"; exit 1; }
if c "$BASE/clientes?q=$SLUG" | grep -q "/clientes/[0-9]*\">"; then
  CID="$(c "$BASE/clientes?q=$SLUG" | grep -o 'href="/painel/clientes/[0-9]*"' | head -1 | grep -o '[0-9]*')"
  echo "→ cliente $SLUG já existe (#$CID)"
else
  echo "→ criando grupo"
  GID="$(c -o /dev/null -w '%{redirect_url}' -X POST "$BASE/grupos/novo" --data-urlencode "_csrf=$CSRF" --data-urlencode "nome=Rede $NOME" --data-urlencode "cnpj_responsavel=$CNPJ" --data-urlencode "email_financeiro=financeiro@exemplo.com" --data-urlencode "cobranca=unificada" --data-urlencode "cep=60000000" --data-urlencode "logradouro=Rua Exemplo" --data-urlencode "numero=100" --data-urlencode "bairro=Centro" --data-urlencode "municipio=Fortaleza" --data-urlencode "uf=CE" | grep -o 'grupos/[0-9]*' | grep -o '[0-9]*')"
  echo "   grupo #$GID"
  echo "→ criando cliente (CNPJ)"
  CID="$(c -o /dev/null -w '%{redirect_url}' -X POST "$BASE/clientes/novo" --data-urlencode "_csrf=$CSRF" --data-urlencode "grupo_id=$GID" --data-urlencode "razao_social=$NOME Ltda" --data-urlencode "nome_fantasia=$NOME" --data-urlencode "cnpj=$CNPJ" --data-urlencode "slug=$SLUG" --data-urlencode "modelo_crm=vendas_b2b" --data-urlencode "cidade=Fortaleza" --data-urlencode "uf=CE" --data-urlencode "admin_nome=Administrador" --data-urlencode "admin_email=admin@$SLUG.local" --data-urlencode "plano_id=2" --data-urlencode "usuarios_contratados=5" --data-urlencode "auto_atualizar=on" --data-urlencode "cor_primaria=#6d5df6" | grep -o 'clientes/[0-9]*' | grep -o '[0-9]*')"
  echo "   cliente #$CID"
fi
echo "→ provisionando (PostgreSQL + Redis + app; leva 1–2 min)"
DEST="$(c -o /dev/null -w '%{redirect_url}' -X POST "$BASE/clientes/$CID/provisionar" --data-urlencode "_csrf=$CSRF" --data-urlencode "versao=")"
echo "   $(python3 -c "import sys,urllib.parse as u;q=u.parse_qs(u.urlparse(sys.argv[1]).query);print((q.get('ok') or q.get('erro') or ['?'])[0])" "$DEST")"
SENHA_ADMIN="$(c "$BASE/clientes/$CID" | sed -n 's/.*<span class="senha-inicial">\([^<]*\)<\/span>.*/\1/p' | head -1)"
if ! c "$BASE/licencas" | grep -q "Rede $NOME"; then
  echo "→ contratando licença recorrente (simulada)"
  GRUPO="${GID:-$(c "$BASE/clientes/$CID" | grep -o 'grupos/[0-9]*' | head -1 | grep -o '[0-9]*')}"
  c -o /dev/null -X POST "$BASE/licencas/nova" --data-urlencode "_csrf=$CSRF" --data-urlencode "grupo_id=$GRUPO" --data-urlencode "plano_id=2" --data-urlencode "unidades=1" --data-urlencode "periodicidade=mensal" --data-urlencode "cobranca=recorrente" --data-urlencode "inicio=$(date +%F)"
fi
echo ""
echo "✓ CRM do cliente:   https://$HOST$SUFIXO/$SLUG"
echo "  login do admin:   admin@$SLUG.local"
[ -n "$SENHA_ADMIN" ] && echo "  senha inicial:    $SENHA_ADMIN   (exibida só uma vez — anote)"
echo "  ficha no painel:  $BASE/clientes/$CID"
echo "  link do cartão:   em Licenças → “link do cartão” (simulado: 4111 1111 1111 1111 aprova)"
