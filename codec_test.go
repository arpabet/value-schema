/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/shopspring/decimal"
	"go.arpabet.com/value"
	vs "go.arpabet.com/value-schema"
)

type Item struct {
	SKU   string          `value:"sku"`
	Qty   int             `value:"qty"`
	Price decimal.Decimal `value:"price"`
	Ship  *Address        `value:"ship"`
}

// ItemV1 and ItemV2 are two versions of a type: ItemV2 gained a field.
type ItemV1 struct {
	SKU string `value:"sku"`
}

type ItemV2 struct {
	SKU   string `value:"sku"`
	Color string `value:"color"`
}

func TestEncodeDecode(t *testing.T) {
	ctx := context.Background()
	reg := vs.NewMemoryRegistry(0)
	c := vs.NewCodec(reg)
	in := Item{SKU: "a-1", Qty: 3, Price: decimal.RequireFromString("9.90"), Ship: &Address{City: "Oslo"}}
	b, err := c.Encode(ctx, &in)
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := vs.Split(b)
	if err != nil || id == vs.Untyped {
		t.Fatalf("type %d: %v", id, err)
	}
	typ, err := reg.ByID(ctx, id)
	if err != nil || typ.Name != vs.GoName(reflect.TypeOf(in)) {
		t.Fatalf("definition %+v: %v", typ, err)
	}
	entries, _ := reg.Types(ctx)
	if _, ok := vs.Latest(entries, vs.GoName(reflect.TypeOf(Address{}))); !ok || len(entries) != 2 {
		t.Fatalf("nested definition not defined: %v", entries)
	}
	var out Item
	if err := c.Decode(ctx, b, &out); err != nil {
		t.Fatal(err)
	}
	if out.SKU != "a-1" || out.Qty != 3 || out.Price.String() != "9.9" || out.Ship == nil || out.Ship.City != "Oslo" {
		t.Fatalf("decoded %+v", out)
	}

	// Values that are not structs are untyped.
	for _, v := range []any{"text", 5, map[string]int{"a": 1}, nil, value.Long(1)} {
		b, err := c.Encode(ctx, v)
		if err != nil {
			t.Fatal(err)
		}
		if id, _, _ := vs.Split(b); id != vs.Untyped {
			t.Fatalf("%T typed %d", v, id)
		}
	}
}

func TestSchemaEvolution(t *testing.T) {
	ctx := context.Background()
	c := vs.NewCodec(vs.NewMemoryRegistry(0))
	old, err := c.Encode(ctx, ItemV1{SKU: "s"})
	if err != nil {
		t.Fatal(err)
	}
	var newer ItemV2
	if err := c.Decode(ctx, old, &newer); err != nil || newer != (ItemV2{SKU: "s"}) {
		t.Fatalf("a newer reader: %+v %v", newer, err)
	}
	b, err := c.Encode(ctx, ItemV2{SKU: "s", Color: "red"})
	if err != nil {
		t.Fatal(err)
	}
	var older ItemV1
	if err := c.Decode(ctx, b, &older); err != nil || older != (ItemV1{SKU: "s"}) {
		t.Fatalf("an older reader: %+v %v", older, err)
	}
}

func TestDecodeLossless(t *testing.T) {
	ctx := context.Background()
	c := vs.NewCodec(vs.NewMemoryRegistry(0))
	b, err := c.FromJSON(ctx, "", []byte(`{"sku":"x","qty":2.5}`))
	if err != nil {
		t.Fatal(err)
	}
	var out Item
	if err := c.Decode(ctx, b, &out); err == nil {
		t.Fatalf("2.5 decoded into an int as %d", out.Qty)
	}
	if b, err = c.FromJSON(ctx, "", []byte(`{"sku":"x","qty":2}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.Decode(ctx, b, &out); err != nil || out.Qty != 2 {
		t.Fatalf("decoded %+v: %v", out, err)
	}
}

func TestDecodeIntoAny(t *testing.T) {
	ctx := context.Background()
	reg := vs.NewMemoryRegistry(0)
	writer := vs.NewCodec(reg)
	b, err := writer.Encode(ctx, Address{City: "Rome"})
	if err != nil {
		t.Fatal(err)
	}
	// The writer knows the Go type of what it wrote.
	var v any
	if err := writer.Decode(ctx, b, &v); err != nil {
		t.Fatal(err)
	}
	if a, ok := v.(Address); !ok || a.City != "Rome" {
		t.Fatalf("decoded %T %v", v, v)
	}
	// A reader that does not know it reads an instance, until it is told.
	reader := vs.NewCodec(reg)
	if err := reader.Decode(ctx, b, &v); err != nil {
		t.Fatal(err)
	}
	if i, ok := v.(vs.Instance); !ok || i.ID() == vs.Untyped {
		t.Fatalf("decoded %T", v)
	}
	if err := reader.Register(&Address{}); err != nil {
		t.Fatal(err)
	}
	if err := reader.Decode(ctx, b, &v); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(Address); !ok {
		t.Fatalf("decoded %T after Register", v)
	}
	// Read serialized: always an instance.
	serialized := vs.NewCodec(reg, vs.ReadSerialized(true))
	_ = serialized.Register(Address{})
	if err := serialized.Decode(ctx, b, &v); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(vs.Instance); !ok {
		t.Fatalf("read serialized gave %T", v)
	}
	// An untyped value is its tree.
	u, _ := vs.EncodeUntyped(map[string]string{"a": "b"})
	if err := reader.Decode(ctx, u, &v); err != nil {
		t.Fatal(err)
	}
	if m, ok := v.(value.Map); !ok || m.GetString("a").Utf8() != "b" {
		t.Fatalf("untyped gave %T", v)
	}
	if err := reader.Register(5); !errors.Is(err, vs.ErrNotTyped) {
		t.Fatalf("registered an int: %v", err)
	}
}
