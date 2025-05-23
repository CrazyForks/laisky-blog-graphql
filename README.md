# laisky-blog-graphql

graphql backend for laisky-blog depends on gqlgen & gin.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Commitizen friendly](https://img.shields.io/badge/commitizen-friendly-brightgreen.svg)](http://commitizen.github.io/cz-cli/)
[![Go Report Card](https://goreportcard.com/badge/github.com/Laisky/laisky-blog-graphql)](https://goreportcard.com/report/github.com/Laisky/laisky-blog-graphql)
[![GoDoc](https://godoc.org/github.com/Laisky/laisky-blog-graphql?status.svg)](https://godoc.org/github.com/Laisky/laisky-blog-graphql)
[![Build Status](https://travis-ci.com/Laisky/laisky-blog-graphql.svg?branch=master)](https://travis-ci.com/Laisky/laisky-blog-graphql)
[![codecov](https://codecov.io/gh/Laisky/laisky-blog-graphql/branch/master/graph/badge.svg)](https://codecov.io/gh/Laisky/laisky-blog-graphql)


Example: <https://gq.laisky.com/ui/>

Introduction: <https://blog.laisky.com/p/gqlgen/>


Run:

```sh
go generate
go run -race main.go \
    --listen=127.0.0.1:8080 \
    --config=./docs/settings.yml \
    --debug
```

Build:

```sh
docker build . -t ppcelery/laisky-blog-graphql:0.3.1
```
