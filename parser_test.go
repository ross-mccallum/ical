package ical

import (
	"fmt"
	"os"
	"testing"
)

var calendarList = []string{
	"icsfiles/AirBnB.google.com.ics",
	//"icsfiles/example.ics",
	//"icsfiles/google_example.ics",
	//"icsfiles/facebookbirthday.ics",
	//"icsfiles/malformed-date.ics",
	//"icsfiles/with-alarm.ics",
}

func TestParse(t *testing.T) {
	for _, filename := range calendarList {
		file, _ := os.Open(filename)
		_, err := Parse(file, nil)
		file.Close()
		if err != nil {
			t.Error(fmt.Errorf("%v on '%s'", err, filename))
		}
	}
}
