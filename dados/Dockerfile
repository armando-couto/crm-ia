# Serviço de dados públicos do CRM IA (CNPJ da Receita e dívida ativa da PGFN).
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /dados .
# A imagem final é distroless (sem shell): o diretório de trabalho nasce aqui
# e é copiado com dono nonroot. Um volume nomeado montado em /trabalho herda
# o dono do diretório da imagem na primeira montagem; sem isto ele nasceria
# do root e o processo não conseguiria gravar os downloads.
RUN mkdir -p /trabalho

FROM gcr.io/distroless/static-debian12:nonroot
ENV TZ=America/Sao_Paulo
COPY --from=build /dados /dados
COPY --from=build --chown=65532:65532 /trabalho /trabalho
VOLUME /trabalho
EXPOSE 7896
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD ["/dados", "-healthcheck"]
ENTRYPOINT ["/dados"]
