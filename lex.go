// Package schema is a GraphQL Schema declaration format lexer and parser
package schema

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

type pos int

// token type for parser
type token int

// item represents a token or text string returned from the scanner.
type item struct {
	typ  token  // The type of this item.
	pos  pos    // The starting position, in bytes, of this item in the input string.
	val  string // The value of this item.
	line int    // The line number at the start of this item.
}

func (i item) String() string {
	switch {
	case i.typ == itemIdentifier:
		return "Identifier:" + fmt.Sprintf("%q", i.val)
	case i.typ == itemEOF:
		return "EOF"
	case i.typ == itemError:
		return i.val
	case i.typ > itemKeyword:
		return fmt.Sprintf("<%s>", i.val)
	case i.typ == itemBlockStart:
		return "BS:{"
	case i.typ == itemBlockEnd:
		return "BE:}"
	case i.typ == itemColon:
		return "Colon:':'"
	case i.typ == itemComma:
		return "Comma:','"
	case len(i.val) > 10:
		return fmt.Sprintf("%.10q...", i.val)
	}
	return fmt.Sprintf("'%q'", i.val)
}

const (
	// Special tokens
	itemError        token = iota
	itemColon              // :
	itemComma              // ,
	itemEqual              // =
	itemExclamation        // !
	itemPipe               // |
	itemEOF                // EOF
	itemIdentifier         // alphanumeric (plus '_') identifier
	itemLeftParen          // '('
	itemRightParen         // ')'
	itemLeftBracket        // '['
	itemRightBracket       // ']'
	itemSpace              // run of spaces separating arguments
	itemStringValue        // String value enclosed by """ and """

	itemBlockStart // Definition block start
	itemBlockEnd   // Definition block end
	itemUnionEnd   // Union definition's end

	// Keywords after this
	itemKeyword // used only to delimit the keywords

	itemSchema     // Root Schema declaration
	itemType       // type keyword
	itemEnum       // enum keyword
	itemUnion      // union keyword
	itemInterface  // interface keyword
	itemScalar     // scalar keyword
	itemInput      // input keyword
	itemImplements // implements keyword
)

// LexNames is used for debugging
var LexNames = map[token]string{
	itemError:        "Error",
	itemColon:        ":",
	itemComma:        ",",
	itemEqual:        "=",
	itemPipe:         "|",
	itemExclamation:  "!",
	itemEOF:          "EOF",
	itemIdentifier:   "identifier",
	itemLeftParen:    "(",
	itemRightParen:   ")",
	itemLeftBracket:  "[",
	itemRightBracket: "]",
	itemSpace:        "space",
	itemStringValue:  "String Value",

	itemBlockStart: "block start",
	itemBlockEnd:   "block end",
	itemUnionEnd:   "union end",

	// Keywords after this
	itemKeyword: "keyword",

	itemSchema:     "schema",
	itemType:       "type",
	itemEnum:       "enum",
	itemUnion:      "union",
	itemInterface:  "interface",
	itemScalar:     "scalar",
	itemInput:      "input",
	itemImplements: "implements",
}

var key = map[string]token{
	"schema":     itemSchema,
	"type":       itemType,
	"enum":       itemEnum,
	"union":      itemUnion,
	"interface":  itemInterface,
	"scalar":     itemScalar,
	"input":      itemInput,
	"implements": itemImplements,
	"{":          itemBlockStart,
	"}":          itemBlockEnd,
}

const eof = -1

// stateFn represents the state of the scanner as a function that returns the next state.
type stateFn func(*lexer) stateFn

// lexer holds the state of the scanner.
type lexer struct {
	name       string    // the name of the input; used only for error reports
	input      string    // the string being scanned
	pos        pos       // current position in the input
	start      pos       // start position of this item
	width      pos       // width of last rune read from input
	items      chan item // channel of scanned items
	parenDepth int       // nesting depth of ( ) exprs
	line       int       // 1+number of newlines seen
	fnStack    []stateFn // Stack for stateFn to know where to return
}

func (l *lexer) push(f stateFn) {
	ln := len(l.fnStack)
	if ln > 0 && reflect.ValueOf(l.fnStack[ln-1]) == reflect.ValueOf(f) {
		return
	}
	l.fnStack = append(l.fnStack, f)
}

func (l *lexer) pop() stateFn {
	ln := len(l.fnStack)
	if ln == 0 {
		return nil
	}
	last := l.fnStack[ln-1]
	l.fnStack = l.fnStack[:ln-1]
	return last
}

func (l *lexer) last() stateFn {
	ln := len(l.fnStack)
	return l.fnStack[ln-1]
}

// next returns the next rune in the input.
func (l *lexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = pos(w)
	l.pos += l.width
	if r == '\n' {
		l.line++
	}
	return r
}

// peek returns but does not consume the next rune in the input.
func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// backup steps back one rune. Can only be called once per call of next.
func (l *lexer) backup() {
	l.pos -= l.width
	// Correct newline count.
	if l.width == 1 && l.input[l.pos] == '\n' {
		l.line--
	}
}

// emit passes an item back to the client.
func (l *lexer) emit(t token) {
	l.items <- item{t, l.start, l.input[l.start:l.pos], l.line}
	// Some items contain text internally. If so, count their newlines.
	switch t {
	case itemStringValue:
		l.line += strings.Count(l.input[l.start:l.pos], "\n")
	}
	l.start = l.pos
}

// ignore skips over the pending input before this point.
func (l *lexer) ignore() {
	l.line += strings.Count(l.input[l.start:l.pos], "\n")
	l.start = l.pos
}

// errorf returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.nextItem.
func (l *lexer) errorf(format string, args ...interface{}) stateFn {
	l.items <- item{itemError, l.start, fmt.Sprintf(format, args...), l.line}
	return nil
}

// nextItem returns the next item from the input.
// Called by the parser, not in the lexing goroutine.
func (l *lexer) nextItem() item {
	return <-l.items
}

// drain drains the output so the lexing goroutine will exit.
// Called by the parser, not in the lexing goroutine.
func (l *lexer) drain() {
	for range l.items {
	}
}

// lex creates a new scanner for the input string.
func lex(name, input string) *lexer {
	l := &lexer{
		name:  name,
		input: input,
		items: make(chan item),
		line:  1,
	}
	go l.run()
	return l
}

// run runs the state machine for the lexer.
func (l *lexer) run() {
	for state := lexSchema; state != nil; {
		state = state(l)
	}
	close(l.items)
}

// state functions

// Utility functions

// isSpace reports whether r is a space character.
func isSpace(r rune) bool {
	return r == ' ' || r == '\t'
}

// isEndOfLine reports whether r is an end-of-line character.
func isEndOfLine(r rune) bool {
	return r == '\r' || r == '\n'
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// atTerminator reports whether the input is at valid termination character to
// appear after an identifier. Breaks .X.Y into two pieces. Also catches cases
// like "$x+2" not being acceptable without a space, in case we decide one
// day to implement arithmetic.
func (l *lexer) atTerminator() bool {
	r := l.peek()
	if isSpace(r) || isEndOfLine(r) {
		return true
	}
	switch r {
	case eof, ':', ')', '(', ',', ']', '!', '|':
		return true
	}
	return false
}

// Lex functions

// lexSchema is outside of any definition, default state
func lexSchema(l *lexer) stateFn {
	l.push(lexSchema)
	switch r := l.next(); {
	case r == eof:
		l.emit(itemEOF)
		return nil // EOF
	case isSpace(r) || isEndOfLine(r):
		l.ignore()
		return lexSchema // no need to handle spaces
	case r == '#':
		return lexComment
	case r == '{':
		l.ignore()
		l.emit(itemBlockStart)
		return lexBlock
	case isAlphaNumeric(r):
		l.backup()
		return lexIdentifier
	default:
		return l.errorf("unrecognized character in action: %#U at pos %v", r, l.pos)
	}
}

// lexComment scans a comment. The left comment marker is known to be present.
func lexComment(l *lexer) stateFn {
	for {
		switch r := l.next(); {
		case r == eof || isEndOfLine(r):
			return l.last()
		}
		l.ignore()
	}
}

func lexUnion(l *lexer) stateFn {
	l.push(lexUnion)
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r):
			l.backup()
			return lexIdentifier
		case r == '=':
			l.emit(itemEqual)
		case r == '|':
			l.emit(itemPipe)
		case r == eof || isEndOfLine(r):
			l.emit(itemUnionEnd)
			l.pop()
			return l.last()
		}
		l.ignore()
	}
}

// lexIdentifier scans an alphanumeric.
func lexIdentifier(l *lexer) stateFn {
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r):
			// absorb.
		default:
			l.backup()
			word := l.input[l.start:l.pos]
			if !l.atTerminator() {
				return l.errorf("bad character %#U", r)
			}
			switch {
			case word == "union":
				l.emit(key[word])
				return lexUnion
			case key[word] > itemKeyword:
				l.emit(key[word])
			default:
				l.emit(itemIdentifier)
			}
			return l.last()
		}
	}
}

func lexArgs(l *lexer) stateFn {
	l.push(lexArgs)
	startLine := l.line
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r):
			l.backup()
			return lexIdentifier
		case isSpace(r):
			l.ignore()
		case r == '#':
			return lexComment
		case r == eof:
			// Restore line number to location of opening quote.
			// We will error out so it's ok just to overwrite the field.
			l.line = startLine
			return l.errorf("unterminated arguments block")
		case r == '\n':
			l.ignore()
		case r == ':':
			l.emit(itemColon)
		case r == ',':
			l.emit(itemComma)
		case r == '!':
			l.emit(itemExclamation)
		case r == '=':
			l.emit(itemEqual)
		case r == '[':
			l.emit(itemLeftBracket)
			l.parenDepth++
		case r == ']':
			l.emit(itemRightBracket)
			l.parenDepth--
			if l.parenDepth < 0 {
				return l.errorf("unexpected right bracket %#U", r)
			}
		case r == ')':
			l.emit(itemRightParen)
			l.pop()
			return l.last()
		}
	}
}

func lexBlock(l *lexer) stateFn {
	l.push(lexBlock)
	startLine := l.line
	for {
		if strings.HasPrefix(l.input[l.pos:], "\"\"\"") {
			return lexStringValue
		}
		switch r := l.next(); {
		case isAlphaNumeric(r):
			l.backup()
			return lexIdentifier
		case isSpace(r):
			l.ignore()
		case r == '#':
			return lexComment
		case r == eof:
			// Restore line number to location of opening quote.
			// We will error out so it's ok just to overwrite the field.
			l.line = startLine
			return l.errorf("unterminated block")
		case r == '\n':
			l.ignore()
		case r == ':':
			l.emit(itemColon)
		case r == '!':
			l.emit(itemExclamation)
		case r == '=':
			l.emit(itemEqual)
		case r == '(':
			l.emit(itemLeftParen)
			return lexArgs
		case r == '[':
			l.emit(itemLeftBracket)
			l.parenDepth++
		case r == ']':
			l.emit(itemRightBracket)
			l.parenDepth--
			if l.parenDepth < 0 {
				return l.errorf("unexpected right bracket %#U", r)
			}
		case r == '}':
			l.ignore()
			l.emit(itemBlockEnd)
			l.pop()
			return l.last()
		}
	}
}

// lexStringValue scans a string value. The left """ marker is known to be present.
func lexStringValue(l *lexer) stateFn {
	l.pos += pos(3) // Length of """
	l.ignore()
	i := strings.Index(l.input[l.pos:], "\"\"\"")
	if i < 0 {
		return l.errorf("unclosed string at pos %#v", l.pos)
	}
	l.pos += pos(i)
	l.emit(itemStringValue)
	l.pos += pos(3)
	return l.last()
}
