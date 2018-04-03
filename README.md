# graphql-schema

[![Documentation](https://godoc.org/github.com/shoobyban/graphql-schema?status.svg)](http://godoc.org/github.com/shoobyban/graphql-schema)
[![Go Report Card](https://goreportcard.com/badge/github.com/shoobyban/graphql-schema)](https://goreportcard.com/report/github.com/shoobyban/graphql-schema)
[![Build Status](https://travis-ci.org/shoobyban/graphql-schema.svg?branch=master)](https://travis-ci.org/shoobyban/graphql-schema)

GraphQL Schema language parser for [graphql-go](https://github.com/graphql-go/graphql) based on [GraphQL-js](https://github.com/graphql/graphql-js/)

## Goals

* [ ] full support of [GraphQL schema spec (October 2016)](https://facebook.github.io/graphql/)
  * [x] simple Query declaration
  * [x] type declarations (without interfaces)
  * [x] arrays (list types)
  * [x] argument handling
  * [ ] argument '=' handling
  * [ ] non-null check (!)
  * [ ] interfaces, implements keyword
  * [ ] scalar types
  * [ ] union types, '|' character
  * [ ] input types
  * [ ] declaration order swapping
  * [ ] whatever comes up that I haven't seen

Caching the resulting schema object is probably a good practice GraphQL object as I made no effort of any premature optimisation.

## Usage

`schema.MustBuildSchema` requires two arguments:
- schema as string
- `map[string]graphql.FieldResolveFn` for resolver mappings

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

Type declarations and arrays:

```go
package main

import (
	"fmt"
	"strconv"

	"github.com/graphql-go/graphql"
	schema "github.com/shoobyban/graphql-schema"
)

func main() {
	params := graphql.Params{
		Schema: schema.MustBuildSchema(`
			type Starship {
				name: String
    		        }

			type Human {
				name: String
				appearsIn: [String]
				starships: [Starship]
			}

			type Query {
				human(id: ID): Human
			}			  
			`, map[string]graphql.FieldResolveFn{
			"human": func(p graphql.ResolveParams) (interface{}, error) {
				type starship struct {
					Name string
				}
				type human struct {
					Name      string
					AppearsIn []string
					Starships []starship
				}
				id, _ := strconv.Atoi(p.Args["id"].(string))
				humans := map[int]human{
					1002: {
						Name:      "Han Solo",
						AppearsIn: []string{"NEWHOPE", "EMPIRE", "JEDI"},
						Starships: []starship{{Name: "Millenium Falcon"}, {Name: "Imperial shuttle"}},
					},
				}
				return humans[id], nil
			},
		}),
		RequestString: `
		query {
			human(id: 1002) {
				name
				appearsIn
				starships {
					name
				}
			}
		}
		`,
	}
	result := graphql.Do(params)
	fmt.Printf("%#v\n", result)
}
```
