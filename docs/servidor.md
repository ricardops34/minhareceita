# Criando seu próprio servidor

## Banco de dados

O projeto requer um banco de dados PostgreSQL ou MongoDB e os comandos que requerem banco de dados aceitam `--database-uri` (ou `-u`) como argumento com a URI de acesso ao banco de dados (o padrão é o valor da variável de ambiente `DATABASE_URL`).

Caso deseje usar o Docker Compose do projeto para subir uma instância do banco de dados:

```console
$ docker compose up -d postgres
$ docker compose up -d mongo
```

Usando PostgreSQL, a URI será `postgres://minhareceita:minhareceita@localhost:5432/minhareceita?sslmode=disable`.

Usando MongoDB, a URI será `mongodb://minhareceita:minhareceita@localhost:27017/minhareceita?authSource=admin`.

### Provisionando o banco de dados

O comando `provision db` instala e configura o PostgreSQL em um servidor remoto via SSH, salva as credenciais em `/etc/minha-receita/.env` e exibe as credenciais de acesso apenas uma vez (quando criadas). O processo cria dois usuários no Postgres: `etl` (com permissões de escrita) e `web` (apenas leitura), ambos tem senhas distintas e aleatórias.

O comando `provision web` inicia a API web principal (`minhareceita.org`) e a API do grafo (`grafo.minhareceita.org`) no mesmo servidor, usando as credenciais salvas pelo `provision db`. Só pode ser executado depois do `provision db`.

#### Requisitos

* servidor Debian ou Ubuntu
* cliente SSH disponível no `$PATH`
* acesso SSH (chave ou SSH agent) e acesso `sudo` ao servidor de destino

#### Exemplos

```console
$ minha-receita provision db root@200.100.0.1
```

E, depois de carregar os dados:

```console
$ minha-receita provision web root@200.100.0.1
```

## Dados

Os dados são disponibilizados mensalmente pela [Receita Federal](https://dados.gov.br/dados/conjuntos-dados/cadastro-nacional-da-pessoa-juridica-cnpj). Para o comando `transform` funcionar, é necessário um diretório contendo:

* o arquivo `YYYY-MM.zip` da Receita Federal com os dados do CNPJ
* os arquivos de regime tributário com prefixo `entidades-*.zip`

A fonte oficial desses arquivos é sempre o [Portal de Dados Abertos](https://dados.gov.br) do governo federal:

1. Busque por _CPNJ_ e escolha _Cadastro Nacional da Pessoa Jurídica - CNPJ_
2. Na aba _Recursos_ os arquivos necessários são encontrados em:
    * Inscrições no CNPJ (`YYYY-MM.zip`)
    * Regimes Tributários (`entidades-*.zip`)

## Tratamento dos dados

O comando `transform` transforma os arquivos para o formato JSON, consolidando as informações de todos os arquivos CSV. Esse JSON é armazenado diretamente no banco de dados. Para tanto, é preciso criar a tabela no banco de dados com o comando `create` (o comando `drop` pode ser utilizado para excluir essa mesma tabela).

Para especificar onde ficam os arquivos originais da Receita Federal e do Tesouro Nacional, o comando aceita como argumento `--directory` (ou `-d`), sendo o padrão `data/`.


!!! danger "Importante"
    Não existe “atualizar” o banco de dados. O processo de _upsert_ mais o gerenciamento de registros ausentes nos novos lotes faria o comando `transform` extremamente lento. Como a ideia é reproduzir o estado atual dos dados oficiais divulgados pela Receita Federal, o recomendado é subir um novo banco de dados, apontar a API web para o novo banco de dados, e depois excluir o banco de dados antigo.

### Exemplos de uso

Sem Docker, com a variável de ambiente `DATABASE_URL` configurada:

```console
$ minha-receita drop  # caso necessário
$ minha-receita create
$ minha-receita transform
```

Com Docker:

```console
$ docker compose run --rm minha-receita drop  # caso necessário
$ docker compose run --rm minha-receita create
$ docker compose run --rm minha-receita transform -d /mnt/data/
```

### Questões de privacidade

Assim como o [`socios-brasil`](https://github.com/turicas/socios-brasil#privacidade) removemos alguns dados para evitar exposição de dados sensíveis de pessoas físicas, bem como SPAM. A opção `--no-privacy` do comando `transform` remove essa precaução de privacidade.


## Iniciando a API web

A API web é uma aplicação super simples que, por padrão, ficará disponível em [`localhost:8000`](http://localhost:8000).

### Exemplos de uso

Sem Docker, com a variável de ambiente `DATABASE_URL` configurada:

```console
$ minha-receita api
```

Com Docker:

```console
$ docker compose up
```
