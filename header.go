/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema

import (
	"encoding/binary"

	"golang.org/x/xerrors"
)

// Marker is the first byte of every value: 0xc1, the one byte MessagePack
// never uses, so a value is never mistaken for bare MessagePack, and bare
// MessagePack written before the header existed is told apart.
const Marker byte = 0xc1

// HeaderSize is the size of the header: the marker, the type ID, the length
// and the flags.
const HeaderSize = 10

// TypeID identifies a definition in a registry. The top 8 bits are the
// distributed-system ID of the registry that defined it and the lower 24 its
// number there, so that the registries of different sites never hand out the
// same ID. 0 is no type: the payload is untyped canonical MessagePack.
type TypeID uint32

// Untyped is the type ID of a value without a type.
const Untyped TypeID = 0

// MaxNumber is the largest number of a type ID within a distributed system.
const MaxNumber = 0xffffff

// NewTypeID returns the type ID of a number within a distributed system.
func NewTypeID(dsid uint8, number uint32) TypeID {
	return TypeID(uint32(dsid)<<24 | number&MaxNumber)
}

// DistributedSystem is the distributed-system ID of the registry that defined
// the type.
func (id TypeID) DistributedSystem() uint8 { return uint8(id >> 24) }

// Number is the type's number within its distributed system.
func (id TypeID) Number() uint32 { return uint32(id) & MaxNumber }

// Flags are the header's flag bits. None is defined: a writer sets 0, and a
// reader refuses a value with any flag set, so that a flag defined later is
// never misread.
type Flags uint8

// Header precedes the payload of every value.
type Header struct {
	// Type is the payload's type; Untyped when it has none.
	Type TypeID
	// Length is the payload's length in bytes.
	Length uint32
	// Flags are 0.
	Flags Flags
}

// AppendHeader appends the header of an n-byte payload of the type.
func AppendHeader(dst []byte, id TypeID, n int) []byte {
	dst = append(dst, Marker)
	dst = binary.BigEndian.AppendUint32(dst, uint32(id))
	dst = binary.BigEndian.AppendUint32(dst, uint32(n))
	return append(dst, 0)
}

// Wrap returns the payload behind its header, in a new slice.
func Wrap(id TypeID, payload []byte) []byte {
	return append(AppendHeader(make([]byte, 0, HeaderSize+len(payload)), id, len(payload)), payload...)
}

// ParseHeader reads the header b starts with.
func ParseHeader(b []byte) (Header, error) {
	if len(b) < HeaderSize {
		return Header{}, xerrors.Errorf("valueschema: %d bytes hold no header: %w", len(b), ErrHeader)
	}
	if b[0] != Marker {
		return Header{}, xerrors.Errorf("valueschema: value starts with %#02x, not the marker: %w", b[0], ErrHeader)
	}
	h := Header{
		Type:   TypeID(binary.BigEndian.Uint32(b[1:5])),
		Length: binary.BigEndian.Uint32(b[5:9]),
		Flags:  Flags(b[9]),
	}
	if h.Flags != 0 {
		return Header{}, xerrors.Errorf("valueschema: unknown flags %#02x: %w", h.Flags, ErrHeader)
	}
	return h, nil
}

// Split checks a value's header and returns its type ID and its payload, a
// subslice of b. The header's length must be the payload's.
func Split(b []byte) (TypeID, []byte, error) {
	h, err := ParseHeader(b)
	if err != nil {
		return 0, nil, err
	}
	payload := b[HeaderSize:]
	if uint64(h.Length) != uint64(len(payload)) {
		return 0, nil, xerrors.Errorf("valueschema: header says %d bytes, the payload has %d: %w", h.Length, len(payload), ErrHeader)
	}
	return h.Type, payload, nil
}

// HasHeader reports whether b starts with the marker, as a value with its
// header does and bare MessagePack never does.
func HasHeader(b []byte) bool { return len(b) > 0 && b[0] == Marker }

// Normalize returns a value with its header: b itself when it has a header
// that checks, or b behind an untyped header when it is bare MessagePack
// written before the header existed.
func Normalize(b []byte) ([]byte, error) {
	if HasHeader(b) {
		if _, _, err := Split(b); err != nil {
			return nil, err
		}
		return b, nil
	}
	if len(b) == 0 {
		return nil, xerrors.Errorf("valueschema: an empty value: %w", ErrHeader)
	}
	return Wrap(Untyped, b), nil
}
