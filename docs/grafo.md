# Grafo (Experimental)

O comando `graph` permite criar uma API para explorar as relações entre quadro societários e pessoas, tanto jurídicas quanto físicas.

!!! info "Funcionalidade experimental"
    Esta é uma funcionalidade experimental e pode sofrer alterações, inclusive ser retirada do ar sem aviso prévio.

* A API do grafo está disponível em `https://grafo.minhareceita.org/`
* Um app experimental construído com essa API, também utilizando [código aberto](https://tangled.org/cuducos.me/meu-garfo), é o [Meu Garfo](https://cuducos.tngl.io/meu-garfo/)

## Como usar

Para consultar as *relações de uma empresa ou de uma pessoa física*, use `GET /<id>`, sendo que o valor de `id` é [o CNPJ para pessoas jurídicas, ou um _hash_ para as demais](./contributing/etl.md#chaves).

??? example "Exemplo de resposta para `GET /33683111000280`"
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

A partir de então navegue no grafo com mais requisições para `/<id>` para saber em quais outros quadro societários essa pessoa (física ou jurídica) está.

Para descobrir a *menor conexão entre duas pessoas físicas ou jurídicas*, use `GET /<id1>/<id2>`. A busca dura no máximo 90 segundos; caso, nesse intervalo, nenhuma conexão seja encontrada, isso não significa que nenhuma conexão é possível.

??? example "Exemplo de resposta para `GET /34712359000103/27516314000106`"
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

Ao [carregar os dados no banco de dados](./servidor.md#tratamento-dos-dados) é criado um arquivo `graph.tar.gz` (por padrão em `data/`). Inicie o servidor da API de grafos com:

```console
$ minha-receita graph
```
