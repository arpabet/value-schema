/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema

import (
	"bytes"
	"context"

	"github.com/tinylib/msgp/msgp"
	"go.arpabet.com/value"
	"golang.org/x/xerrors"
)

// Instance is a value read without its Go type, as GemFire's PdxInstance: its
// type ID and its bytes, whose fields are read one at a time without decoding
// the rest. An instance shares its bytes with whoever opened it.
type Instance struct {
	b       []byte
	id      TypeID
	payload []byte
	codec   *Codec
}

// Open returns the instance of a value's bytes, checking the header and that
// the payload is one MessagePack value. Its Type needs a registry: open it
// with a Codec for that.
func Open(b []byte) (Instance, error) {
	return open(b, nil)
}

// Open returns the instance of a value's bytes, whose Type the codec's
// registry gives.
func (c *Codec) Open(b []byte) (Instance, error) {
	return open(b, c)
}

func open(b []byte, c *Codec) (Instance, error) {
	id, payload, err := Split(b)
	if err != nil {
		return Instance{}, err
	}
	rest, err := msgp.Skip(payload)
	if err != nil || len(rest) != 0 {
		return Instance{}, xerrors.Errorf("valueschema: payload is not one MessagePack value: %w", ErrHeader)
	}
	return Instance{b: b, id: id, payload: payload, codec: c}, nil
}

// IsZero reports whether the instance is the zero Instance, which holds no
// value.
func (i Instance) IsZero() bool { return i.b == nil }

// ID is the type ID; Untyped for a value without a type.
func (i Instance) ID() TypeID { return i.id }

// Bytes are the value's bytes, its header first.
func (i Instance) Bytes() []byte { return i.b }

// Payload is the value's canonical MessagePack.
func (i Instance) Payload() []byte { return i.payload }

// Type returns the value's definition from the registry of the codec that
// opened it.
func (i Instance) Type(ctx context.Context) (Type, error) {
	if i.id == Untyped {
		return Type{}, xerrors.Errorf("valueschema: an untyped value has no definition: %w", ErrUnknownType)
	}
	if i.codec == nil {
		return Type{}, ErrNoRegistry
	}
	return i.codec.reg.ByID(ctx, i.id)
}

// FieldBytes locates a field of the value's map and returns its MessagePack,
// a subslice of the payload, or nil when the value has no such field. It
// allocates nothing.
func (i Instance) FieldBytes(name string) []byte {
	raw := msgp.Locate(name, i.payload)
	if len(raw) == 0 {
		return nil
	}
	return raw
}

// HasField reports whether the value's map has the field.
func (i Instance) HasField(name string) bool { return i.FieldBytes(name) != nil }

// Field decodes a single field of the value's map, or returns nil when the
// value has no such field, as PdxInstance.getField does.
func (i Instance) Field(name string) (value.Value, error) {
	raw := i.FieldBytes(name)
	if raw == nil {
		return nil, nil
	}
	v, err := value.Unpack(raw, true)
	if err != nil {
		return nil, xerrors.Errorf("valueschema: field %q: %w", name, err)
	}
	return v, nil
}

// FieldNames returns the keys of the value's map, in their order there:
// sorted, for canonical bytes.
func (i Instance) FieldNames() ([]string, error) {
	n, rest, err := msgp.ReadMapHeaderBytes(i.payload)
	if err != nil {
		return nil, xerrors.Errorf("valueschema: the value is not a map: %w", err)
	}
	names := make([]string, 0, min(int(n), len(rest)))
	for range n {
		var key []byte
		if key, rest, err = msgp.ReadStringZC(rest); err != nil {
			return nil, xerrors.Errorf("valueschema: map key: %w", err)
		}
		names = append(names, string(key))
		if rest, err = msgp.Skip(rest); err != nil {
			return nil, xerrors.Errorf("valueschema: field %q: %w", key, err)
		}
	}
	return names, nil
}

// Value decodes the whole value into its tree.
func (i Instance) Value() (value.Value, error) {
	v, err := value.Unpack(i.payload, true)
	if err != nil {
		return nil, xerrors.Errorf("valueschema: payload: %w", err)
	}
	return v, nil
}

// Decode reads the value into dst, as Codec.Decode reads into a Go value.
func (i Instance) Decode(dst any) error {
	v, err := i.Value()
	if err != nil {
		return err
	}
	if err := fieldCodec.Unmarshal(v, dst); err != nil {
		return xerrors.Errorf("valueschema: decode into %T: %w", dst, err)
	}
	return nil
}

// JSON renders the value as lossless JSON in the mode (value.FormatJSON).
func (i Instance) JSON(mode value.JSONMode) ([]byte, error) {
	v, err := i.Value()
	if err != nil {
		return nil, err
	}
	return value.FormatJSON(v, mode)
}

// MarshalJSON renders the value as relaxed lossless JSON.
func (i Instance) MarshalJSON() ([]byte, error) { return i.JSON(value.JSONRelaxed) }

// Equal reports whether two instances hold the same bytes: the same type and
// the same canonical payload.
func (i Instance) Equal(o Instance) bool { return bytes.Equal(i.b, o.b) }

// String renders the value as relaxed JSON.
func (i Instance) String() string {
	b, err := i.JSON(value.JSONRelaxed)
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return string(b)
}
