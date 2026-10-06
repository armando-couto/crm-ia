# Aplicação do ambiente de um cliente CRM IA. Uma imagem, muitos clientes: a
# identidade do tenant (slug, nome, tema, segredos, licença) chega por
# variáveis de ambiente no boot, injetadas pelo provisionador.
#
# Etapa 1: build do frontend (Vue).
FROM node:22-alpine AS front
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# Etapa 2: build do backend (Go), com a versão gravada no binário.
FROM golang:1.26-alpine AS back
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSAO=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.versaoBuild=${VERSAO}" -o /crm-ia ./application.go

# Etapa 3: imagem final, pequena, sem shell de root.
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 crmia
ENV TZ=America/Sao_Paulo APP_ENV=production PORT=8080
WORKDIR /app
COPY --from=back /crm-ia /app/crm-ia
COPY --from=front /src/web/dist /app/web/dist
USER crmia
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=25s --retries=3 CMD ["/app/crm-ia", "-healthcheck"]
ENTRYPOINT ["/app/crm-ia"]
