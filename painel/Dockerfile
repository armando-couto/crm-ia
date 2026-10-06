# Painel administrativo do CRM IA.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /painel .

FROM gcr.io/distroless/static-debian12:nonroot
ENV TZ=America/Sao_Paulo
COPY --from=build /painel /painel
EXPOSE 7894
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD ["/painel", "-healthcheck"]
ENTRYPOINT ["/painel"]
