/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
	"sync"
	"sync/atomic"

	"go.arpabet.com/value"
	"golang.org/x/xerrors"
)

// Codec writes Go values with their types and reads them back, defining the
// definitions of struct values in a registry. It is safe for concurrent use.
type Codec struct {
	reg            Registry
	readSerialized bool
	goTypes        sync.Map // reflect.Type → *goType
	byName         sync.Map // definition name → reflect.Type
}

// goType is what the codec knows of a Go type it writes.
type goType struct {
	t      Type
	nested []Type
	err    error
	mu     sync.Mutex
	id     atomic.Uint32 // 0 until defined
}

// Option configures a codec.
type Option func(*Codec)

// ReadSerialized makes Decode into an any give an Instance for every typed
// value, as GemFire's read-serialized does, instead of the Go type registered
// for its definition's name.
func ReadSerialized(on bool) Option {
	return func(c *Codec) { c.readSerialized = on }
}

// NewCodec returns a codec over a registry.
func NewCodec(r Registry, opts ...Option) *Codec {
	c := &Codec{reg: r}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Registry returns the codec's registry.
func (c *Codec) Registry() Registry { return c.reg }

// ReadsSerialized reports whether Decode into an any gives an Instance for
// every typed value.
func (c *Codec) ReadsSerialized() bool { return c.readSerialized }

// Register makes the codec read a typed value into the Go type whose name its
// definition has when the destination is an any, as GemFire finds the class
// of a PDX type. The samples are values of the Go types or pointers to them.
// A Go type the codec writes is registered by that.
func (c *Codec) Register(samples ...any) error {
	for _, s := range samples {
		rt := reflect.TypeOf(s)
		for rt != nil && rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		if rt == nil || !Typed(rt) {
			return xerrors.Errorf("valueschema: register %T: %w", s, ErrNotTyped)
		}
		c.byName.Store(GoName(rt), rt)
	}
	return nil
}

// TypeID returns the ID of a Go struct type's definition. The first time, it
// defines the definition, and those of the structs its fields hold, in the
// registry.
func (c *Codec) TypeID(ctx context.Context, rt reflect.Type) (TypeID, error) {
	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	g := c.goType(rt)
	if g.err != nil {
		return 0, g.err
	}
	if id := g.id.Load(); id != 0 {
		return TypeID(id), nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if id := g.id.Load(); id != 0 {
		return TypeID(id), nil
	}
	for _, n := range g.nested {
		if _, err := c.reg.Define(ctx, n); err != nil {
			return 0, xerrors.Errorf("valueschema: define %s: %w", n.Name, err)
		}
	}
	id, err := c.reg.Define(ctx, g.t)
	if err != nil {
		return 0, xerrors.Errorf("valueschema: define %s: %w", g.t.Name, err)
	}
	g.id.Store(uint32(id))
	c.byName.LoadOrStore(g.t.Name, rt)
	return id, nil
}

func (c *Codec) goType(rt reflect.Type) *goType {
	if g, ok := c.goTypes.Load(rt); ok {
		return g.(*goType)
	}
	t, nested, err := TypeOf(rt)
	g, _ := c.goTypes.LoadOrStore(rt, &goType{t: t, nested: nested, err: err})
	return g.(*goType)
}

// Encode returns a value's bytes: its header and its canonical MessagePack. A
// struct value (Typed) is typed: the first time the codec writes its Go type,
// the definitions of the type and of the structs its fields hold are defined
// in the registry. An Instance is written as it is. Any other value is
// untyped.
func (c *Codec) Encode(ctx context.Context, v any) ([]byte, error) {
	switch x := v.(type) {
	case Instance:
		return x.Bytes(), nil
	case *Instance:
		if x != nil {
			return x.Bytes(), nil
		}
	}
	id := Untyped
	if rt := structType(v); rt != nil && Typed(rt) {
		var err error
		if id, err = c.TypeID(ctx, rt); err != nil {
			return nil, err
		}
	}
	val, err := value.Marshal(v)
	if err != nil {
		return nil, xerrors.Errorf("valueschema: encode %T: %w", v, err)
	}
	return Pack(id, val)
}

// EncodeUntyped returns the bytes of a value without a type, whatever it is.
func EncodeUntyped(v any) ([]byte, error) {
	val, err := value.Marshal(v)
	if err != nil {
		return nil, xerrors.Errorf("valueschema: encode %T: %w", v, err)
	}
	return Pack(Untyped, val)
}

// Pack returns the bytes of a value tree with the type ID in its header.
func Pack(id TypeID, v value.Value) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(AppendHeader(make([]byte, 0, HeaderSize), id, 0))
	if err := value.Write(&buf, v); err != nil {
		return nil, xerrors.Errorf("valueschema: pack: %w", err)
	}
	b := buf.Bytes()
	binary.BigEndian.PutUint32(b[5:9], uint32(len(b)-HeaderSize))
	return b, nil
}

// structType returns the struct type a value is, through pointers, or nil.
func structType(v any) reflect.Type {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	return rv.Type()
}

// Decode reads a value's bytes into dst, a non-nil pointer:
//
//   - into a Go value of any type, field by field: keys the Go type lacks are
//     ignored, fields the value lacks stay at their zero value, and numbers
//     convert only without loss (value.ValueCodec.Lossless);
//   - into an *Instance, the value opened;
//   - into a *value.Value, its tree;
//   - into an *any, a typed value as the Go type registered for its
//     definition's name (a struct, not a pointer), or as an Instance when no
//     Go type is registered or the codec reads serialized; an untyped value
//     as its tree.
func (c *Codec) Decode(ctx context.Context, b []byte, dst any) error {
	id, payload, err := Split(b)
	if err != nil {
		return err
	}
	switch d := dst.(type) {
	case *Instance:
		*d = Instance{b: b, id: id, payload: payload, codec: c}
		return nil
	case *value.Value:
		v, err := value.Unpack(payload, true)
		if err != nil {
			return xerrors.Errorf("valueschema: payload: %w", err)
		}
		*d = v
		return nil
	case *any:
		if id != Untyped {
			if !c.readSerialized {
				if rt, err := c.goTypeOf(ctx, id); err != nil {
					return err
				} else if rt != nil {
					rv := reflect.New(rt)
					if err := c.Decode(ctx, b, rv.Interface()); err != nil {
						return err
					}
					*d = rv.Elem().Interface()
					return nil
				}
			}
			*d = Instance{b: b, id: id, payload: payload, codec: c}
			return nil
		}
		v, err := value.Unpack(payload, true)
		if err != nil {
			return xerrors.Errorf("valueschema: payload: %w", err)
		}
		*d = v
		return nil
	}
	v, err := value.Unpack(payload, true)
	if err != nil {
		return xerrors.Errorf("valueschema: payload: %w", err)
	}
	if err := fieldCodec.Unmarshal(v, dst); err != nil {
		return xerrors.Errorf("valueschema: decode into %T: %w", dst, err)
	}
	return nil
}

// goTypeOf returns the Go type registered for the name of a definition, or
// nil.
func (c *Codec) goTypeOf(ctx context.Context, id TypeID) (reflect.Type, error) {
	t, err := c.reg.ByID(ctx, id)
	if err != nil {
		if xerrors.Is(err, ErrUnknownType) {
			return nil, nil
		}
		return nil, err
	}
	if rt, ok := c.byName.Load(t.Name); ok {
		return rt.(reflect.Type), nil
	}
	return nil, nil
}
