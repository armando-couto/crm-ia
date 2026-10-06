# syntax=docker/dockerfile:1.7
# Imagem do app de um cliente CRM IA (API Go + frontend Vue embutido).
# A tag da imagem é a "versão" que o painel gerencia por cliente.
#   docker build --build-arg VERSAO=1.2.0 -t crmia/app:1.2.0 ./app

# ---- frontend ---------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- backend ----------------------------------------------------------------
FROM golang:1.26-alpine AS api
ARG VERSAO=dev
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.versaoBuild=${VERSAO}" -o /out/crm-ia .

# ---- imagem final -----------------------------------------------------------
FROM alpine:3.21
ARG VERSAO=dev
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 crmia
ENV TZ=America/Sao_Paulo APP_ENV=production PORT=8080 VERSAO=${VERSAO}
WORKDIR /app
COPY --from=api /out/crm-ia /app/crm-ia
COPY --from=web /src/web/dist /app/web/dist
USER crmia
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/app/crm-ia", "-healthcheck"]
LABEL org.opencontainers.image.title="CRM IA" org.opencontainers.image.version="${VERSAO}"
CMD ["/app/crm-ia"]
