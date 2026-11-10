/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"go.arpabet.com/value"
	vs "go.arpabet.com/value-schema"
)

func TestInstance(t *testing.T) {
	ctx := context.Background()
	c := vs.NewCodec(vs.NewMemoryRegistry(1))
	b, err := c.Encode(ctx, Item{SKU: "k", Qty: 7, Price: decimal.RequireFromString("1.10")})
	if err != nil {
		t.Fatal(err)
	}
	i, err := c.Open(b)
	if err != nil {
		t.Fatal(err)
	}
	names, err := i.FieldNames()
	if err != nil || strings.Join(names, ",") != "price,qty,ship,sku" {
		t.Fatalf("names %v %v", names, err)
	}
	qty, err := i.Field("qty")
	if err != nil || qty.(value.Number).Long() != 7 {
		t.Fatalf("qty %v %v", qty, err)
	}
	if v, err := i.Field("missing"); v != nil || err != nil || i.HasField("missing") {
		t.Fatalf("missing field: %v %v", v, err)
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = i.FieldBytes("sku") }); allocs != 0 {
		t.Fatalf("FieldBytes allocates %v times", allocs)
	}
	typ, err := i.Type(ctx)
	if err != nil || len(typ.Fields) != 4 {
		t.Fatalf("type %+v %v", typ, err)
	}
	var out Item
	if err := i.Decode(&out); err != nil || out.Qty != 7 {
		t.Fatalf("decode %+v %v", out, err)
	}
	text, err := i.JSON(value.JSONCanonical)
	if err != nil || string(text) != `{"price":{"$decimal":"1.10"},"qty":{"$long":"7"},"ship":null,"sku":"k"}` {
		t.Fatalf("json %s %v", text, err)
	}
	if i.String() != `{"price":1.10,"qty":7,"ship":null,"sku":"k"}` {
		t.Fatalf("string %s", i.String())
	}
	if again, _ := vs.Open(b); !again.Equal(i) {
		t.Fatal("not equal to itself")
	}
	if _, err := (vs.Instance{}).Type(ctx); err == nil {
		t.Fatal("the zero instance has a type")
	}
	bare, _ := vs.Open(b)
	if _, err := bare.Type(ctx); !errors.Is(err, vs.ErrNoRegistry) {
		t.Fatalf("type without a registry: %v", err)
	}
	if _, err := vs.Open(append(append([]byte{}, b...), 0xc0)); err == nil {
		t.Fatal("opened a payload with trailing bytes")
	}
	trailing := vs.Wrap(0, []byte{0xc0, 0xc0})
	if _, err := vs.Open(trailing); err == nil {
		t.Fatal("opened two values as one")
	}
}

func FuzzInstance(f *testing.F) {
	c := vs.NewCodec(vs.NewMemoryRegistry(0))
	b, _ := c.Encode(context.Background(), Item{SKU: "x", Ship: &Address{City: "y"}})
	f.Add(b)
	f.Add(vs.Wrap(0, []byte{0x81, 0xa1, 0x61, 0xc0}))
	f.Fuzz(func(t *testing.T, b []byte) {
		i, err := vs.Open(b)
		if err != nil {
			return
		}
		names, err := i.FieldNames()
		if err != nil {
			return
		}
		for _, n := range names {
			_ = i.FieldBytes(n)
			_, _ = i.Field(n)
		}
		_, _ = i.JSON(value.JSONCanonical)
	})
}
