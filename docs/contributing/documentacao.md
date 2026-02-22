# Documentação

Utilizamos o [Zensical](https://zensical.org/):

```console
$ docker pull zensical/zensical
$ docker run --rm -v $(pwd):/docs zensical/zensical build
```

A documentação vai ser gerada em `site/index.html`. Para servir enquanto desenvolve:

```console
$ docker run -p 8000:8000 --rm -v $(pwd):/docs zensical/zensical serve --dev-addr 0.0.0.0:8000
```
