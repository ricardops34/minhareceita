ARG GOLANG_VERSION=1.26
ARG DISTROLESS_TAG=nonroot

FROM golang:${GOLANG_VERSION}-trixie AS build
ENV GOEXPERIMENT=jsonv2
WORKDIR /minha-receita
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /usr/bin/minha-receita

FROM gcr.io/distroless/cc-debian13:${DISTROLESS_TAG} AS base
COPY --from=build /usr/bin/minha-receita /usr/bin/minha-receita
ENTRYPOINT ["/usr/bin/minha-receita"]

FROM base AS graph
LABEL org.opencontainers.image.description="Sua API de grafos para consulta de informações do CNPJ da Receita Federal"
LABEL org.opencontainers.image.source="https://codeberg.org/cuducos/minha-receita"
LABEL org.opencontainers.image.title="Minha Receita - Grafo"
ARG GRAPH_PATH
COPY --chown=65532:65532 ${GRAPH_PATH} /graph.tar.gz
CMD ["graph", "--graph", "/graph.tar.gz"]

FROM base AS main
LABEL org.opencontainers.image.description="Sua API web para consulta de informações do CNPJ da Receita Federal"
LABEL org.opencontainers.image.source="https://codeberg.org/cuducos/minha-receita"
LABEL org.opencontainers.image.title="Minha Receita"
CMD ["api"]
