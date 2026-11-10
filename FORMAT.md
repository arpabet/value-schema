# value-schema format

**Status:** normative, version 1.

## 1. A value

Every value is a header followed by its payload:

| Offset | Size | Field | |
|---|---|---|---|
| 0 | 1 | marker | `0xc1`, the one byte MessagePack never uses |
| 1 | 4 | type ID | big-endian; `0` is no type |
| 5 | 4 | length | big-endian: the payload's size in bytes |
| 9 | 1 | flags | `0` |
| 10 | length | payload | one value in value's canonical MessagePack (go.arpabet.com/value, CANONICAL.md) |

- A reader refuses a value that is shorter than 10 bytes, does not start with the marker, has a flag set or
  whose length is not the size of the rest. No flag is defined; a reader that met one would misread what it
  means, so it refuses it.
- Because MessagePack never uses `0xc1`, bytes that do not start with it are bare MessagePack, written before
  the header existed. `Normalize` puts them behind an untyped header.
- `c1 00000000 00000002 00 9101` is the untyped list `[1]`.

## 2. Types

A **typed** value (type ID ≠ 0) has a map payload: the map `value.Marshal` writes for a Go struct, keyed by
field name. The type ID names a **definition** in a registry:

```text
definition = {name: string, fields: [field...]}            fields sorted by name, names unique
field      = {name: string, type: fieldtype, optional?: bool}
fieldtype  = {kind: string, elem?: fieldtype, object?: string, nullable?: bool}
```

A definition is stored and compared as its canonical MessagePack (keys absent when false or empty). Kinds:
`bool`, `int8` … `int64`, `uint8` … `uint64`, `float32`, `float64`, `string`, `bytes`, `time`, `bigint`,
`decimal`, `list` and `map` (with `elem`; map keys are strings), `object` (with `object`, the name of the
definition its map follows) and `any` (untyped). `optional` marks a field that may be absent (a Go field
tagged `omitempty`), `nullable` one that may hold null (a Go pointer or interface).

For a Go struct, the name is the package path and type name (`github.com/acme/shop.Order`), and the fields are
those `value.Marshal` writes, named as it names them. An embedded struct is a field named by its type. A
nested struct is not typed in the payload: its field is an `object` naming its own definition.

A definition never changes. A Go type that gains or loses a field is a new definition under the same name, a
**version**. Readers match fields by name, so any version reads into any version of the Go type: keys the Go
type lacks are ignored and fields the value lacks stay at their zero value. Numbers convert into Go fields
only without loss (value's `Lossless` codec).

## 3. Type IDs

A type ID is the distributed-system ID (0 to 255) in the top 8 bits and a number from 1 to `0xffffff` in the
lower 24, so that registries of different sites never hand out the same ID. A registry allocates the ID of a
new definition as GemFire allocates PDX type IDs: it starts from the lower 24 bits of the 32-bit FNV-1a hash
of the definition's canonical bytes (1 when they are 0) and, while that ID is taken, tries the next number,
wrapping to 1 after `0xffffff`. Identical definitions get the same ID; a registry defines a definition once.

## 4. JSON

`FromJSON` reads lossless JSON (`value.ParseJSON`, CANONICAL.md "JSON"). Without a type name the value is
untyped and a bare number is a Decimal. With a type name the value takes the latest version of the name and
`Conform` converts each field to its kind:

| Kind | Takes | Writes |
|---|---|---|
| integers | a number that is integral and within range | an integer (a BigInt above int64 for `uint64`) |
| `float32`, `float64` | a number whose shortest float text reads back as the same number | a double |
| `decimal` | any finite number | a Decimal |
| `bigint` | an integral number | a BigInt |
| `bytes` | binary, or a standard base64 string | binary |
| `time` | a timestamp, or an RFC 3339 string | a timestamp |
| `bool`, `string` | the same kind | as it is |
| `list`, `map`, `object` | a list, a map, a map with the definition's keys | each item, value or field converted |
| `any` | anything | as it is |

Null is allowed only in a nullable or `any` field. A key the definition lacks, a kind that does not fit and a
number that does not convert exactly fail, naming the field.

## 5. TypeScript

`TypeScript` writes one interface per definition name, merging its versions: a field that every version has,
required and of one type, is required; any other is optional, and a field whose versions disagree takes the
union of their types. Field types are what `@arpabet/vrpc` reads: `boolean`; `number` for integers up to 32
bits and floats; `number | bigint` for 64-bit integers; `bigint`; `number | VrpcDecimal` for decimals;
`string`; `Uint8Array`; `Date`; arrays; `{ [key: string]: T }`; the interface of an object's definition;
`unknown` for `any`; `| null` where nullable.
