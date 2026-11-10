# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/), and the project aims to follow
[Semantic Versioning](https://semver.org/).

## v0.1.0 — unreleased

First release ([FORMAT.md](FORMAT.md) version 1).

- The value header: marker `0xc1`, type ID, length and flags before value's
  canonical MessagePack; `Wrap`, `Split`, `Normalize` for bare MessagePack
  written before it.
- Definitions derived from Go structs as `value.Marshal` maps them (`TypeOf`),
  with nested struct definitions, validated and compared by their canonical
  bytes.
- `Registry` with an in-memory implementation and a caching wrapper; type IDs
  allocated as GemFire allocates PDX type IDs (distributed-system ID, hash of
  the definition, linear probing); versions per name.
- `Codec`: `Encode` defines struct types on first use; `Decode` reads any
  version into any Go type, losslessly; `Register` and `ReadSerialized` decide
  what an `any` destination gets.
- `Instance`: single fields located with tinylib/msgp without allocating,
  field names, the definition, lossless JSON.
- `FromJSON` with `Conform` (typed writers without a Go type), `ToJSON`.
- `TypeScript` declarations.
- `FuzzSplit`, `FuzzInstance`.
