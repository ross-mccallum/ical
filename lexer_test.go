package ical

import (
	"log"
	"os"
	"testing"
)

func TestLexer(t *testing.T) {
	var data []byte
	data, err := os.ReadFile("icsfiles/example.ics")
	if err != nil {
		log.Fatal(err)
	}

	lexer := lex("Test", string(data))
	for {
		item := lexer.nextItem()
		if item.typ == itemEOF {
			break
		}
		if item.typ == itemError {
			t.Error(item)
		}
	}
}
