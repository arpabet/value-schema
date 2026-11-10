/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema

import (
	"context"
	"encoding/base64"
	"math"
	"strings"

	"go.arpabet.com/value"
	"golang.org/x/xerrors"
)

// FromJSON reads JSON text (value.ParseJSON, either mode) into a value's
// bytes. Without a type name the value is untyped, and a bare number is a
// Decimal from its exact text. With a type name the value is written with the
// latest definition of the name in the registry, each field converted to its
// kind by Conform.
func (c *Codec) FromJSON(ctx context.Context, typeName string, data []byte) ([]byte, error) {
	v, err := value.ParseJSON(data)
	if err != nil {
		return nil, xerrors.Errorf("valueschema: %w", err)
	}
	if typeName == "" {
		return Pack(Untyped, v)
	}
	entries, err := c.reg.Types(ctx)
	if err != nil {
		return nil, err
	}
	e, ok := Latest(entries, typeName)
	if !ok {
		return nil, xerrors.Errorf("valueschema: type %s: %w", typeName, ErrUnknownType)
	}
	if v, err = Conform(v, e.Type, entries); err != nil {
		return nil, err
	}
	return Pack(e.ID, v)
}

// ToJSON renders a value's bytes as lossless JSON in the mode.
func ToJSON(b []byte, mode value.JSONMode) ([]byte, error) {
	i, err := Open(b)
	if err != nil {
		return nil, err
	}
	return i.JSON(mode)
}

// Conform converts a value tree to a definition, as a typed writer that has
// no Go type needs (untyped-values R6): a map whose keys are fields of the
// definition, each value converted to its field's kind without loss. A number
// takes the form a Go reader of the kind expects (an integer, a double, a
// Decimal or a BigInt); a bytes field also takes a base64 string and a time
// field an RFC 3339 string; null is allowed only where the field is
// nullable. An object field follows the latest definition of its name among
// entries. A key the definition lacks, a kind that does not fit and a number
// that does not convert exactly fail with ErrConform, naming the field.
func Conform(v value.Value, t Type, entries []Entry) (value.Value, error) {
	return conformer{entries: entries}.object(v, t, t.Name, 0)
}

type conformer struct {
	entries []Entry
}

func (c conformer) object(v value.Value, t Type, path string, depth int) (value.Value, error) {
	if value.MaxParseDepth > 0 && depth > value.MaxParseDepth {
		return nil, xerrors.Errorf("valueschema: %s nests too deep: %w", path, ErrConform)
	}
	m, ok := v.(value.Map)
	if !ok || v.Kind() != value.MAP {
		return nil, xerrors.Errorf("valueschema: %s holds %s, want a map for %s: %w", path, kindOf(v), t.Name, ErrConform)
	}
	entries := make([]value.MapEntry, 0, m.Len())
	for _, e := range m.Entries() {
		f, ok := t.Field(e.Key())
		if !ok {
			return nil, xerrors.Errorf("valueschema: %s.%s: %s has no such field: %w", path, e.Key(), t.Name, ErrConform)
		}
		fv, err := c.field(e.Value(), f.Type, path+"."+e.Key(), depth+1)
		if err != nil {
			return nil, err
		}
		entries = append(entries, value.ImmutableEntry(e.Key(), fv))
	}
	return value.ImmutableMap(entries, false), nil
}

func (c conformer) field(v value.Value, ft FieldType, path string, depth int) (value.Value, error) {
	if v == nil || v.Kind() == value.NULL {
		if ft.Nullable || ft.Kind == KindAny {
			return value.Null, nil
		}
		return nil, xerrors.Errorf("valueschema: %s: null in a field that is not nullable: %w", path, ErrConform)
	}
	mismatch := func() error {
		return xerrors.Errorf("valueschema: %s holds %s, want %s: %w", path, kindOf(v), ft.Kind, ErrConform)
	}
	inexact := func(err error) error {
		return xerrors.Errorf("valueschema: %s: %v: %w", path, err, ErrConform)
	}
	switch ft.Kind {
	case KindAny:
		return v, nil
	case KindBool:
		if v.Kind() != value.BOOL {
			return nil, mismatch()
		}
		return v, nil
	case KindString:
		if s, ok := v.(value.String); !ok || s.Type() != value.UTF8 {
			return nil, mismatch()
		}
		return v, nil
	case KindBytes:
		s, ok := v.(value.String)
		if !ok {
			return nil, mismatch()
		}
		if s.Type() == value.RAW {
			return v, nil
		}
		b, err := base64.StdEncoding.DecodeString(s.Utf8())
		if err != nil {
			return nil, inexact(err)
		}
		return value.Raw(b, false), nil
	case KindTime:
		switch x := v.(type) {
		case value.Timestamp:
			return x, nil
		case value.String:
			if x.Type() == value.UTF8 {
				ts, err := value.ParseTimestamp(x.Utf8())
				if err != nil {
					return nil, inexact(err)
				}
				return ts, nil
			}
		}
		return nil, mismatch()
	}
	if n, ok := v.(value.Number); ok && v.Kind() == value.NUMBER {
		switch ft.Kind {
		case KindInt8, KindInt16, KindInt32, KindInt64:
			i, err := value.ExactInt(n, bitsOf(ft.Kind))
			if err != nil {
				return nil, inexact(err)
			}
			return value.Long(i), nil
		case KindUint8, KindUint16, KindUint32, KindUint64:
			u, err := value.ExactUint(n, bitsOf(ft.Kind))
			if err != nil {
				return nil, inexact(err)
			}
			if u > math.MaxInt64 {
				b, _ := value.ExactBigInt(n)
				return value.BigInt(b), nil
			}
			return value.Long(int64(u)), nil
		case KindFloat32, KindFloat64:
			f, err := value.ExactFloat(n, bitsOf(ft.Kind))
			if err != nil {
				return nil, inexact(err)
			}
			return value.Double(f), nil
		case KindDecimal:
			d, err := value.ExactDecimal(n)
			if err != nil {
				return nil, inexact(err)
			}
			return value.Decimal(d), nil
		case KindBigInt:
			b, err := value.ExactBigInt(n)
			if err != nil {
				return nil, inexact(err)
			}
			return value.BigInt(b), nil
		}
		return nil, mismatch()
	}
	switch ft.Kind {
	case KindList:
		l, ok := v.(value.List)
		if !ok || v.Kind() != value.LIST {
			return nil, mismatch()
		}
		items := l.Values()
		out := make([]value.Value, len(items))
		for i, item := range items {
			x, err := c.field(item, *ft.Elem, path+"[]", depth+1)
			if err != nil {
				return nil, err
			}
			out[i] = x
		}
		return value.ImmutableList(out), nil
	case KindMap:
		m, ok := v.(value.Map)
		if !ok || v.Kind() != value.MAP {
			return nil, mismatch()
		}
		entries := make([]value.MapEntry, 0, m.Len())
		for _, e := range m.Entries() {
			x, err := c.field(e.Value(), *ft.Elem, path+"."+e.Key(), depth+1)
			if err != nil {
				return nil, err
			}
			entries = append(entries, value.ImmutableEntry(e.Key(), x))
		}
		return value.ImmutableMap(entries, false), nil
	case KindObject:
		e, ok := Latest(c.entries, ft.Object)
		if !ok {
			return nil, xerrors.Errorf("valueschema: %s: type %s: %w", path, ft.Object, ErrUnknownType)
		}
		return c.object(v, e.Type, path, depth)
	}
	return nil, mismatch()
}

func bitsOf(k Kind) int {
	switch {
	case strings.HasSuffix(string(k), "8"):
		return 8
	case strings.HasSuffix(string(k), "16"):
		return 16
	case strings.HasSuffix(string(k), "32"):
		return 32
	}
	return 64
}

func kindOf(v value.Value) string {
	if v == nil {
		return "nothing"
	}
	if n, ok := v.(value.Number); ok {
		return n.Type().String()
	}
	if s, ok := v.(value.String); ok && s.Type() == value.RAW {
		return "binary"
	}
	return strings.ToLower(v.Kind().String())
}
