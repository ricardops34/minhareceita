# Introdução

Os arquivos estão organizados em pacotes Go, sendo esses os quatro principais:

1. `download/` responsável pelo download dos dados da Receita Federal e do Tesouro Nacional
1. `transform/` é um <abbr title="Extract, Transform, Load">ETL</abbr> que lê os arquivos baixados pelo pacote acima, processa cada um deles e salva os dados no banco de dados — entenda melhor sobre essa etapa lendo sobre os [detalhes do <abbr title="Extract, Transform, Load">ETL</abbr>](etl.md)
1. `db/` implementação dos _backends_ de banco de dados, ou seja, implementa as interfaces necessárias pelo ETL e pela API em diferentes bancos de dados (PostgreSQL é utilizado em produção)
1. `api/` a API web que serve os dados do projeto para o público

Escreva testes e rode os testes, use autoformatação e _linter_:

```console
$ gofmt ./
$ golanglint-ci run ./...
$ go test --race ./...
```

Os testes requerem uma instância de cada banco de dados implementado. Atualmente eles precisam ser configurados em `TEST_POSTGRES_URL` e `TEST_MONGODB_URL`, como no exemplo em `.env`, e podem ser [facilmente criados com o `compose.yml`](containers.md).

O FoundationDB é uma exceção: por exigir a biblioteca C `libfdb_c` instalada no sistema, seu código fonte usa a _build tag_ `foundationdb`. Sem essa _tag_, o arquivo `db/foundationdb.go` é ignorado pelo compilador. Os testes do FoundationDB rodam contra um mock em memória — sem dependência externa. Para compilar ou testar com o FoundationDB:

```console
$ go build -tags foundationdb ./...
$ go test -tags foundationdb ./...
```

## Vibe coding

Sobre contribuições e [_vibe coding_](https://pt.wikipedia.org/wiki/Vibe_coding):

* Você pode usar LLMs para ajudar a gerar código, desde que você leia e entenda o código que foi gerado.
* O código enviado deve ser algo que você testou, compreende e é capaz de discutir sobre a implementação, abstrações e testes sugeridos.
* Você deve seguir os critérios de [transparência como na política da comunidade do Fedora](https://docs.fedoraproject.org/en-US/council/policy/ai-contribution-policy/); resumindo: se uma parte signifiativa da contribuição foi gerada por LLM e você não mudou quase nada, é **obrigatório** informar sobre o emprego dessas ferramentas.

Contribuições que não se encaixem nesses critérios serão simplesmente descartadas. Se você não se importou em escrever teu código, não espere que alguém vá se importar em lê-lo.
