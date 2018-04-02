package schema

import (
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/testutil"
)

var debug = flag.Bool("debug", false, "show the errors produced by the main tests")

type parseTest struct {
	name   string
	input  string
	ok     bool
	result graphql.SchemaConfig // what the user would see in an error message.
}

const (
	noError  = true
	hasError = false
)

type T struct {
	Name     string
	Query    string
	Schema   graphql.Schema
	Expected interface{}
}

func TestGraphql(t *testing.T) {
	for _, test := range Tests {
		params := graphql.Params{
			Schema:        test.Schema,
			RequestString: test.Query,
		}
		result := graphql.Do(params)
		if len(result.Errors) > 0 {
			t.Fatalf("%s: wrong result, unexpected errors: %v", test.Name, result.Errors)
		}
		if !reflect.DeepEqual(result, test.Expected) {
			t.Fatalf("%s: wrong result, query: %v, graphql result diff: %v", test.Name, test.Query, testutil.Diff(test.Expected, result))
		}
	}
}

var Tests = []T{
	{
		Name: "hello",
		Query: `
			query { hello }
		`,
		Schema: MustBuildSchema(`
			type Query {
				hello: String
			}
		`, map[string]graphql.FieldResolveFn{
			"hello": func(p graphql.ResolveParams) (interface{}, error) {
				return "world", nil
			},
		}),
		Expected: &graphql.Result{
			Data: map[string]interface{}{
				"hello": "world",
			},
		},
	},
	{
		Name: "get hex",
		Query: `
			query { hex(r: 0, g: 0, b: 0) }
		`,
		Schema: MustBuildSchema(`
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
		Expected: &graphql.Result{
			Data: map[string]interface{}{"hex": "#000000"},
		},
	},
	{
		Name: "starwars single",
		Query: `
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
		Schema: MustBuildSchema(`
			type Starship {
				name: String
    		}

			type Human {
				name: String
				appearsIn: String
				starships: Starship
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
					AppearsIn string
					Starships starship
				}
				id, _ := strconv.Atoi(p.Args["id"].(string))
				humans := map[int]human{
					1002: {
						Name:      "Han Solo",
						AppearsIn: "NEWHOPE",
						Starships: starship{Name: "Millenium Falcon"},
					},
				}
				return humans[id], nil
			},
		}),
		Expected: &graphql.Result{
			Data: map[string]interface{}{
				"human": map[string]interface{}{
					"name":      "Han Solo",
					"appearsIn": "NEWHOPE",
					"starships": map[string]interface{}{
						"name": "Millenium Falcon"},
				},
			},
		},
	},
}
