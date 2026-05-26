FROM golang:1.26-trixie AS build
ENV GOEXPERIMENT=jsonv2
WORKDIR /minha-receita
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN go build -o /usr/bin/minha-receita

FROM debian:trixie-slim
LABEL org.opencontainers.image.description="Sua API web para consulta de informações do CNPJ da Receita Federal"
LABEL org.opencontainers.image.source="https://codeberg.org/cuducos/minha-receita"
LABEL org.opencontainers.image.title="Minha Receita"

RUN apt-get update && \
    # Security updates
    apt-get upgrade -y \
        bsdutils \
        coreutils \
        libc-bin \
        libc6 \
        libsqlite3-0 \
        libsystemd0 \
        libudev1 \
        util-linux && \
    apt-get install -y --no-install-recommends ca-certificates && \
    update-ca-certificates && \
    apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build /usr/bin/minha-receita /usr/bin/minha-receita
ENTRYPOINT ["/usr/bin/minha-receita"]
CMD ["api"]
