/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema

import (
	"math/big"
	"reflect"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"go.arpabet.com/value"
	"golang.org/x/xerrors"
)

// Kind is what a field holds.
type Kind string

// The kinds of fields. The integer and float kinds carry their Go width, so
// that a value written from JSON or JavaScript per the definition takes the
// form a Go reader of the type expects.
const (
	KindBool    Kind = "bool"
	KindInt8    Kind = "int8"
	KindInt16   Kind = "int16"
	KindInt32   Kind = "int32"
	KindInt64   Kind = "int64"
	KindUint8   Kind = "uint8"
	KindUint16  Kind = "uint16"
	KindUint32  Kind = "uint32"
	KindUint64  Kind = "uint64"
	KindFloat32 Kind = "float32"
	KindFloat64 Kind = "float64"
	KindString  Kind = "string"
	KindBytes   Kind = "bytes"
	KindTime    Kind = "time"
	KindBigInt  Kind = "bigint"
	KindDecimal Kind = "decimal"
	// KindList holds a list of Elem.
	KindList Kind = "list"
	// KindMap holds a map of string keys to Elem.
	KindMap Kind = "map"
	// KindObject holds a map that is a value of the definition named Object.
	KindObject Kind = "object"
	// KindAny holds any value, untyped.
	KindAny Kind = "any"
)

var kinds = map[Kind]bool{
	KindBool: true, KindInt8: true, KindInt16: true, KindInt32: true, KindInt64: true,
	KindUint8: true, KindUint16: true, KindUint32: true, KindUint64: true, KindFloat32: true, KindFloat64: true,
	KindString: true, KindBytes: true, KindTime: true, KindBigInt: true, KindDecimal: true,
	KindList: true, KindMap: true, KindObject: true, KindAny: true,
}

// FieldType is what a field holds.
type FieldType struct {
	// Kind is the field's kind.
	Kind Kind `value:"kind"`
	// Elem is what a list's items or a map's values hold.
	Elem *FieldType `value:"elem,omitempty"`
	// Object names the definition of an object field's value.
	Object string `value:"object,omitempty"`
	// Nullable: the field may hold null (a Go pointer or interface).
	Nullable bool `value:"nullable,omitempty"`
}

// Field is a field of a definition: a key of the value's map.
type Field struct {
	// Name is the key.
	Name string `value:"name"`
	// Type is what the field holds.
	Type FieldType `value:"type"`
	// Optional: the field may be absent (a Go field tagged omitempty).
	Optional bool `value:"optional,omitempty"`
}

// Type is a definition: the name of a domain type, for a Go type its package
// path and name, and the fields of its values' maps, sorted by name. A
// definition never changes; a domain type that gains or loses a field is a
// new definition under the same name, with an ID of its own.
type Type struct {
	// Name is the domain type's name.
	Name string `value:"name"`
	// Fields are sorted by name.
	Fields []Field `value:"fields"`
}

// Field returns the field with the name.
func (t Type) Field(name string) (Field, bool) {
	i := sort.Search(len(t.Fields), func(i int) bool { return t.Fields[i].Name >= name })
	if i < len(t.Fields) && t.Fields[i].Name == name {
		return t.Fields[i], true
	}
	return Field{}, false
}

// Value returns the definition as a value: the map its canonical bytes hold.
func (t Type) Value() (value.Value, error) {
	return value.Marshal(t)
}

// Bytes returns the canonical bytes of the definition, which are the same
// for identical definitions.
func (t Type) Bytes() ([]byte, error) {
	v, err := t.Value()
	if err != nil {
		return nil, xerrors.Errorf("valueschema: definition %s: %w", t.Name, err)
	}
	return value.Pack(v)
}

// Equal reports whether two definitions are identical.
func (t Type) Equal(o Type) bool {
	a, err := t.Bytes()
	if err != nil {
		return false
	}
	b, err := o.Bytes()
	return err == nil && string(a) == string(b)
}

// Clone returns a copy that shares nothing with the definition.
func (t Type) Clone() Type {
	out := Type{Name: t.Name, Fields: make([]Field, len(t.Fields))}
	for i, f := range t.Fields {
		f.Type = f.Type.clone()
		out.Fields[i] = f
	}
	return out
}

func (ft FieldType) clone() FieldType {
	if ft.Elem != nil {
		e := ft.Elem.clone()
		ft.Elem = &e
	}
	return ft
}

// Validate checks a definition: a name, fields with names, sorted and
// unique, of known kinds, an element for every list and map and a definition
// name for every object.
func (t Type) Validate() error {
	if t.Name == "" {
		return xerrors.Errorf("valueschema: a definition without a name: %w", ErrDefinition)
	}
	for i, f := range t.Fields {
		if f.Name == "" {
			return xerrors.Errorf("valueschema: %s: a field without a name: %w", t.Name, ErrDefinition)
		}
		if i > 0 && t.Fields[i-1].Name >= f.Name {
			return xerrors.Errorf("valueschema: %s: fields %q and %q out of order or repeated: %w", t.Name, t.Fields[i-1].Name, f.Name, ErrDefinition)
		}
		if err := f.Type.validate(); err != nil {
			return xerrors.Errorf("valueschema: %s.%s: %w", t.Name, f.Name, err)
		}
	}
	return nil
}

func (ft FieldType) validate() error {
	if !kinds[ft.Kind] {
		return xerrors.Errorf("unknown kind %q: %w", ft.Kind, ErrDefinition)
	}
	switch ft.Kind {
	case KindList, KindMap:
		if ft.Elem == nil {
			return xerrors.Errorf("%s without an element: %w", ft.Kind, ErrDefinition)
		}
		return ft.Elem.validate()
	case KindObject:
		if ft.Object == "" {
			return xerrors.Errorf("object without a definition name: %w", ErrDefinition)
		}
	}
	if ft.Elem != nil && ft.Kind != KindList && ft.Kind != KindMap {
		return xerrors.Errorf("%s with an element: %w", ft.Kind, ErrDefinition)
	}
	if ft.Object != "" && ft.Kind != KindObject {
		return xerrors.Errorf("%s with a definition name: %w", ft.Kind, ErrDefinition)
	}
	return nil
}

// TypeFromValue reads a definition from a value, as Value writes it, and
// validates it.
func TypeFromValue(v value.Value) (Type, error) {
	var t Type
	if err := fieldCodec.Unmarshal(v, &t); err != nil {
		return Type{}, xerrors.Errorf("valueschema: definition: %v: %w", err, ErrDefinition)
	}
	if err := t.Validate(); err != nil {
		return Type{}, err
	}
	return t, nil
}

// ParseType reads a definition from its canonical bytes and validates it.
func ParseType(b []byte) (Type, error) {
	v, err := value.Unpack(b, true)
	if err != nil {
		return Type{}, xerrors.Errorf("valueschema: definition: %v: %w", err, ErrDefinition)
	}
	return TypeFromValue(v)
}

var (
	timeType    = reflect.TypeOf(time.Time{})
	decimalType = reflect.TypeOf(decimal.Decimal{})
	bigIntType  = reflect.TypeOf(big.Int{})
	valueType   = reflect.TypeOf((*value.Value)(nil)).Elem()
)

// Typed reports whether values of a Go type are written with a type: a named
// struct, other than the structs value writes as a kind of their own
// (time.Time, decimal.Decimal, big.Int) and those that are a value.Value.
func Typed(rt reflect.Type) bool {
	return rt.Kind() == reflect.Struct && rt.Name() != "" &&
		rt != timeType && rt != decimalType && rt != bigIntType &&
		!rt.Implements(valueType) && !reflect.PointerTo(rt).Implements(valueType)
}

// GoName is the name of a Go type's definitions: its package path and name.
func GoName(rt reflect.Type) string {
	if rt.PkgPath() == "" {
		return rt.Name()
	}
	return rt.PkgPath() + "." + rt.Name()
}

// TypeOf derives the definition of a Go struct type (or a pointer to one), as
// value.Marshal maps its values, and the definitions of the struct types its
// fields hold. An embedded struct is a field named by its type, as value
// writes it.
func TypeOf(rt reflect.Type) (Type, []Type, error) {
	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if !Typed(rt) {
		return Type{}, nil, xerrors.Errorf("valueschema: %s: %w", rt, ErrNotTyped)
	}
	d := deriver{seen: map[reflect.Type]bool{}}
	t, err := d.derive(rt)
	if err != nil {
		return Type{}, nil, err
	}
	return t, d.nested, nil
}

type deriver struct {
	seen   map[reflect.Type]bool
	nested []Type
}

func (d *deriver) derive(rt reflect.Type) (Type, error) {
	d.seen[rt] = true
	fields, err := fieldCodec.StructFields(rt)
	if err != nil {
		return Type{}, xerrors.Errorf("valueschema: %s: %w", rt, err)
	}
	t := Type{Name: GoName(rt), Fields: make([]Field, 0, len(fields))}
	for _, f := range fields {
		ft, err := d.fieldType(f.Field.Type)
		if err != nil {
			return Type{}, xerrors.Errorf("valueschema: %s.%s: %w", rt, f.Field.Name, err)
		}
		t.Fields = append(t.Fields, Field{Name: f.Name, Type: ft, Optional: f.OmitEmpty})
	}
	sort.Slice(t.Fields, func(i, j int) bool { return t.Fields[i].Name < t.Fields[j].Name })
	for i := 1; i < len(t.Fields); i++ {
		if t.Fields[i-1].Name == t.Fields[i].Name {
			return Type{}, xerrors.Errorf("valueschema: %s: two fields named %q: %w", rt, t.Fields[i].Name, ErrNotTyped)
		}
	}
	return t, nil
}

func (d *deriver) fieldType(rt reflect.Type) (FieldType, error) {
	nullable := false
	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
		nullable = true
	}
	ft := FieldType{Nullable: nullable}
	switch rt {
	case timeType:
		ft.Kind = KindTime
		return ft, nil
	case decimalType:
		ft.Kind = KindDecimal
		return ft, nil
	case bigIntType:
		ft.Kind = KindBigInt
		return ft, nil
	}
	if rt.Kind() != reflect.Interface && (rt.Implements(valueType) || reflect.PointerTo(rt).Implements(valueType)) {
		ft.Kind = KindAny
		return ft, nil
	}
	switch rt.Kind() {
	case reflect.Interface:
		ft.Kind, ft.Nullable = KindAny, true
	case reflect.Bool:
		ft.Kind = KindBool
	case reflect.Int, reflect.Int64:
		ft.Kind = KindInt64
	case reflect.Int8:
		ft.Kind = KindInt8
	case reflect.Int16:
		ft.Kind = KindInt16
	case reflect.Int32:
		ft.Kind = KindInt32
	case reflect.Uint, reflect.Uint64:
		ft.Kind = KindUint64
	case reflect.Uint8:
		ft.Kind = KindUint8
	case reflect.Uint16:
		ft.Kind = KindUint16
	case reflect.Uint32:
		ft.Kind = KindUint32
	case reflect.Float32:
		ft.Kind = KindFloat32
	case reflect.Float64:
		ft.Kind = KindFloat64
	case reflect.String:
		ft.Kind = KindString
	case reflect.Slice, reflect.Array:
		if rt.Elem().Kind() == reflect.Uint8 {
			ft.Kind = KindBytes
			break
		}
		elem, err := d.fieldType(rt.Elem())
		if err != nil {
			return FieldType{}, err
		}
		ft.Kind, ft.Elem = KindList, &elem
	case reflect.Map:
		if rt.Key().Kind() != reflect.String {
			return FieldType{}, xerrors.Errorf("map key %s is not a string: %w", rt.Key(), ErrNotTyped)
		}
		elem, err := d.fieldType(rt.Elem())
		if err != nil {
			return FieldType{}, err
		}
		ft.Kind, ft.Elem = KindMap, &elem
	case reflect.Struct:
		if rt.Name() == "" {
			// An unnamed struct has no definition to name: its map is untyped.
			ft.Kind = KindAny
			break
		}
		if !d.seen[rt] {
			nested, err := d.derive(rt)
			if err != nil {
				return FieldType{}, err
			}
			d.nested = append(d.nested, nested)
		}
		ft.Kind, ft.Object = KindObject, GoName(rt)
	default:
		return FieldType{}, xerrors.Errorf("kind %s has no value form: %w", rt.Kind(), ErrNotTyped)
	}
	return ft, nil
}

// fieldCodec reads values field by field, as schema evolution needs: a field
// the value lacks stays at its zero value, and a number converts into a
// field only without loss.
var fieldCodec = value.DefaultValueCodec().AllowMissing().Lossless()
