# Grafo (Experimental)

O comando `graph` é experimental e permite explorar as relações entre quadro societários e pessoas, tanto jurídicas quanto físicas.

!!! info "Funcionalidade experimental"
    Esta é uma funcionalidade experimental e pode sofrer alterações, inclusive ser retirada do ar sem aviso prévio.

* A API do grafo está disponível em `https://grafo.minhareceita.org/`
* Um app experimental construído com essa API, também utilizando [código aberto](https://tangled.org/cuducos.me/meu-garfo), é o [Meu Garfo](https://cuducos.tngl.io/meu-garfo/)

## Como usar

### Conceito de `id`

Essa API usa uma `id` para identificar tanto pessoas físicas quanto pessoas jurídicas:

* Para pessoas jurídicas, o identificador é o próprio CNPJ
* Para pessoas físicas, o identificador é um campo de texto (pseudo) aleatório (um _hash_ MD5 do CPF e nome como constam nos dados públicos do CNPJ)

### Consultar

Para consultar as *relações de uma empresa ou de uma pessoa física*, use `GET /relacoes/<id>`.

??? example "Exemplo de resposta para `GET /relacoes/33683111000280`"
    ```json
    [
      {
        "cnpj": "33683111000280",
        "razao_social": "SERVICO FEDERAL DE PROCESSAMENTO DE DADOS (SERPRO)",
        "id": "3573de271293797f2abddc036be8f35e",
        "nome": "ALEXANDRE BRANDAO HENRIQUES MAIMONI",
        "cpf": "***641988**"
      },
      {
        "cnpj": "33683111000280",
        "razao_social": "SERVICO FEDERAL DE PROCESSAMENTO DE DADOS (SERPRO)",
        "id": "70ec112375aec9de541ae0b7c54d7cac",
        "nome": "ANDRE PICOLI AGATTE",
        "cpf": "***035378**"
      },
      {
        "cnpj": "33683111000280",
        "razao_social": "SERVICO FEDERAL DE PROCESSAMENTO DE DADOS (SERPRO)",
        "id": "5581a696ef726dcb05b563690e9d3ced",
        "nome": "ARIADNE DE SANTA TERESA LOPES FONSECA",
        "cpf": "***077170**"
      },
      {
        "cnpj": "33683111000280",
        "razao_social": "SERVICO FEDERAL DE PROCESSAMENTO DE DADOS (SERPRO)",
        "id": "76677f4091254d210276fc0febfb97a0",
        "nome": "ERMES FERREIRA COSTA NETO",
        "cpf": "***269764**"
      },
      {
        "cnpj": "33683111000280",
        "razao_social": "SERVICO FEDERAL DE PROCESSAMENTO DE DADOS (SERPRO)",
        "id": "40089c4776191cc8e870d14f2f431477",
        "nome": "OSMAR QUIRINO DA SILVA",
        "cpf": "***109571**"
      },
      {
        "cnpj": "33683111000280",
        "razao_social": "SERVICO FEDERAL DE PROCESSAMENTO DE DADOS (SERPRO)",
        "id": "3d988b184dcd37bc88e9766af21cf25f",
        "nome": "WALLYSON LEMOS DOS REIS OLIVEIRA",
        "cpf": "***286423**"
      },
      {
        "cnpj": "33683111000280",
        "razao_social": "SERVICO FEDERAL DE PROCESSAMENTO DE DADOS (SERPRO)",
        "id": "79292eb5f29feca8f3434e6f44b2f4a4",
        "nome": "WILTON ITAIGUARA GONCALVES MOTA",
        "cpf": "***623503**"
      }
    ]
    ```

A partir de então navegue no grafo com mais requisições para `/relacoes/<id>` para saber em quais outros quadro societários essa pessoa (física ou jurídica) está.

Para descobrir a *menor conexão entre duas pessoas físicas ou jurídicas*, use `GET /conexao/<id1>/<id2>` (limitado a uma distância máxima de 16).

??? example "Exemplo de resposta para `GET /conexao/34712359000103/27516314000106`"
    ```json
    [
      {
        "cnpj": "34712359000103",
        "razao_social": "INSTITUTO DE ACAO CONSERVADORA",
        "id": "c23aa26674515674f7738d6b68c37d6d",
        "nome": "HELOISA WOLF BOLSONARO",
        "cpf": "***791930**"
      },
      {
        "cnpj": "46053446000185",
        "razao_social": "H&E PRODUCOES LTDA",
        "id": "c23aa26674515674f7738d6b68c37d6d",
        "nome": "HELOISA WOLF BOLSONARO",
        "cpf": "***791930**"
      },
      {
        "cnpj": "46053446000185",
        "razao_social": "H&E PRODUCOES LTDA",
        "id": "1defe1dc76aa2a3d5533a0cf1d444fa1",
        "nome": "EDUARDO NANTES BOLSONARO",
        "cpf": "***553657**"
      },
      {
        "cnpj": "27516314000106",
        "razao_social": "BOLSONARO DIGITAL LTDA",
        "id": "1defe1dc76aa2a3d5533a0cf1d444fa1",
        "nome": "EDUARDO NANTES BOLSONARO",
        "cpf": "***553657**"
      }
    ]
    ```
## Rodando localmente

Para iniciar o servidor de grafo, execute:

```console
$ minha-receita graph
```

Este comando irá:
1. Criar a tabela ou coleção `graph` no banco de dados baseada nos dados já carregados em `DATABASE_URL` (para pular esta etapa utilize `--skip-create`).
2. Iniciar um servidor web (por padrão na porta 8000, ou na porta definida pela variável de ambiente `PORT`).
