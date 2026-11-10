/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

/*
Package valueschema is PDX for the value framework (go.arpabet.com/value): a
type registry, values that carry their type, and instances whose fields are
read without decoding the whole value, as GemFire's PDX type registry and
PdxInstance provide.

A value is a header followed by canonical MessagePack (FORMAT.md):

	0      0xc1, the byte MessagePack never uses
	1..4   the type ID, big-endian; 0 is no type
	5..8   the payload's length, big-endian
	9      flags, all 0
	10..   the payload, canonical MessagePack

A typed value's payload is the map value.Marshal writes for a Go struct, keyed
by field name, and its type ID names a definition in a Registry: the type's
name and the name and kind of each field. A definition never changes; a Go
type that gains a field is a new definition under the same name, a version.
Readers match fields by name, so a value of any version reads into any version
of the Go type: keys the Go type lacks are ignored and fields the value lacks
stay at their zero value.

IDs are allocated as GemFire allocates PDX type IDs: the distributed-system ID
in the top 8 bits, and in the lower 24 the hash of the definition, probing for
the next free number on a collision.

A Codec writes Go values with their types (Encode), reads them back (Decode)
and opens them as an Instance, whose Field locates a single field in the
payload without allocating. FromJSON and Instance.JSON convert to and from
lossless JSON (value.ParseJSON), and TypeScript generates declarations for a
registry's types.
*/
package valueschema

import "golang.org/x/xerrors"

var (
	// ErrHeader: the bytes do not start with a well-formed header.
	ErrHeader = xerrors.New("valueschema: malformed header")
	// ErrUnknownType: the registry holds no definition with the ID or name.
	ErrUnknownType = xerrors.New("valueschema: unknown type")
	// ErrNotTyped: the Go type is not a struct that is written with a type.
	ErrNotTyped = xerrors.New("valueschema: not a typed Go type")
	// ErrDefinition: a definition is malformed.
	ErrDefinition = xerrors.New("valueschema: malformed definition")
	// ErrConform: a value does not fit a definition.
	ErrConform = xerrors.New("valueschema: value does not fit its type")
	// ErrNoRegistry: an instance opened without a codec has no registry to
	// read its type from.
	ErrNoRegistry = xerrors.New("valueschema: no registry")
)
