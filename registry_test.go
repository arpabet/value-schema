/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	vs "go.arpabet.com/value-schema"
)

func strType(name string, fields ...string) vs.Type {
	t := vs.Type{Name: name}
	for _, f := range fields {
		t.Fields = append(t.Fields, vs.Field{Name: f, Type: vs.FieldType{Kind: vs.KindString}})
	}
	return t
}

func TestMemoryRegistry(t *testing.T) {
	ctx := context.Background()
	r := vs.NewMemoryRegistry(5)
	v1 := strType("shop.Order", "id")
	id1, err := r.Define(ctx, v1)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := vs.FirstID(5, v1)
	if id1 != first || id1.DistributedSystem() != 5 {
		t.Fatalf("id %d, first %d", id1, first)
	}
	if again, err := r.Define(ctx, strType("shop.Order", "id")); err != nil || again != id1 {
		t.Fatalf("redefined as %d: %v", again, err)
	}
	id2, err := r.Define(ctx, strType("shop.Order", "id", "note"))
	if err != nil || id2 == id1 {
		t.Fatalf("second version %d: %v", id2, err)
	}
	got, err := r.ByID(ctx, id2)
	if err != nil || !got.Equal(strType("shop.Order", "id", "note")) {
		t.Fatalf("by id: %+v %v", got, err)
	}
	got.Fields[0].Name = "changed" // a copy: the registry keeps its own
	if again, _ := r.ByID(ctx, id2); again.Fields[0].Name != "id" {
		t.Fatal("the registry shares its definitions")
	}
	if id, ok, err := r.IDFor(ctx, v1); err != nil || !ok || id != id1 {
		t.Fatalf("id for: %d %v %v", id, ok, err)
	}
	if _, ok, _ := r.IDFor(ctx, strType("other")); ok {
		t.Fatal("found an undefined definition")
	}
	if _, err := r.ByID(ctx, 12345); !errors.Is(err, vs.ErrUnknownType) {
		t.Fatalf("unknown id: %v", err)
	}
	if _, err := r.Define(ctx, vs.Type{}); !errors.Is(err, vs.ErrDefinition) {
		t.Fatalf("malformed definition: %v", err)
	}
	entries, err := r.Types(ctx)
	if err != nil || len(entries) != 2 {
		t.Fatalf("types %v %v", entries, err)
	}
	latest, ok := vs.Latest(entries, "shop.Order")
	if !ok || latest.ID != id2 || latest.Version != 2 {
		t.Fatalf("latest %+v", latest)
	}
}

func TestAllocateProbes(t *testing.T) {
	typ := strType("T", "a")
	first, err := vs.FirstID(9, typ)
	if err != nil {
		t.Fatal(err)
	}
	// The first two numbers are taken: the third is the ID.
	id, err := vs.Allocate(9, typ, func(id vs.TypeID) (bool, error) {
		return id == first || id == vs.NextID(first), nil
	})
	if err != nil || id != vs.NextID(vs.NextID(first)) {
		t.Fatalf("allocated %d: %v", id, err)
	}
	if vs.NextID(vs.NewTypeID(9, vs.MaxNumber)) != vs.NewTypeID(9, 1) {
		t.Fatal("no wrap to 1")
	}
}

type countingRegistry struct {
	vs.Registry
	byID, define atomic.Int32
}

func (c *countingRegistry) ByID(ctx context.Context, id vs.TypeID) (vs.Type, error) {
	c.byID.Add(1)
	return c.Registry.ByID(ctx, id)
}

func (c *countingRegistry) Define(ctx context.Context, t vs.Type) (vs.TypeID, error) {
	c.define.Add(1)
	return c.Registry.Define(ctx, t)
}

func TestCached(t *testing.T) {
	ctx := context.Background()
	inner := &countingRegistry{Registry: vs.NewMemoryRegistry(0)}
	r := vs.Cached(inner)
	id, err := r.Define(ctx, strType("T", "a"))
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := r.Define(ctx, strType("T", "a")); err != nil {
			t.Fatal(err)
		}
		if _, err := r.ByID(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if inner.define.Load() != 1 || inner.byID.Load() != 0 {
		t.Fatalf("asked the registry %d defines and %d lookups", inner.define.Load(), inner.byID.Load())
	}
}
