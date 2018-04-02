# graphql-schema

[![Documentation](https://godoc.org/github.com/shoobyban/graphql-schema?status.svg)](http://godoc.org/github.com/shoobyban/graphql-schema)
[![Go Report Card](https://goreportcard.com/badge/github.com/shoobyban/graphql-schema)](https://goreportcard.com/report/github.com/shoobyban/graphql-schema)
[![Build Status](https://travis-ci.org/shoobyban/graphql-schema.svg?branch=master)](https://travis-ci.org/shoobyban/graphql-schema)

Schema language parser for [graphql-go](https://github.com/graphql-go/graphql)

## Usage

```go
package main

import (
	"fmt"

	"github.com/graphql-go/graphql"
	schema "github.com/shoobyban/graphql-schema"
)

func main() {
	params := graphql.Params{
		Schema: schema.MustBuildSchema(`
			type Query {
				hex(r: Int, g: Int, b: Int): String
			}
			`, map[string]graphql.FieldResolveFn{
			"hex": func(p graphql.ResolveParams) (interface{}, error) {
				r := p.Args["r"]
				g := p.Args["g"]
				b := p.Args["b"]
				return fmt.Sprintf("#%02x%02x%02x", r, g, b), nil
			},
		}),
		RequestString: `query { hex(r: 0, g: 0, b: 0) }`,
	}
	result := graphql.Do(params)
	fmt.Println(result)
}
```
