/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema

import (
	"context"
	"hash/fnv"
	"sort"
	"sync"

	"golang.org/x/xerrors"
)

// Registry holds definitions under their IDs, as GemFire's PDX type registry
// does. A definition is never changed or removed, and an ID never names
// another definition, so what a registry answers may be kept forever.
type Registry interface {
	// Define returns the ID of the definition, defining it when the registry
	// holds no identical one.
	Define(ctx context.Context, t Type) (TypeID, error)
	// ByID returns the definition with the ID, or ErrUnknownType.
	ByID(ctx context.Context, id TypeID) (Type, error)
	// IDFor returns the ID of an identical definition, and false when the
	// registry holds none.
	IDFor(ctx context.Context, t Type) (TypeID, bool, error)
	// Types returns every definition the registry holds.
	Types(ctx context.Context) ([]Entry, error)
}

// Entry is a definition in a registry. Version numbers the definitions of
// one name in the order the registry defined them, from 1.
type Entry struct {
	ID      TypeID `value:"id"`
	Version int    `value:"version"`
	Type    Type   `value:"type"`
}

// Latest returns the entry with the highest version for a name.
func Latest(entries []Entry, name string) (Entry, bool) {
	var best Entry
	found := false
	for _, e := range entries {
		if e.Type.Name == name && (!found || e.Version > best.Version) {
			best, found = e, true
		}
	}
	return best, found
}

// FirstID is the ID a registry tries first for a definition, as GemFire
// starts from the hash of a PDX type: the distributed-system ID in the top 8
// bits and the lower 24 bits of the FNV-1a hash of the definition's canonical
// bytes in the rest, or 1 when those are 0.
func FirstID(dsid uint8, t Type) (TypeID, error) {
	b, err := t.Bytes()
	if err != nil {
		return 0, err
	}
	h := fnv.New32a()
	_, _ = h.Write(b)
	n := h.Sum32() & MaxNumber
	if n == 0 {
		n = 1
	}
	return NewTypeID(dsid, n), nil
}

// NextID is the ID tried after a taken one: the next number in the same
// distributed system, wrapping to 1 after MaxNumber.
func NextID(id TypeID) TypeID {
	n := id.Number() + 1
	if n > MaxNumber {
		n = 1
	}
	return NewTypeID(id.DistributedSystem(), n)
}

// Allocate finds the ID for a new definition: FirstID, then NextID until
// taken reports one free. It fails when every number of the distributed
// system is taken.
func Allocate(dsid uint8, t Type, taken func(TypeID) (bool, error)) (TypeID, error) {
	id, err := FirstID(dsid, t)
	if err != nil {
		return 0, err
	}
	for range MaxNumber {
		used, err := taken(id)
		if err != nil {
			return 0, err
		}
		if !used {
			return id, nil
		}
		id = NextID(id)
	}
	return 0, xerrors.Errorf("valueschema: all %d type IDs of distributed system %d are taken", MaxNumber, dsid)
}

// NewMemoryRegistry returns a registry held in memory, for one process or for
// tests, allocating IDs in the distributed system.
func NewMemoryRegistry(dsid uint8) Registry {
	return &memoryRegistry{dsid: dsid, byID: map[TypeID]Entry{}, byBytes: map[string]TypeID{}, versions: map[string]int{}}
}

type memoryRegistry struct {
	dsid     uint8
	mu       sync.RWMutex
	byID     map[TypeID]Entry
	byBytes  map[string]TypeID
	versions map[string]int
}

func (r *memoryRegistry) Define(_ context.Context, t Type) (TypeID, error) {
	if err := t.Validate(); err != nil {
		return 0, err
	}
	b, err := t.Bytes()
	if err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.byBytes[string(b)]; ok {
		return id, nil
	}
	id, err := Allocate(r.dsid, t, func(id TypeID) (bool, error) {
		_, ok := r.byID[id]
		return ok, nil
	})
	if err != nil {
		return 0, err
	}
	r.versions[t.Name]++
	r.byID[id] = Entry{ID: id, Version: r.versions[t.Name], Type: t.Clone()}
	r.byBytes[string(b)] = id
	return id, nil
}

func (r *memoryRegistry) ByID(_ context.Context, id TypeID) (Type, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byID[id]
	if !ok {
		return Type{}, xerrors.Errorf("valueschema: type %d: %w", id, ErrUnknownType)
	}
	return e.Type.Clone(), nil
}

func (r *memoryRegistry) IDFor(_ context.Context, t Type) (TypeID, bool, error) {
	b, err := t.Bytes()
	if err != nil {
		return 0, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byBytes[string(b)]
	return id, ok, nil
}

func (r *memoryRegistry) Types(context.Context) ([]Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, 0, len(r.byID))
	for _, e := range r.byID {
		e.Type = e.Type.Clone()
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Cached returns a registry that keeps what r answers: definitions and IDs
// never change, so Define, ByID and IDFor ask r once per definition. Types
// always asks r.
func Cached(r Registry) Registry {
	return &cachedRegistry{r: r}
}

type cachedRegistry struct {
	r       Registry
	byID    sync.Map // TypeID → Type
	byBytes sync.Map // string → TypeID
}

func (c *cachedRegistry) Define(ctx context.Context, t Type) (TypeID, error) {
	b, err := t.Bytes()
	if err != nil {
		return 0, err
	}
	if id, ok := c.byBytes.Load(string(b)); ok {
		return id.(TypeID), nil
	}
	id, err := c.r.Define(ctx, t)
	if err != nil {
		return 0, err
	}
	c.byBytes.Store(string(b), id)
	c.byID.Store(id, t.Clone())
	return id, nil
}

func (c *cachedRegistry) ByID(ctx context.Context, id TypeID) (Type, error) {
	if t, ok := c.byID.Load(id); ok {
		return t.(Type).Clone(), nil
	}
	t, err := c.r.ByID(ctx, id)
	if err != nil {
		return Type{}, err
	}
	c.byID.Store(id, t.Clone())
	return t, nil
}

func (c *cachedRegistry) IDFor(ctx context.Context, t Type) (TypeID, bool, error) {
	b, err := t.Bytes()
	if err != nil {
		return 0, false, err
	}
	if id, ok := c.byBytes.Load(string(b)); ok {
		return id.(TypeID), true, nil
	}
	id, ok, err := c.r.IDFor(ctx, t)
	if err != nil || !ok {
		return 0, ok, err
	}
	c.byBytes.Store(string(b), id)
	return id, true, nil
}

func (c *cachedRegistry) Types(ctx context.Context) ([]Entry, error) {
	entries, err := c.r.Types(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		c.byID.LoadOrStore(e.ID, e.Type.Clone())
	}
	return entries, nil
}
