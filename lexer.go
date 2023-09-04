package ical

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type itemType int

const (
	// Special tokens
	itemError itemType = iota
	itemEOF
	itemLineEnd

	// Propertiies
	itemComponent
	itemParamName
	itemParamValue
	itemValue

	// Punctuation
	itemColon
	itemSemiColon
	itemEqual
	itemComma

	// Keyword
	itemKeyword

	// Delimiters / Components
	itemBeginVCalendar
	itemEndVCalendar
	itemBeginVEvent
	itemEndVEvent
	itemBeginVAlarm
	itemEndVAlarm
)

// item represents a token or text string returned from the scanner.
type item struct {
	typ itemType // Type of this item.
	pos int      // The starting position, in bytes, of this item in the input string.
	val string   // The value of this item.
}

func (i item) String() string {
	switch {
	case i.typ == itemEOF:
		return "EOF"
	case i.typ == itemError:
		return i.val
	case i.typ > itemKeyword:
		return fmt.Sprintf("<%s>", i.val)
	case len(i.val) > 10:
		return fmt.Sprintf("%q", i.val)
	}
	return fmt.Sprintf("%q", i.val)
}

const eof = -1

// stateFn represents the state of the scanner as a function that returns the next state.
type stateFn func(*lexer) stateFn

// lexer holds the state of the scanner
type lexer struct {
	name    string    // Used for error reporting.
	input   string    // The string being scanned.
	start   int       // The start position of this item.
	pos     int       // Currennt position in the input.
	width   int       // Width of the last rune read from the input.
	lastPos int       // Position of the most recent item returned by nextItem.
	state   stateFn   // The next lexing function to enter.
	items   chan item // Channel of scanned items.
}

// Create a new scanner for the input string.
func lex(name, input string) *lexer {
	l := &lexer{
		name:  name,
		input: input,
		items: make(chan item),
	}
	go l.run() // Concurrently run state machine.
	return l
}

// run lexes the input by executing state functions until the state is nil.
func (l *lexer) run() {
	for l.state = lexComponent; l.state != nil; {
		l.state = l.state(l)
	}
	close(l.items) // No more tokens will be delivered.
}

// Pass an item back to the client.
func (l *lexer) emit(t itemType) {
	l.items <- item{t, l.start, l.input[l.start:l.pos]}
	l.start = l.pos
}

// Skip over input before this point
func (l *lexer) ignore() {
	l.start = l.pos
}

// backup steps back one rune. Can be called only once per call of next.
func (l *lexer) backup() {
	l.pos -= l.width
}

// peek returns but does not consume the next rune in the input.
func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// Return the next rune in the input.
func (l *lexer) next() rune {
	if l.pos >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = w
	l.pos += l.width
	return r
}

// errorf returns an error token and terminates the scan by passing back
// a nil pointer that will be the next atate, terminating l.nextItem.
func (l *lexer) errorf(format string, args ...interface{}) stateFn {
	l.items <- item{itemError, l.start, fmt.Sprintf(format, args...)}
	return nil
}

// nextItem returns the next item from the input.
// Called by the parser. It is not part of the lexing goroutine.
func (l *lexer) nextItem() item {
	item := <-l.items
	l.lastPos = item.pos
	fmt.Printf("Returning NEXTITEM: %d -- %s\n", item.pos, item)
	return item
}

// State functions -------------------------------------------------------------

const (
	beginVCalendar = "BEGIN:VCALENDAR"
	endVCalendar   = "END:VCALENDAR"
	beginVEvent    = "BEGIN:VEVENT"
	endVEvent      = "END:VEVENT"
	beginVAlarm    = "BEGIN:VALARM"
	endVAlarm      = "END:VALARM"
	crlf           = "\r\n"
)

// lexComponent - scans the name in the content line.
// BEGIN / END lines (VCALENDAR, VEVENT, VALARM)
func lexComponent(l *lexer) stateFn {
	//fmt.Printf("---stateFn-lexComponent\n")
	if strings.HasPrefix(l.input[l.pos:], beginVCalendar) {
		l.pos += len(beginVCalendar)
		l.emit(itemBeginVCalendar)
		return lexNewLine
	}
	if strings.HasPrefix(l.input[l.pos:], endVCalendar) {
		l.pos += len(endVCalendar)
		l.emit(itemEndVCalendar)
		return lexNewLine
	}
	if strings.HasPrefix(l.input[l.pos:], beginVEvent) {
		l.pos += len(beginVEvent)
		l.emit(itemBeginVEvent)
		return lexNewLine
	}
	if strings.HasPrefix(l.input[l.pos:], endVEvent) {
		l.pos += len(endVEvent)
		l.emit(itemEndVEvent)
		return lexNewLine
	}
	if strings.HasPrefix(l.input[l.pos:], beginVAlarm) {
		l.pos += len(beginVAlarm)
		l.emit(itemBeginVAlarm)
		return lexNewLine
	}
	if strings.HasPrefix(l.input[l.pos:], endVAlarm) {
		l.pos += len(endVAlarm)
		l.emit(itemEndVAlarm)
		return lexNewLine
	}
	for {
		if !isName(l.next()) {
			l.backup()
			l.emit(itemComponent)
			break
		}
	}
	return lexContentLine
}

// lexNewLine scans a line for the crlf
func lexNewLine(l *lexer) stateFn {
	//fmt.Printf("---stateFn-lexNewLine\n")
	if l.peek() == eof {
		return nil
	}
	if !strings.HasPrefix(l.input[l.pos:], crlf) {
		l.errorf("unable to find end of line \"CRLF\"")
	}
	l.pos += len(crlf)
	l.emit(itemLineEnd)
	if l.next() == eof {
		l.emit(itemEOF)
		return nil
	}
	l.backup()
	return lexComponent
}

// lexContentLine
func lexContentLine(l *lexer) stateFn {
	//fmt.Printf("---stateFn-lexContentLine\n")
	switch r := l.next(); {
	case r == ';':
		l.emit(itemSemiColon)
		return lexParamName
	case r == ':':
		l.emit(itemColon)
		return lexValue
	case r == ',':
		l.emit(itemComma)
		return lexParamValue
	default:
		return l.errorf("unrecognized character in action: %#U", r)
	}
}

// lexParamName
func lexParamName(l *lexer) stateFn {
	//fmt.Printf("---stateFn-lexParamName\n")
	for {
		if !isName(l.next()) {
			l.backup()
			l.emit(itemParamName)
			break
		}
	}
	r := l.next()
	if r == '=' {
		l.emit(itemEqual)
		return lexParamValue
	}
	return l.errorf("missing \"=\" after parameter name, got %#U", r)
}

// lexParamValue scans the parameter value in the content line
func lexParamValue(l *lexer) stateFn {
	//fmt.Printf("---stateFn-lexParamValue\n")
	r := l.next()
	if r == '"' {
		l.ignore()
		for {
			r = l.next()
			if !isQSafeChar(r) {
				l.backup()
				l.emit(itemParamValue)
				break
			}
		}
		if r = l.next(); r == '"' {
			l.ignore()
		} else {
			l.errorf("missing closing \" for parameter value")
		}
	} else {
		l.backup()
		for {
			r = l.next()
			if !isSafeChar(r) {
				l.backup()
				l.emit(itemParamValue)
				break
			}
		}
	}
	return lexContentLine
}

// lexValue scans a value in a content line
func lexValue(l *lexer) stateFn {
	//fmt.Printf("---stateFn-lexValue\n")
	for {
		r := l.next()
		if !isValueChar(r) {
			l.backup()
			l.emit(itemValue)
			break
		}
	}
	return lexNewLine
}

// rune helper functions -------------------------------------------------------

func isName(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-'
}

func isQSafeChar(r rune) bool {
	return !unicode.IsControl(r) && r != '"'
}

func isSafeChar(r rune) bool {
	return !unicode.IsControl(r) && r != '"' && r != ';' && r != ':' && r != ','
}

func isValueChar(r rune) bool {
	return r == '\t' || (!unicode.IsControl(r) && utf8.ValidRune(r))
}

// isItemComponent checks if the item is an ical component
func isItemComponent(i item) bool {
	return i.typ == itemComponent
}
