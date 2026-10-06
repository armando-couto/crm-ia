#!/usr/bin/env bash
# Release do app dos clientes:
#   make publicar                # patch (1.2.3 → 1.2.4)
#   make publicar SALTO=minor    # 1.2.3 → 1.3.0  (SALTO=major → 2.0.0)
#   make publicar VERSAO=2.0.0   # explícita
# Testa, constrói a imagem crmia/app:X.Y.Z, cria e envia a tag vX.Y.Z (a
# pipeline publica as imagens) e registra a versão no painel alcançável.
set -euo pipefail
cd "$(dirname "$0")/../.."
[ -f .env ] || { echo "sem .env — rode make env"; exit 1; }
valor() { { grep -E "^$1=" .env || true; } | head -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//'; }
TOKEN="$(valor PROVISIONER_TOKEN)"; DOMINIO="$(valor DOMAIN)"
ULTIMA="$(git tag --list 'v[0-9]*' --sort=-v:refname | head -1)"
if [ -n "${VERSAO:-}" ]; then NOVA="${VERSAO#v}"; elif [ -z "$ULTIMA" ]; then NOVA="1.0.0"; else
  IFS=. read -r MA MI PA <<< "${ULTIMA#v}"
  case "${SALTO:-patch}" in major) NOVA="$((MA+1)).0.0" ;; minor) NOVA="$MA.$((MI+1)).0" ;; *) NOVA="$MA.$MI.$((PA+1))" ;; esac
fi
if git status --porcelain -- app painel provisioner | grep -q .; then echo "erro: há mudanças não commitadas. Commite antes de publicar." >&2; exit 1; fi
echo "→ versão $NOVA (anterior: ${ULTIMA:-nenhuma})"
echo "→ testes"; (cd app && go vet ./... && go test ./... >/dev/null) && (cd app/web && npm run typecheck --silent && npm test --silent >/dev/null)
echo "→ imagem crmia/app:$NOVA"; docker build -q --build-arg VERSAO="$NOVA" -t "crmia/app:$NOVA" -t crmia/app:latest ./app >/dev/null
echo "→ tag v$NOVA"; git tag -a "v$NOVA" -m "CRM IA $NOVA" && git push origin "v$NOVA"
PAINEL="${PAINEL_URL:-https://$DOMINIO/painel}"
codigo=$(curl -sSk -o /dev/null -w '%{http_code}' --max-time 20 -X POST "$PAINEL/api/interno/versoes" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"tag\":\"$NOVA\",\"descricao\":\"publicada por make publicar\",\"estavel\":true,\"padrao\":${PADRAO:-true},\"aplicar\":${APLICAR:-true}}" || echo 000)
case "$codigo" in 2??) echo "✓ versão $NOVA registrada no painel ($PAINEL)" ;; *) echo "aviso: o painel respondeu $codigo — cadastre a versão $NOVA na tela Versões" ;; esac
