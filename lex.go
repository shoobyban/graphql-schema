// Package schema is a GraphQL Schema declaration format lexer and parser
package schema

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Pos int

// item represents a token or text string returned from the scanner.
type item struct {
	typ  Token  // The type of this item.
	pos  Pos    // The starting position, in bytes, of this item in the input string.
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

// Token type for parser
type Token int

const (
	// Special tokens
	itemError        Token = iota
	itemColon              // :
	itemComma              // ,
	itemEqual              // =
	itemExclamation        // !
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
var LexNames = map[Token]string{
	itemError:        "Error",
	itemColon:        ":",
	itemComma:        ",",
	itemEqual:        "=",
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

var key = map[string]Token{
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

var fnStack = []stateFn{}

func push(f stateFn) {
	ln := len(fnStack)
	if ln > 0 && reflect.ValueOf(fnStack[ln-1]) == reflect.ValueOf(f) {
		return
	}
	fnStack = append(fnStack, f)
}

func pop() stateFn {
	ln := len(fnStack)
	if ln == 0 {
		return nil
	}
	last := fnStack[ln-1]
	fnStack = fnStack[:ln-1]
	return last
}

func last() stateFn {
	ln := len(fnStack)
	return fnStack[ln-1]
}

// lexer holds the state of the scanner.
type lexer struct {
	name       string    // the name of the input; used only for error reports
	input      string    // the string being scanned
	pos        Pos       // current position in the input
	start      Pos       // start position of this item
	width      Pos       // width of last rune read from input
	items      chan item // channel of scanned items
	parenDepth int       // nesting depth of ( ) exprs
	line       int       // 1+number of newlines seen
}

// next returns the next rune in the input.
func (l *lexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = Pos(w)
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
func (l *lexer) emit(t Token) {
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

// accept consumes the next rune if it's from the valid set.
func (l *lexer) accept(valid string) bool {
	if strings.ContainsRune(valid, l.next()) {
		return true
	}
	l.backup()
	return false
}

// acceptRun consumes a run of runes from the valid set.
func (l *lexer) acceptRun(valid string) {
	for strings.ContainsRune(valid, l.next()) {
	}
	l.backup()
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
	case eof, ':', ')', '(', ',', ']':
		return true
	}
	return false
}

// Lex functions

// lexSchema is outside of any definition, default state
func lexSchema(l *lexer) stateFn {
	push(lexSchema)
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
			return last()
		}
		l.ignore()
	}
}

// lexIdentifier scans an alphanumeric.
func lexIdentifier(l *lexer) stateFn {
Loop:
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
			case key[word] > itemKeyword:
				l.emit(key[word])
			default:
				l.emit(itemIdentifier)
			}
			break Loop
		}
	}
	return last()
}

func lexProperty(l *lexer) stateFn {
	push(lexProperty)
	startPos := l.pos
	for {
		switch r := l.next(); {
		case isSpace(r):
			l.ignore()
		case isAlphaNumeric(r):
			l.backup()
			return lexIdentifier
		case r == '\n' || r == '}':
			if l.pos == startPos {
				return l.errorf("no value at %v", l.pos)
			}
			l.ignore()
			pop()
			return last()
		default:
			return l.errorf("unterminated property %v", string(r))
		}
	}
}

func lexArgs(l *lexer) stateFn {
	push(lexArgs)
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
			pop()
			return last()
		}
	}
}

func lexBlock(l *lexer) stateFn {
	push(lexBlock)
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
			pop()
			return last()
		}
	}
}

// lexStringValue scans a string value. The left """ marker is known to be present.
func lexStringValue(l *lexer) stateFn {
	l.pos += Pos(3) // Length of """
	l.ignore()
	i := strings.Index(l.input[l.pos:], "\"\"\"")
	if i < 0 {
		return l.errorf("unclosed string at pos %#v", l.pos)
	}
	l.pos += Pos(i)
	l.emit(itemStringValue)
	l.pos += Pos(3)
	return last()
}
