package schema

import (
	"fmt"
	"testing"
)

type lexTest struct {
	name  string
	input string
	items []item
}

// Make the types prettyprint.
var itemName = map[token]string{
	itemError:       "error",
	itemEOF:         "EOF",
	itemColon:       ":",
	itemComma:       ",",
	itemBlockStart:  "{",
	itemBlockEnd:    "}",
	itemStringValue: "string",
	itemType:        "type",
}

var (
	tBlockStart = mkItem(itemBlockStart, "")
	tBlockEnd   = mkItem(itemBlockEnd, "")
	tEOF        = mkItem(itemEOF, "")
	tColon      = mkItem(itemColon, ":")
	tComma      = mkItem(itemComma, ",")
	tLpar       = mkItem(itemLeftParen, "(")
	tRpar       = mkItem(itemRightParen, ")")
	tType       = mkItem(itemType, "type")
	comment     = "# comment"
)

func (i token) String() string {
	s := itemName[i]
	if s == "" {
		return fmt.Sprintf("item%d", int(i))
	}
	return s
}

func mkItem(typ token, text string) item {
	return item{
		typ: typ,
		val: text,
	}
}

var lexTests = []lexTest{
	{"empty", "", []item{tEOF}},
	{"comment only", "# comment", []item{tEOF}},
	{"2 lines comment only", "# comment\n# comment line 2", []item{tEOF}},
	{"in block string", `
		# comment1
		type User {
			name: String
			"""
			Name
			"""
		}
		# comment2
		`,
		[]item{tType,
			mkItem(itemIdentifier, "User"),
			tBlockStart,
			mkItem(itemIdentifier, "name"),
			mkItem(itemColon, ":"),
			mkItem(itemIdentifier, "String"),
			mkItem(itemStringValue, "\n\t\t\tName\n\t\t\t"),
			tBlockEnd,
			tEOF,
		},
	},
	{"in parenthesis string", `
		# comment1
		type Query {
			some(a: Int, b: String): String
			"""
			Some function
			"""
		}
		# comment2
		`,
		[]item{tType,
			mkItem(itemIdentifier, "Query"),
			tBlockStart,
			mkItem(itemIdentifier, "some"),
			tLpar,
			mkItem(itemIdentifier, "a"),
			mkItem(itemColon, ":"),
			mkItem(itemIdentifier, "Int"),
			tComma,
			mkItem(itemIdentifier, "b"),
			mkItem(itemColon, ":"),
			mkItem(itemIdentifier, "String"),
			tRpar,
			mkItem(itemColon, ":"),
			mkItem(itemIdentifier, "String"),
			mkItem(itemStringValue, "\n\t\t\tSome function\n\t\t\t"),
			tBlockEnd,
			tEOF,
		},
	},
	{
		"union pipes",
		`union SearchResult = Photo | Person

		type Person {
			name: String
			age: Int
		}

		type Photo {
			height: Int
			width: Int
		}

		type SearchQuery {
			firstSearchResult: SearchResult
		}`,
		[]item{
			mkItem(itemUnion, "union"),
			mkItem(itemIdentifier, "SearchResult"),
			mkItem(itemEqual, "="),
			mkItem(itemIdentifier, "Photo"),
			mkItem(itemPipe, "|"),
			mkItem(itemIdentifier, "Person"),
			mkItem(itemUnionEnd, "\n"),
			tType,
			mkItem(itemIdentifier, "Person"),
			tBlockStart,
			mkItem(itemIdentifier, "name"),
			mkItem(itemColon, ":"),
			mkItem(itemIdentifier, "String"),
			mkItem(itemIdentifier, "age"),
			mkItem(itemColon, ":"),
			mkItem(itemIdentifier, "Int"),
			tBlockEnd,
			tType,
			mkItem(itemIdentifier, "Photo"),
			tBlockStart,
			mkItem(itemIdentifier, "height"),
			mkItem(itemColon, ":"),
			mkItem(itemIdentifier, "Int"),
			mkItem(itemIdentifier, "width"),
			mkItem(itemColon, ":"),
			mkItem(itemIdentifier, "Int"),
			tBlockEnd,
			tType,
			mkItem(itemIdentifier, "SearchQuery"),
			tBlockStart,
			mkItem(itemIdentifier, "firstSearchResult"),
			mkItem(itemColon, ":"),
			mkItem(itemIdentifier, "SearchResult"),
			tBlockEnd,
			tEOF,
		},
	},
}

func TestLex(t *testing.T) {
	for _, test := range lexTests {
		items := collect(&test)
		if !equal(items, test.items, false) {
			t.Errorf("%s: got\n\t%+v\nexpected\n\t%v", test.name, items, test.items)
		}
	}
}

// collect gathers the emitted items into a slice.
func collect(t *lexTest) (items []item) {
	l := lex(t.name, t.input)
	for {
		item := l.nextItem()
		items = append(items, item)
		if item.typ == itemEOF || item.typ == itemError {
			break
		}
	}
	return
}

func equal(i1, i2 []item, checkPos bool) bool {
	if len(i1) != len(i2) {
		return false
	}
	for k := range i1 {
		if i1[k].typ != i2[k].typ {
			return false
		}
		if i1[k].val != i2[k].val {
			fmt.Printf("Val not eq: %#v %#v\n", i1[k].val, i2[k].val)
			return false
		}
		if checkPos && i1[k].pos != i2[k].pos {
			return false
		}
		if checkPos && i1[k].line != i2[k].line {
			return false
		}
	}
	return true
}
