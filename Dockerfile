ARG GOLANG_VERSION=1.26
ARG DISTROLESS_TAG=nonroot

FROM golang:${GOLANG_VERSION}-trixie AS build
ENV GOEXPERIMENT=jsonv2
WORKDIR /minha-receita
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /usr/bin/minha-receita

FROM debian:trixie AS download
WORKDIR /download
ARG GRAPH_URL=https://bucket.minhareceita.org/graph.tar.gz
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl && \
    curl -L ${GRAPH_URL} | tar -xzv

FROM gcr.io/distroless/cc-debian13:${DISTROLESS_TAG} AS base
COPY --from=build /usr/bin/minha-receita /usr/bin/minha-receita
ENTRYPOINT ["/usr/bin/minha-receita"]

FROM base AS graph
LABEL org.opencontainers.image.description="Sua API de grafos para consulta de informações do CNPJ da Receita Federal"
LABEL org.opencontainers.image.source="https://codeberg.org/cuducos/minha-receita"
LABEL org.opencontainers.image.title="Minha Receita - Grafo"
COPY --chown=65532:6553 --from=download /download/graph.db /graph.db
CMD ["graph", "--graph", "/graph.db"]

FROM base AS main
LABEL org.opencontainers.image.description="Sua API web para consulta de informações do CNPJ da Receita Federal"
LABEL org.opencontainers.image.source="https://codeberg.org/cuducos/minha-receita"
LABEL org.opencontainers.image.title="Minha Receita"
CMD ["api"]
