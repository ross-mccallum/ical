package ical

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type Calendar struct {
	Prodid     string
	Version    string
	Calscale   string
	Method     string
	Properties []*Property
	Events     []*Event
}

type Event struct {
	UID         string
	Timestamp   time.Time
	StartDate   time.Time
	EndDate     time.Time
	Summary     string
	Description string
	Properties  []*Property
	Alarms      []*Alarm
}

type Alarm struct {
	Action     string
	Trigger    string
	Properties []*Property
}

type Property struct {
	Name   string
	Value  string
	Params map[string]*Param
}

type Param struct {
	Values []string
}

type parser struct {
	lex       *lexer
	token     [2]item
	peekCount int
	scope     int
	c         *Calendar
	v         *Event
	a         *Alarm
	location  *time.Location
}

// Parse transforms the raw iCalendar into a Calendar struct.
// It's up to the caller to close the io.Reader.
// If the time.Location parameter is not set it will default to the system location.
func Parse(r io.Reader, l *time.Location) (*Calendar, error) {
	p := &parser{}
	p.c = NewCalendar()
	p.scope = scopeCalendar
	bytes, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if l == nil {
		l = time.Local
	}
	p.location = l
	text := unfold(string(bytes))
	p.lex = lex("ical1", text)
	return p.parse()
}

// NewCalendar creates an empty calendar.
func NewCalendar() *Calendar {
	c := &Calendar{Calscale: "GREGORIAN"}
	c.Properties = make([]*Property, 0)
	c.Events = make([]*Event, 0)
	return c
}

// NewProprty creates an empty Property.
func NewProperty() *Property {
	p := &Property{}
	p.Params = make(map[string]*Param)
	return p
}

// NewEvent creates an empty Event.
func NewEvent() *Event {
	v := &Event{}
	v.Properties = make([]*Property, 0)
	v.Alarms = make([]*Alarm, 0)
	return v
}

// NewAlarm creates an empty Alarm.
func NewAlarm() *Alarm {
	a := &Alarm{}
	a.Properties = make([]*Property, 0)
	return a
}

// NewParam creates an empty Param.
func NewParam() *Param {
	p := &Param{}
	p.Values = make([]string, 0)
	return p
}

// unfold converts multi line values into a sinle line.
func unfold(text string) string {
	return strings.Replace(text, "\r\n ", "", -1)
}

// The parser method next returns the next token.
func (p *parser) next() item {
	if p.peekCount > 0 {
		p.peekCount--
	} else {
		p.token[0] = p.lex.nextItem()
	}
	//fmt.Printf("nextItem: %d -- %s\n", p.peekCount, p.token[0])
	//fmt.Printf("--------: %s\n", p.token[1])
	//fmt.Printf("xxxxxxxx: %s\n", p.lex.input[p.lex.start:10])
	return p.token[p.peekCount]
}

// The backup method backs the input stream up one token.
func (p *parser) backup() {
	p.peekCount++
}

// The peek method returns, but does not consumme, the next token.
/*
func (p *parser) peek() item {
	if p.peekCount > 0 {
		return p.token[p.peekCount-1]
	}
	p.peekCount = 1
	p.token[0] = p.lex.nextItem()
	return p.token[0]
}
*/

// Switch scope between Calendar, Event, and Alarm.
func (p *parser) enterScope() {
	p.scope++
}

// Return to previous scope.
func (p *parser) leaveScope() {
	p.scope--
}

var errorDone = errors.New("Done")

const (
	scopeCalendar int = iota
	scopeEvent
	scopeAlarm
)

// Parse the input
func (p *parser) parse() (*Calendar, error) {
	if item := p.next(); item.typ != itemBeginVCalendar {
		return nil, fmt.Errorf("found %s, expected BEGIN:VCALENDAR", item)
	}
	if item := p.next(); item.typ != itemLineEnd {
		return nil, fmt.Errorf("found %s, expected CRLF", item)
	}
	for {
		err := p.scanContentLine()
		if err == errorDone {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return p.c, nil
}

// scanDelimiter switches scope and validates related component
func (p *parser) scanDelimiter(delimiter item) error {
	switch delimiter.typ {
	case itemBeginVEvent:
		if err := p.validateCalendar(p.c); err != nil {
			return err
		}
		p.v = NewEvent()
		p.enterScope()
		if item := p.next(); item.typ != itemLineEnd {
			return fmt.Errorf("found %s, expected CRLF", item)
		}
	case itemEndVEvent:
		if p.scope > scopeEvent {
			return fmt.Errorf("found %s, expected END:VALARM", delimiter)
		}
		if err := p.validateEvent(p.v); err != nil {
			return err
		}
		p.c.Events = append(p.c.Events, p.v)
		p.leaveScope()
		if item := p.next(); item.typ != itemLineEnd {
			return fmt.Errorf("found %s, expected CRLF", item)
		}
	case itemBeginVAlarm:
		p.a = NewAlarm()
		p.enterScope()
		if item := p.next(); item.typ != itemLineEnd {
			return fmt.Errorf("found %s, expected CRLF", item)
		}
	case itemEndVAlarm:
		if err := p.validateAlarm(p.a); err != nil {
			return err
		}
		p.v.Alarms = append(p.v.Alarms, p.a)
		p.leaveScope()
		if item := p.next(); item.typ != itemLineEnd {
			return fmt.Errorf("found %s, expected CRLF", item)
		}
	case itemEndVCalendar:
		if p.scope > scopeCalendar {
			return fmt.Errorf("found %s, expected END:VEVENT", delimiter)
		}
		return errorDone
	}
	return nil
}

// scanContentLine parses a calendar content line
func (p *parser) scanContentLine() error {
	name := p.next()
	if name.typ > itemKeyword {
		if err := p.scanDelimiter(name); err != nil {
			return err
		}
		return p.scanContentLine()
	}
	if !isItemComponent(name) {
		return fmt.Errorf("found %s, expected a \"component\" token", name)
	}
	prop := NewProperty()
	prop.Name = name.val
	if err := p.scanParams(prop); err != nil {
		return err
	}
	if item := p.next(); item.typ != itemColon {
		return fmt.Errorf("found %s, expected \":\"", item)
	}
	// Property value.
	value := p.next()
	if value.typ != itemValue {
		return fmt.Errorf("found %s, expected a value", value)
	}
	prop.Value = value.val
	// End of line
	if item := p.next(); item.typ != itemLineEnd {
		return fmt.Errorf("found %s, expected CRLF", item)
	}

	switch p.scope {
	case scopeCalendar:
		p.c.Properties = append(p.c.Properties, prop)
	case scopeEvent:
		p.v.Properties = append(p.v.Properties, prop)
	case scopeAlarm:
		p.a.Properties = append(p.a.Properties, prop)
	}

	return nil
}

// scanParams parses a list of parameters inside a content line
func (p *parser) scanParams(prop *Property) error {
	for {
		var item item
		if item = p.next(); item.typ != itemSemiColon {
			p.backup()
			return nil
		}
		if item = p.next(); item.typ != itemParamName {
			return fmt.Errorf("found %s, expected a parameter name", item)
		}
		if item = p.next(); item.typ != itemEqual {
			return fmt.Errorf("found %s, expected =", item)
		}
		param := NewParam()
		if err := p.scanValues(param); err != nil {
			return err
		}
		prop.Params[item.val] = param
	}
}

// scanValues scans a list of one or more parameter values
func (p *parser) scanValues(param *Param) error {
	for {
		var item item
		if item = p.next(); item.typ != itemParamValue {
			return fmt.Errorf("found %s, expected a parameter value", item)
		}
		param.Values = append(param.Values, item.val)
		if item = p.next(); item.typ != itemComma {
			p.backup()
			return nil
		}
	}
}

// validateCalendar validates the calendar properties
func (p *parser) validateCalendar(c *Calendar) error {
	propertyCount := 0
	for _, property := range c.Properties {
		if property.Name == "PRODID" {
			c.Prodid = property.Value
			propertyCount++
		}
		if property.Name == "VERSION" {
			c.Version = property.Value
			propertyCount++
		}
		if property.Name == "CALSCALE" {
			c.Calscale = property.Value
		}
		if property.Name == "METHOD" {
			c.Method = property.Value
		}
	}
	if propertyCount != 2 {
		return fmt.Errorf("missing required property \"prodid or version\"")
	}
	return nil
}

// validateEvent validate the properties of an event
func (p *parser) validateEvent(v *Event) error {
	propertyCount := make(map[string]int)
	for _, property := range v.Properties {
		switch property.Name {
		case "UID":
			v.UID = property.Value
			propertyCount["UID"]++
		case "DTSTAMP":
			v.Timestamp, _ = parseDate(property, p.location)
			propertyCount["DTSTAMP"]++
		case "DTSTART":
			v.StartDate, _ = parseDate(property, p.location)
			propertyCount["DTSTART"]++
		case "DTEND":
			if hasProperty("DURATION", v.Properties) {
				return fmt.Errorf("cannot have both \"DTEND\" and \"DURATION\"")
			}
			v.EndDate, _ = parseDate(property, p.location)
			propertyCount["DTEND"]++
		case "DURATION":
			if hasProperty("DTEND", v.Properties) {
				return fmt.Errorf("cannot have both \"DTEND\" and \"DURATION\"")
			}
			propertyCount["DURATION"]++
		case "SUMMARY":
			v.Summary = property.Value
			propertyCount["SUMMARY"]++
		case "DESCRIPTION":
			v.Description = property.Value
			propertyCount["DESCRIPTION"]++
		}
	}
	if p.c.Method == "" && v.Timestamp.IsZero() {
		return fmt.Errorf("missing required property \"DTSTAMP\"")
	}
	if v.UID == "" {
		return fmt.Errorf("missing required property \"UID\"")
	}
	if v.StartDate.IsZero() {
		return fmt.Errorf("missing required property \"DTSTART\"")
	}
	for key, val := range propertyCount {
		if val > 1 {
			return fmt.Errorf("\"%s\" property occurs more than once", key)
		}
	}
	if !hasProperty("DTEND", v.Properties) {
		v.EndDate = v.StartDate.Add(time.Hour * 24)
	}
	return nil
}

// validateAlarm validate the proprties of an alarm
func (p *parser) validateAlarm(a *Alarm) error {
	propertyCount := make(map[string]int)
	for _, property := range a.Properties {
		switch property.Name {
		case "ACTION":
			a.Action = property.Value
			propertyCount["ACTION"]++
		case "TRIGGER":
			a.Trigger = property.Value
			propertyCount["TRIGGER"]++
		}
	}
	for key, val := range propertyCount {
		if val < 1 {
			return fmt.Errorf("missing required property \"%s\"", key)
		}
		if val > 1 {
			return fmt.Errorf("\"%s\" property occurs more than once", key)
		}
	}
	return nil
}

// hasProperty checks if a component has a property
func hasProperty(name string, properties []*Property) bool {
	for _, property := range properties {
		if name == property.Name {
			return true
		}
	}
	return false
}

const (
	dateLayout              = "20060102"
	dateTimeLayoutUTC       = "20060102T150405Z"
	dateTimeLayoutLocalized = "20060102T150405"
)

// parseDate transforms an ical date property into a time.Time object
func parseDate(p *Property, l *time.Location) (time.Time, error) {
	if strings.HasSuffix(p.Value, "Z") {
		return time.Parse(dateTimeLayoutUTC, p.Value)
	}
	if tz, ok := p.Params["TZID"]; ok {
		loc, err := time.LoadLocation(tz.Values[0])
		if err != nil {
			loc = time.UTC
		}
		return time.ParseInLocation(dateTimeLayoutLocalized, p.Value, loc)
	}
	if len(p.Value) == 8 {
		return time.Parse(dateLayout, p.Value)
	}
	layout := dateTimeLayoutLocalized
	if val, ok := p.Params["VALUE"]; ok {
		if val.Values[0] == "DATE" {
			if len(p.Value) == 8 {
				layout = dateLayout
			}
		}
	}
	return time.ParseInLocation(layout, p.Value, l)
}
