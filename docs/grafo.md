# Grafo (Experimental)

O comando `graph` é experimental e permite explorar as relações entre quadro societários e pessoas, tanto jurídicas quanto físicas.

!!! info "Funcionalidade experimental"
    Esta é uma funcionalidade experimental e pode sofrer alterações, inclusive ser retirada do ar sem aviso prévio.

A API do grafo está disponível em `https://grafo.minhareceita.org/`.

## Como usar

Para consultar o quadro societário de uma empresa, use `GET /qsa/<CNPJ>`.

??? example "Exemplo de resposta para /qsa/19131243000197"
    ```json
    {
      "company_id": "19131243000197",
      "name": "OPEN KNOWLEDGE BRASIL",
      "partners": [
        {
          "partner_id": "7f34c2ed2c1e8587d5598686e0c65360",
          "name": "HAYDEE SVAB",
          "cpf": "***112108**"
        }
      ]
    }
    ```

Nessa resposta, o formato do `partner_id` varia de acordo com a natureza da pessoa:

* Para pessoas jurídicas, esse campo é o próprio CNPJ
* Para pessoas físicas, esse campo é um campo de texto (pseudo) aleatório (um _hash_ MD5 do CPF e nome como constam nos dados públicos do CNPJ)

Além disso, os campos `name` e `cpf` só são retornados para pessoas físicas.

A partir de então navegue no grafo com mais requisições para `/cnpjs/<partner_id>` para saber em quais outros quadro societários essa pessoa (física ou jurídica) está.

??? example "Exemplo de resposta para /cnpjs/7f34c2ed2c1e8587d5598686e0c65360"
    ```json
    {
      "partner_id": "7f34c2ed2c1e8587d5598686e0c65360",
      "name": "HAYDEE SVAB",
      "cpf": "***112108**",
      "companies": [
        {
          "cnpj": "19131243000197",
          "name": "OPEN KNOWLEDGE BRASIL"
        }
      ]
    }
    ```

## Rodando localmente

Para iniciar o servidor de grafo, execute:

```bash
minha-receita graph
```

Este comando irá:
1. Criar a tabela ou coleção `graph` no banco de dados baseada nos dados já carregados em `DATABASE_URL` (para pular esta etapa utilize `--skip-create`).
2. Iniciar um servidor web (por padrão na porta 8000, ou na porta definida pela variável de ambiente `PORT`).
