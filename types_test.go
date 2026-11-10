/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema_test

import (
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.arpabet.com/value"
	vs "go.arpabet.com/value-schema"
)

type Address struct {
	City string `value:"city"`
	Zip  string `value:"zip,omitempty"`
}

type Base struct {
	Created time.Time `value:"created"`
}

type Node struct {
	Name string `value:"name"`
	Next *Node  `value:"next"`
}

type Order struct {
	Base
	ID       string              `value:"id"`
	Qty      int32               `value:"qty"`
	Total    int                 `value:"total"`
	Price    decimal.Decimal     `value:"price"`
	Big      big.Int             `value:"big"`
	Ratio    float64             `value:"ratio,omitempty"`
	Raw      []byte              `value:"raw"`
	Tags     []string            `value:"tags"`
	Ship     *Address            `value:"ship"`
	Lines    []Address           `value:"lines"`
	ByCode   map[string]*Address `value:"by_code"`
	Extra    value.Value         `value:"extra"`
	Any      any                 `value:"any"`
	Anon     struct{ X int }     `value:"anon"`
	Chain    Node                `value:"chain"`
	Skipped  string              `value:"-"`
	internal int
}

func TestTypeOf(t *testing.T) {
	typ, nested, err := vs.TypeOf(reflect.TypeOf(&Order{}))
	if err != nil {
		t.Fatal(err)
	}
	if typ.Name != "go.arpabet.com/value-schema_test.Order" {
		t.Fatalf("name %s", typ.Name)
	}
	var got []string
	for _, f := range typ.Fields {
		got = append(got, describe(f))
	}
	want := []string{
		"Base object(go.arpabet.com/value-schema_test.Base)",
		"anon any",
		"any any null",
		"big bigint",
		"by_code map(object(go.arpabet.com/value-schema_test.Address) null)",
		"chain object(go.arpabet.com/value-schema_test.Node)",
		"extra any null",
		"id string",
		"lines list(object(go.arpabet.com/value-schema_test.Address))",
		"price decimal",
		"qty int32",
		"ratio float64 optional",
		"raw bytes",
		"ship object(go.arpabet.com/value-schema_test.Address) null",
		"tags list(string)",
		"total int64",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("fields\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var names []string
	for _, n := range nested {
		names = append(names, n.Name[strings.LastIndexByte(n.Name, '.')+1:])
	}
	if strings.Join(names, ",") != "Base,Address,Node" {
		t.Fatalf("nested %v", names)
	}
	if err := typ.Validate(); err != nil {
		t.Fatal(err)
	}
	b, err := typ.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	back, err := vs.ParseType(b)
	if err != nil || !back.Equal(typ) {
		t.Fatalf("definition round trip: %v", err)
	}
	_ = Order{internal: 1}
}

func describe(f vs.Field) string {
	s := f.Name + " " + describeType(f.Type)
	if f.Optional {
		s += " optional"
	}
	return s
}

func describeType(ft vs.FieldType) string {
	s := string(ft.Kind)
	switch ft.Kind {
	case vs.KindList, vs.KindMap:
		s += "(" + describeType(*ft.Elem) + ")"
	case vs.KindObject:
		s += "(" + ft.Object + ")"
	}
	if ft.Nullable {
		s += " null"
	}
	return s
}

type badKey struct {
	M map[int]string `value:"m"`
}

type badChan struct {
	C chan int `value:"c"`
}

type twice struct {
	A string `value:"x"`
	B string `value:"x"`
}

func TestTypeOfRefused(t *testing.T) {
	for _, v := range []any{badKey{}, badChan{}, twice{}, time.Time{}, decimal.Decimal{}, 5, struct{ A int }{}} {
		if _, _, err := vs.TypeOf(reflect.TypeOf(v)); !errors.Is(err, vs.ErrNotTyped) {
			t.Errorf("%T: %v", v, err)
		}
	}
}

func TestValidate(t *testing.T) {
	str := vs.FieldType{Kind: vs.KindString}
	for name, typ := range map[string]vs.Type{
		"no name":      {Fields: []vs.Field{{Name: "a", Type: str}}},
		"unsorted":     {Name: "T", Fields: []vs.Field{{Name: "b", Type: str}, {Name: "a", Type: str}}},
		"repeated":     {Name: "T", Fields: []vs.Field{{Name: "a", Type: str}, {Name: "a", Type: str}}},
		"unknown kind": {Name: "T", Fields: []vs.Field{{Name: "a", Type: vs.FieldType{Kind: "complex"}}}},
		"list no elem": {Name: "T", Fields: []vs.Field{{Name: "a", Type: vs.FieldType{Kind: vs.KindList}}}},
		"object name":  {Name: "T", Fields: []vs.Field{{Name: "a", Type: vs.FieldType{Kind: vs.KindObject}}}},
		"stray elem":   {Name: "T", Fields: []vs.Field{{Name: "a", Type: vs.FieldType{Kind: vs.KindBool, Elem: &str}}}},
		"empty field":  {Name: "T", Fields: []vs.Field{{Type: str}}},
	} {
		if err := typ.Validate(); !errors.Is(err, vs.ErrDefinition) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
