package schema

import (
	"fmt"
	"runtime"

	"github.com/graphql-go/graphql"
)

type parseContext struct {
	lex       *lexer
	funcs     map[string]graphql.FieldResolveFn
	scalars   map[string]graphql.Type
	objects   map[string]*graphql.Object
	unions    map[string]*graphql.Union
	Root      *parseContext
	token     [10]item
	peekCount int
}

// Funcs declared functions
var funcs = map[string]graphql.FieldResolveFn{}

// Scalar types declared by the schema
var builtinscalars = map[string]graphql.Type{
	"ID":      graphql.ID,
	"String":  graphql.String,
	"Float":   graphql.Float,
	"Int":     graphql.Int,
	"Boolean": graphql.Boolean,
}

// Parsing.

// MustBuildSchema is equivalent of buildSchema() in graphql.org example implementation
func MustBuildSchema(schema string, resolvers map[string]graphql.FieldResolveFn) graphql.Schema {
	schemaConfig, _ := BuildSchemaConfig(schema, resolvers)
	newSchema, _ := graphql.NewSchema(schemaConfig)
	return newSchema
}

// BuildSchemaConfig is creating a graphql.SchemaConfig from a given string
func BuildSchemaConfig(schema string, resolvers map[string]graphql.FieldResolveFn) (graphql.SchemaConfig, error) {
	funcs = resolvers
	schemaConfig := graphql.SchemaConfig{}
	t := &parseContext{
		lex:     lex("", schema),
		scalars: builtinscalars,
		objects: map[string]*graphql.Object{},
		unions:  map[string]*graphql.Union{},
	}
	t.backup()
	for {
		n := t.next()
		switch {
		case n.typ == itemEOF:
			return schemaConfig, nil
		case n.typ == itemType:
			t.processTypeNode(&schemaConfig)
		case n.typ == itemUnion:
			t.processUnionNode()
		}
	}

}

// dumpTokens is only used for debugging
func (t *parseContext) dumpTokens() {
	for {
		n := t.next()
		fmt.Printf("t: %#v, v: %#v\n", LexNames[n.typ], n.val)
		if n.typ == itemEOF {
			return
		}
	}
}

func (t *parseContext) processUnionNode() {
	n := t.next()
	if n.typ != itemIdentifier {
		t.errorf("No identifier after union keyword, got t: %#v, v: %#v", LexNames[n.typ], n.val)
	}
	x := t.next()
	if x.typ != itemEqual {
		t.errorf("No '=' sign after union keyword, got t: %#v, v: %#v", LexNames[x.typ], x.val)
	}
	types := []*graphql.Object{}
	lastispipe := false
Loop:
	for {
		x = t.next()
		if x.typ == itemPipe {
			if lastispipe {
				t.errorf("Double | in union")
			}
			lastispipe = true
			continue Loop
		}
		if x.typ == itemUnionEnd {
			if lastispipe {
				t.errorf("Last item is |")
			}
			break Loop
		}
		if x.typ != itemIdentifier {
			t.errorf("No label after block start, got t: %#v, v: %#v", LexNames[x.typ], x.val)
		}
		label := x.val
		if _, ok := t.objects[x.val]; !ok {
			t.errorf("Not declared object type (yet) '%s'", x.val)
		}
		types = append(types, t.objects[label])
		lastispipe = false
	}

	t.unions[n.val] = graphql.NewUnion(
		graphql.UnionConfig{
			Name:  n.val,
			Types: types,
		},
	)
}

func (t *parseContext) processTypeNode(schemaConfig *graphql.SchemaConfig) {
	n := t.next()
	if n.typ != itemIdentifier {
		t.errorf("No identifier after type keyword, got t: %#v, v: %#v", LexNames[n.typ], n.val)
	}
	x := t.next()
	if x.typ != itemBlockStart {
		t.errorf("No block starting after Query, got t: %#v, v: %#v", LexNames[x.typ], x.val)
	}
	fields := graphql.Fields{}
Loop:
	for {
		isArray := false
		var params graphql.FieldConfigArgument
		x = t.next()
		if x.typ == itemBlockEnd {
			break Loop
		}
		if x.typ != itemIdentifier {
			t.errorf("No label after block start, got t: %#v, v: %#v", LexNames[x.typ], x.val)
		}
		label := x.val
		x = t.next()
		if x.typ == itemLeftParen {
			params = t.handleParams()
			x = t.next()
		}
		if x.typ != itemColon {
			t.errorf("No colon or ( after label, t: %#v, v: %#v", LexNames[x.typ], x.val)
		}
		x = t.next()
		if x.typ == itemLeftBracket {
			isArray = true
			x = t.next()
		}
		tname := x.val
		if x.typ != itemIdentifier {
			t.errorf("No type identifier after label, t: %#v, v: %#v", LexNames[x.typ], x.val)
		}
		if isArray {
			x = t.next()
			if x.typ != itemRightBracket {
				t.errorf("No closing ] after identifier, t: %#v, v: %#v", LexNames[x.typ], x.val)
			}
		}
		var vtype graphql.Output

		fields[label] = &graphql.Field{}

		if _, ok := t.scalars[tname]; !ok {
			if _, ok := t.objects[tname]; !ok {
				if _, ok := t.unions[tname]; !ok {
					t.errorf("Not declared scalar,object type or union (yet) '%s'", x.val)
				} else {
					vtype = t.unions[tname]
				}
			} else {
				vtype = t.objects[tname]
			}
		} else {
			vtype = t.scalars[tname]
		}

		if isArray {
			vtype = graphql.NewList(vtype)
		}

		fields[label].Type = vtype

		if params != nil {
			fields[label].Args = params
			params = nil
		}

		if _, ok := funcs[label]; ok {
			fields[label].Resolve = funcs[label]
		}
	}
	if n.val == "Query" {
		schemaConfig.Query = graphql.NewObject(
			graphql.ObjectConfig{
				Name:   "RootQuery",
				Fields: fields,
			},
		)
	} else {
		t.objects[n.val] = graphql.NewObject(
			graphql.ObjectConfig{
				Name:   n.val,
				Fields: fields,
			},
		)
	}
}

func (t *parseContext) handleParams() graphql.FieldConfigArgument {
	args := graphql.FieldConfigArgument{}
	for {
		x := t.next()
		if x.typ != itemIdentifier {
			t.errorf("No label in argument, got t: %#v, v: %#v", LexNames[x.typ], x.val)
		}
		label := x.val
		x = t.next()
		if x.typ != itemColon {
			t.errorf("No colon after label, got t: %#v, v: %#v", LexNames[x.typ], x.val)
		}
		x = t.next()
		var vtype graphql.Output
		if _, ok := t.scalars[x.val]; !ok {
			if _, ok := t.objects[x.val]; !ok {
				if _, ok := t.unions[x.val]; !ok {
					t.errorf("Not declared scalar,object type or union (yet) '%s'", x.val)
				} else {
					vtype = t.unions[x.val]
				}
			} else {
				vtype = t.objects[x.val]
			}
		} else {
			vtype = t.scalars[x.val]
		}
		args[label] = &graphql.ArgumentConfig{
			Type: vtype,
		}
		x = t.next()
		if x.typ == itemRightParen {
			return args
		}
		if x.typ != itemComma {
			t.errorf("No comma, ended arg declaration")
		}
	}
}

// IsEmptyTree reports whether this parseContext (node) is empty of everything but space.
func (t *parseContext) isEmpty() bool {
	if t.Root == nil {
		return false
	}
	return true
}

// errorf formats the error and terminates processing.
func (t *parseContext) errorf(format string, args ...interface{}) {
	t.Root = nil
	format = fmt.Sprintf("schema: %d: %s", t.token[0].line, format)
	panic(fmt.Errorf(format, args...))
}

// error terminates processing.
func (t *parseContext) error(err error) {
	t.errorf("%s", err)
}

// expect consumes the next token and guarantees it has the required type.
func (t *parseContext) expect(expected token, context string) item {
	token := t.nextNonSpace()
	if token.typ != expected {
		t.unexpected(token, context)
	}
	return token
}

// expectOneOf consumes the next token and guarantees it has one of the required types.
func (t *parseContext) expectOneOf(expectedTokens []token, context string) item {
	token := t.nextNonSpace()
	found := false
	var foundItem item
	for _, expected := range expectedTokens {
		if token.typ == expected {
			found = true
			foundItem = token
		}
	}
	if !found {
		t.unexpected(token, context)
	}
	return foundItem
}

// unexpected complains about the token and terminates processing.
func (t *parseContext) unexpected(token item, context string) {
	t.errorf("unexpected %s in %s", token, context)
}

// recover is the handler that turns panics into returns from the top level of Parse.
func (t *parseContext) recover(errp *error) {
	e := recover()
	if e != nil {
		if _, ok := e.(runtime.Error); ok {
			panic(e)
		}
		if t != nil {
			t.lex.drain()
		}
		*errp = e.(error)
	}
}

// next returns the next token.
func (t *parseContext) next() item {
	if t.peekCount > 0 {
		t.peekCount--
	} else {
		t.token[0] = t.lex.nextItem()
	}
	return t.token[t.peekCount]
}

// backup backs the input stream up one token.
func (t *parseContext) backup() {
	t.peekCount++
}

// backup2 backs the input stream up two tokens.
// The zeroth token is already there.
func (t *parseContext) backup2(t1 item) {
	t.token[1] = t1
	t.peekCount = 2
}

// backup3 backs the input stream up three tokens
// The zeroth token is already there.
func (t *parseContext) backup3(t2, t1 item) { // Reverse order: we're pushing back.
	t.token[1] = t1
	t.token[2] = t2
	t.peekCount = 3
}

// peek returns but does not consume the next token.
func (t *parseContext) peek() item {
	fmt.Println(t.token)
	if t.peekCount > 0 {
		return t.token[t.peekCount-1]
	}
	t.peekCount = 1
	t.token[0] = t.lex.nextItem()
	return t.token[0]
}

// nextNonSpace returns the next non-space token.
func (t *parseContext) nextNonSpace() (token item) {
	for {
		token = t.next()
		if token.typ != itemSpace {
			break
		}
	}
	return token
}

// peekNonSpace returns but does not consume the next non-space token.
func (t *parseContext) peekNonSpace() (token item) {
	for {
		token = t.next()
		if token.typ != itemSpace {
			break
		}
	}
	t.backup()
	return token
}
