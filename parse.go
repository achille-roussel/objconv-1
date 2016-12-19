package objconv

import (
	"errors"
	"reflect"
	"time"
)

// The Parser interface must be implemented by types that provide decoding of a
// specific format (like json, resp, ...).
//
// Parsers are not expected to be safe for use by multiple goroutines.
type Parser interface {
	// ParseType is called by a decoder to ask the parser what is the type of
	// the next value that can be parsed.
	ParseType() (Type, error)

	// ParseNil parses a nil value.
	ParseNil() error

	// ParseBool parses a boolean value.
	ParseBool() (bool, error)

	// ParseInt parses an integer value.
	ParseInt() (int64, error)

	// ParseBool parses an unsigned integer value.
	ParseUint() (uint64, error)

	// ParseBool parses a floating point value.
	ParseFloat() (float64, error)

	// ParseBool parses a string value.
	//
	// The string is returned as a byte slice because it is expected to be
	// pointing at an internal memory buffer, the decoder will make a copy of
	// the value. This design allows more memory allocation optimizations.
	ParseString() ([]byte, error)

	// ParseBool parses a byte array value.
	//
	// The returned byte slice is expected to be pointing at an internal memory
	// buffer, the decoder will make a copy of the value. This design allows more
	// memory allocation optimizations.
	ParseBytes() ([]byte, error)

	// ParseBool parses a time value.
	ParseTime() (time.Time, error)

	// ParseBool parses a duration value.
	ParseDuration() (time.Duration, error)

	// ParseError parses an error value.
	ParseError() (error, error)

	// ParseArrayBegin is called by the array-decoding algorithm when it starts.
	//
	// The method should return the length of the array being decoded, or a
	// negative value if it is unknown (some formats like json don't keep track
	// of the length of the array).
	ParseArrayBegin() (int, error)

	// ParseArrayEnd is called by the array-decoding algorithm when it
	// completes.
	ParseArrayEnd() error

	// ParseArrayNext is called by the array-decoding algorithm between each
	// value parsed in the array.
	//
	// If the ParseArrayBegin method returned a negative value this method
	// should return objconv.End to indicated that there is no more elements to
	// parse in the array.
	ParseArrayNext() error

	// ParseMapBegin is called by the map-decoding algorithm when it starts.
	//
	// The method should return the length of the map being decoded, or a
	// negative value if it is unknown (some formats like json don't keep track
	// of the length of the map).
	ParseMapBegin() (int, error)

	// ParseMapEnd is called by the map-decoding algorithm when it completes.
	ParseMapEnd() error

	// ParseMapValue is called by the map-decoding algorithm after parsing a key
	// but before parsing the associated value.
	ParseMapValue() error

	// ParseMapNext is called by the map-decoding algorithm between each
	// value parsed in the map.
	//
	// If the ParseMapBegin method returned a negative value this method should
	// return objconv.End to indicated that there is no more elements to parse
	// in the map.
	ParseMapNext() error
}

// ValueParser is parser that uses "natural" in-memory representation of data
// structures.
//
// This is mainly useful for testing the decoder algorithms.
type ValueParser struct {
	stack []reflect.Value
	ctx   []valueParserContext
}

type valueParserContext struct {
	index  int
	length int
	value  reflect.Value
	keys   []reflect.Value
	fields []StructField
}

// NewValueParser creates a new parser that exposes the value v.
func NewValueParser(v interface{}) *ValueParser {
	return &ValueParser{
		stack: []reflect.Value{reflect.ValueOf(v)},
	}
}

func (p *ValueParser) ParseType() (Type, error) {
	v := p.value()

	if !v.IsValid() {
		return Nil, nil
	}

	switch v.Interface().(type) {
	case time.Time:
		return Time, nil

	case time.Duration:
		return Duration, nil

	case error:
		return Error, nil
	}

	switch v.Kind() {
	case reflect.Bool:
		return Bool, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Int, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return Uint, nil

	case reflect.Float32, reflect.Float64:
		return Float, nil

	case reflect.String:
		return String, nil

	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return Bytes, nil
		}
		return Array, nil

	case reflect.Array:
		return Array, nil

	case reflect.Map:
		return Map, nil

	case reflect.Struct:
		return Map, nil

	case reflect.Interface:
		if v.IsNil() {
			return Nil, nil
		}
	}

	return Nil, errors.New("objconv: unsupported type found in value parser: " + v.Type().String())
}

func (p *ValueParser) ParseNil() (err error) {
	return
}

func (p *ValueParser) ParseBool() (v bool, err error) {
	v = p.value().Bool()
	return
}

func (p *ValueParser) ParseInt() (v int64, err error) {
	v = p.value().Int()
	return
}

func (p *ValueParser) ParseUint() (v uint64, err error) {
	v = p.value().Uint()
	return
}

func (p *ValueParser) ParseFloat() (v float64, err error) {
	v = p.value().Float()
	return
}

func (p *ValueParser) ParseString() (v []byte, err error) {
	v = []byte(p.value().String())
	return
}

func (p *ValueParser) ParseBytes() (v []byte, err error) {
	v = p.value().Bytes()
	return
}

func (p *ValueParser) ParseTime() (v time.Time, err error) {
	v = p.value().Interface().(time.Time)
	return
}

func (p *ValueParser) ParseDuration() (v time.Duration, err error) {
	v = p.value().Interface().(time.Duration)
	return
}

func (p *ValueParser) ParseError() (v error, err error) {
	v = p.value().Interface().(error)
	return
}

func (p *ValueParser) ParseArrayBegin() (n int, err error) {
	v := p.value()
	n = v.Len()
	p.pushContext(valueParserContext{length: n, value: v})

	if n != 0 {
		p.push(v.Index(0))
	}

	return
}

func (p *ValueParser) ParseArrayEnd() (err error) {
	ctx := p.context()

	if ctx.length != 0 {
		p.pop()
	}

	p.popContext()
	return
}

func (p *ValueParser) ParseArrayNext() (err error) {
	ctx := p.context()
	ctx.index++
	p.pop()
	p.push(ctx.value.Index(ctx.index))
	return
}

func (p *ValueParser) ParseMapBegin() (n int, err error) {
	v := p.value()

	if v.Kind() == reflect.Map {
		n = v.Len()
		k := v.MapKeys()
		p.pushContext(valueParserContext{length: n, value: v, keys: k})
		if n != 0 {
			p.push(k[0])
		}
	} else {
		c := valueParserContext{value: v}
		s := LookupStruct(v.Type())

		for _, f := range s.Fields {
			if !omit(f, v.FieldByIndex(f.Index)) {
				c.fields = append(c.fields, f)
				n++
			}
		}

		p.pushContext(c)
		if n != 0 {
			p.push(reflect.ValueOf(c.fields[0].Name))
		}
	}

	return
}

func (p *ValueParser) ParseMapEnd() (err error) {
	ctx := p.context()

	if ctx.length != 0 {
		p.pop()
	}

	p.popContext()
	return
}

func (p *ValueParser) ParseMapValue() (err error) {
	ctx := p.context()
	p.pop()

	if ctx.keys != nil {
		p.push(ctx.value.MapIndex(ctx.keys[ctx.index]))
	} else {
		p.push(ctx.value.FieldByIndex(ctx.fields[ctx.index].Index))
	}

	return
}

func (p *ValueParser) ParseMapNext() (err error) {
	ctx := p.context()
	ctx.index++
	p.pop()

	if ctx.keys != nil {
		p.push(ctx.keys[ctx.index])
	} else {
		p.push(reflect.ValueOf(ctx.fields[ctx.index].Name))
	}

	return
}

func (p *ValueParser) value() reflect.Value {
	v := p.stack[len(p.stack)-1]

	if !v.IsValid() {
		return v
	}

	switch v.Interface().(type) {
	case error:
		return v
	}

dereference:
	switch v.Kind() {
	case reflect.Interface, reflect.Ptr:
		if !v.IsNil() {
			v = v.Elem()
			goto dereference
		}
	}

	return v
}

func (p *ValueParser) push(v reflect.Value) {
	p.stack = append(p.stack, v)
}

func (p *ValueParser) pop() {
	p.stack = p.stack[:len(p.stack)-1]
}

func (p *ValueParser) pushContext(ctx valueParserContext) {
	p.ctx = append(p.ctx, ctx)
}

func (p *ValueParser) popContext() {
	p.ctx = p.ctx[:len(p.ctx)-1]
}

func (p *ValueParser) context() *valueParserContext {
	return &p.ctx[len(p.ctx)-1]
}
