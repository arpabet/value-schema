# value-schema

PDX for the [value](https://github.com/arpabet/value) framework: a type registry, values that carry their type,
and instances whose fields are read without decoding the whole value, as GemFire's PDX type registry,
`PdxInstance`, `read-serialized` and `JSONFormatter` provide. It is the serialization layer of the Gridolith data
grid, and it works on its own.

```
go get go.arpabet.com/value-schema
```

## Values carry their type

A value is a 10-byte header (a marker, the type ID, the length, flags) and value's canonical MessagePack
([FORMAT.md](FORMAT.md)). A Go struct is written with the ID of its definition, which a `Registry` holds:

```go
reg := valueschema.NewMemoryRegistry(0) // or the grid's __TYPES region
codec := valueschema.NewCodec(reg)

b, err := codec.Encode(ctx, Order{ID: "o-1", Qty: 3})   // defines Order on first use
var o Order
err = codec.Decode(ctx, b, &o)
```

Readers match fields by name, so a Go type may gain fields: old values read into the new type with the new
fields at their zero value, and new values read into the old type without them. Numbers convert into Go fields
only without loss.

## Instances

An `Instance` reads one field without decoding the rest, as `PdxInstance` does:

```go
i, err := codec.Open(b)
qty, err := i.Field("qty")       // value.Value; nil when absent
raw := i.FieldBytes("qty")       // the field's MessagePack, no allocation
t, err := i.Type(ctx)            // the definition, from the registry
text, err := i.JSON(value.JSONCanonical)
```

`Decode` into an `any` gives the Go type registered for the definition's name (`Register`), or an `Instance`;
with `ReadSerialized(true)` it always gives an `Instance`.

## JSON and TypeScript

```go
b, err := codec.FromJSON(ctx, "github.com/acme/shop.Order", []byte(`{"id":"o-2","qty":5}`))
// qty is written as the integer Order's definition says, not as an untyped Decimal

decls, err := valueschema.TypeScript(definitions) // interfaces for @arpabet/vrpc readers
```

## License

Business Source License 1.1 ([LICENSE](LICENSE)).
