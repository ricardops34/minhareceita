# Instalação local

Existem três formas de rodar essa aplicação localmente:

* ou com a imagem de container
* ou gerando o binário a partir do código fonte
* ou com o `compose.yml` — apenas para desenvolvimento (não recomendado para o banco de dados completo)

As duas últimas alternativas necessitam do código fonte. Você pode usar o Git para baixar o código do projeto:

```console
$ git clone https://codeberg.org/cuducos/minha-receita.git
```

## Requisitos e instalação

É necessário cerca de 180 GB disponíveis de espaço em disco para armazenar os dados.

Por exemplo, em um Postgres rodando em um servidor separado, a divisão é aproximadamente:

* Banco de dados
    * 140 GB para as tabelas
    * 10 GB para os índices no banco de dados
* Download e transformação dos dados:
    * 8 GB para os downloads
    * 15 GB de espaço temporário para o tratamento dos dados
    * 7 GB para gerar o arquivo `graph.tar.gz` do [grafo](grafo.md)

### Banco de dados

* O banco de dados gerado utiliza cerca de 140 GB
* Rodar o banco de dados localmente com containers só é recomendado para desenvolvimento (não recomendado para o banco de dados completo)

#### Download e transformação dos dados

* Os arquivos da Receita federal tem menos de 10 GB
* O processo de importação utiliza uma estrutura temporária de cerca de 15 GB

### Imagem de container

* [Podman](https://podman.io/), [Docker](https://www.docker.com/) ou qualquer _runtime_ de container compatíveL

Baixar a imagem com:

```console
$ docker pull atcr.io/cuducos.me/minha-receita:main
```

### A partir do código fonte

* [Go](https://golang.org/) versão 1.26
* Variável de ambiente `GOEXPERIMENT=jsonv2`

Depois de clonar o repositório, baixe as dependências e compile a aplicação para um diretório incluído no `PATH`, por exemplo:

```console
$ go get
$ go build -o /usr/local/bin/minha-receita main.go
```

### Compose

* [Podman](https://podman.io/), [Docker](https://www.docker.com/) ou qualquer _runtime_ de container compatíveL com plugin `compose`
* Arquivo `.env` (copie o `.env.sample` e ajuste caso necessário)

Gere as imagens dos containers com:

```console
$ docker compose build
```

## Imagem de container

O `Dockerfile` define duas imagens a partir do mesmo arquivo: uma para a aplicação principal (API web, ETL, etc.) e outra para a API do grafo. Para escolher qual imagem construir, utilize a opção `--target` do comando de build.

Baixe a imagem pronta com:

```console
$ docker pull atcr.io/cuducos.me/minha-receita:main
```

Ou construa localmente a imagem principal:

```console
$ docker build --target main -t minha-receita .
$ docker run -e DATABASE_URL=$DATABASE_URL -p 8000:8000 minha-receita
```

## Execução e configurações

Várias configurações podem ser passadas para a CLI, e elas estão documentadas no `--help` de cada comando da aplicação.

### Exemplos

#### Imagem de container

```console
$ docker run --rm atcr.io/cuducos.me/minha-receita:main --help
$ docker run --rm atcr.io/cuducos.me/minha-receita:main api --help
```

#### A partir do código fonte

```console
$ minha-receita --help
$ minha-receita api --help
```

#### Compose

```console
$ docker compose run --rm minha-receita --help
$ docker compose run --rm minha-receita api --help
```

### Variáveis de ambiente

Para facilitar a manutenção, algumas variáveis de ambiente podem ser utilizadas, mas todas são opcionais:

| Variável | Descrição |
|---|---|
| `DATABASE_URL` | URI de acesso ao banco de dados |
| `PORT` | Porta na qual a API web ficará disponível |
| `CACHE_SIZE` | Tamanho máximo do cache em MB |
| `BLOOM_FILTER_SIZE` | Tamanho máximo do bloom filter em MB |
| `GRAPH_PATH` | Localização do arquivo do [grafo](grafo.md) |
| `TEST_POSTGRES_URL` | URI de acesso ao banco de dados PostgreSQL para ser utilizado nos testes |
| `TEST_MONGODB_URL` | URI de acesso ao banco de dados MongoDB para ser utilizado nos testes |
