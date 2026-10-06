# Provisionador do CRM IA. Precisa do CLI do Docker (fala com o daemon do nó
# pelo socket montado) — por isso a imagem final é alpine com docker-cli e o
# plugin compose, não distroless.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY go.sum* ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /provisioner .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata docker-cli docker-cli-compose
ENV TZ=America/Sao_Paulo
COPY --from=build /provisioner /provisioner
# O template dos ambientes vai dentro da imagem (a stack do Swarm não monta
# diretórios do repositório). TEMPLATE_PATH aponta para aqui por padrão.
COPY templates/ /templates/
EXPOSE 7898
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/provisioner", "-healthcheck"]
ENTRYPOINT ["/provisioner"]
