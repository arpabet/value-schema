/*
 * Copyright (c) 2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package valueschema_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	vs "go.arpabet.com/value-schema"
)

func TestHeaderLayout(t *testing.T) {
	b := vs.Wrap(vs.NewTypeID(3, 0x0a0b0c), []byte{0x91, 0x01})
	if got, want := hex.EncodeToString(b), "c1030a0b0c00000002"+"00"+"9101"; got != want {
		t.Fatalf("bytes %s, want %s", got, want)
	}
	id, payload, err := vs.Split(b)
	if err != nil {
		t.Fatal(err)
	}
	if id.DistributedSystem() != 3 || id.Number() != 0x0a0b0c || !bytes.Equal(payload, []byte{0x91, 0x01}) {
		t.Fatalf("split %d %x", id, payload)
	}
	h, err := vs.ParseHeader(b)
	if err != nil || h.Length != 2 || h.Flags != 0 || h.Type != id {
		t.Fatalf("header %+v %v", h, err)
	}
}

func TestHeaderRefused(t *testing.T) {
	good := vs.Wrap(7, []byte{0xc0})
	for name, b := range map[string][]byte{
		"empty":     nil,
		"short":     good[:9],
		"no marker": append([]byte{0xc0}, good[1:]...),
		"flags":     append(append([]byte{}, good[:9]...), append([]byte{1}, good[10:]...)...),
		"too long":  append(append([]byte{}, good...), 0xc0),
		"too short": good[:len(good)-1],
	} {
		if _, _, err := vs.Split(b); !errors.Is(err, vs.ErrHeader) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNormalize(t *testing.T) {
	bare := []byte{0x81, 0xa1, 0x61, 0x01} // {"a": 1}, as stored before the header
	b, err := vs.Normalize(bare)
	if err != nil {
		t.Fatal(err)
	}
	id, payload, err := vs.Split(b)
	if err != nil || id != vs.Untyped || !bytes.Equal(payload, bare) {
		t.Fatalf("normalized %x: %d %v", b, id, err)
	}
	again, err := vs.Normalize(b)
	if err != nil || !bytes.Equal(again, b) {
		t.Fatalf("normalizing twice: %x %v", again, err)
	}
	if _, err := vs.Normalize(b[:5]); !errors.Is(err, vs.ErrHeader) {
		t.Fatalf("a broken header normalized: %v", err)
	}
	if _, err := vs.Normalize(nil); !errors.Is(err, vs.ErrHeader) {
		t.Fatalf("nothing normalized: %v", err)
	}
}

func FuzzSplit(f *testing.F) {
	f.Add(vs.Wrap(1, []byte{0x80}))
	f.Add([]byte{0xc1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		id, payload, err := vs.Split(b)
		if err != nil {
			return
		}
		if !bytes.Equal(vs.Wrap(id, payload), b) {
			t.Fatalf("%x does not rewrap", b)
		}
	})
}
