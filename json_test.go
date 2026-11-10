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
	"time"

	"go.arpabet.com/value"
	vs "go.arpabet.com/value-schema"
)

type Shipment struct {
	ID      string            `value:"id"`
	Count   int16             `value:"count"`
	Weight  float32           `value:"weight"`
	Huge    uint64            `value:"huge"`
	Label   []byte            `value:"label"`
	At      time.Time         `value:"at"`
	Stops   []Address         `value:"stops"`
	Notes   map[string]string `value:"notes"`
	Backup  *Address          `value:"backup"`
	Payload any               `value:"payload"`
}

func TestFromJSONTyped(t *testing.T) {
	ctx := context.Background()
	c := vs.NewCodec(vs.NewMemoryRegistry(0))
	// The Go writer defines the type; a JSON writer then writes per it.
	if _, err := c.Encode(ctx, Shipment{}); err != nil {
		t.Fatal(err)
	}
	name := vs.GoName(reflect.TypeOf(Shipment{}))
	b, err := c.FromJSON(ctx, name, []byte(`{
		"id": "s1", "count": 12, "weight": 1.5, "huge": 18446744073709551615,
		"label": "AQI=", "at": "2026-11-10T08:00:00.5Z",
		"stops": [{"city": "Oslo"}, {"city": "Rome", "zip": "00100"}],
		"notes": {"a": "b"}, "backup": null, "payload": {"free": 1.10}}`))
	if err != nil {
		t.Fatal(err)
	}
	id, _, _ := vs.Split(b)
	if want, _ := c.TypeID(ctx, reflect.TypeOf(Shipment{})); id != want {
		t.Fatalf("type %d, want %d", id, want)
	}
	var s Shipment
	if err := c.Decode(ctx, b, &s); err != nil {
		t.Fatal(err)
	}
	if s.Count != 12 || s.Weight != 1.5 || s.Huge != 18446744073709551615 || string(s.Label) != "\x01\x02" ||
		!s.At.Equal(time.Date(2026, 11, 10, 8, 0, 0, 5e8, time.UTC)) || len(s.Stops) != 2 || s.Stops[1].Zip != "00100" ||
		s.Notes["a"] != "b" || s.Backup != nil {
		t.Fatalf("decoded %+v", s)
	}
	i, _ := c.Open(b)
	count, _ := i.Field("count")
	if count.(value.Number).Type() != value.LONG {
		t.Fatalf("count written as %s", count.(value.Number).Type())
	}
	payload, _ := i.Field("payload")
	if free := payload.(value.Map).GetNumber("free"); free.Type() != value.DECIMAL {
		t.Fatalf("an any field was converted to %s", free.Type())
	}

	for _, bad := range []string{
		`{"id": "s", "unknown": 1}`,
		`{"count": 1.5}`,
		`{"count": 40000}`,
		`{"weight": 0.123456789}`,
		`{"id": null}`,
		`{"id": 5}`,
		`{"stops": [{"city": 1}]}`,
		`{"at": "noon"}`,
		`{"label": "!!"}`,
		`[1]`,
	} {
		if _, err := c.FromJSON(ctx, name, []byte(bad)); !errors.Is(err, vs.ErrConform) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	if _, err := c.FromJSON(ctx, "no.Such", []byte(`{}`)); !errors.Is(err, vs.ErrUnknownType) {
		t.Fatalf("unknown type: %v", err)
	}
}

func TestFromJSONUntyped(t *testing.T) {
	ctx := context.Background()
	c := vs.NewCodec(vs.NewMemoryRegistry(0))
	b, err := c.FromJSON(ctx, "", []byte(`{"qty": 5, "tags": ["a"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if id, _, _ := vs.Split(b); id != vs.Untyped {
		t.Fatalf("typed %d", id)
	}
	text, err := vs.ToJSON(b, value.JSONCanonical)
	if err != nil || string(text) != `{"qty":{"$decimal":"5"},"tags":["a"]}` {
		t.Fatalf("json %s %v", text, err)
	}
}
