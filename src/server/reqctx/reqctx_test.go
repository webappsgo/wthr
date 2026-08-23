package reqctx

import (
	"context"
	"testing"
)

// TestSetGet verifies a value stored under a key is retrievable, and an
// absent key reports ok=false.
func TestSetGet(t *testing.T) {
	ctx := Set(context.Background(), "key", "value")

	got, ok := Get(ctx, "key")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got != "value" {
		t.Errorf("Get() = %v, want value", got)
	}

	if _, ok := Get(ctx, "missing"); ok {
		t.Error("Get() for missing key: ok = true, want false")
	}
}

// TestGet_EmptyContext verifies Get on a context with nothing set returns
// (nil, false).
func TestGet_EmptyContext(t *testing.T) {
	got, ok := Get(context.Background(), "key")
	if ok {
		t.Error("Get() on empty context: ok = true, want false")
	}
	if got != nil {
		t.Errorf("Get() on empty context = %v, want nil", got)
	}
}

// TestSet_Overwrite verifies setting the same key twice yields the latest
// value from the most recently derived context.
func TestSet_Overwrite(t *testing.T) {
	ctx := Set(context.Background(), "key", "first")
	ctx = Set(ctx, "key", "second")

	got, ok := Get(ctx, "key")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got != "second" {
		t.Errorf("Get() = %v, want second", got)
	}
}

// TestMustGet verifies MustGet returns the stored value and panics when the
// key is absent.
func TestMustGet(t *testing.T) {
	ctx := Set(context.Background(), "key", 42)
	if got := MustGet(ctx, "key"); got != 42 {
		t.Errorf("MustGet() = %v, want 42", got)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustGet() on missing key did not panic")
		}
	}()
	MustGet(context.Background(), "missing")
}

// TestGetString covers a stored string, a missing key, and a wrong type.
func TestGetString(t *testing.T) {
	ctx := Set(context.Background(), "key", "value")
	if got := GetString(ctx, "key"); got != "value" {
		t.Errorf("GetString() = %q, want value", got)
	}

	if got := GetString(context.Background(), "missing"); got != "" {
		t.Errorf("GetString() on missing key = %q, want empty", got)
	}

	wrongType := Set(context.Background(), "key", 42)
	if got := GetString(wrongType, "key"); got != "" {
		t.Errorf("GetString() on wrong type = %q, want empty", got)
	}
}

// TestGetInt covers a stored int, a missing key, and a wrong type.
func TestGetInt(t *testing.T) {
	ctx := Set(context.Background(), "key", 42)
	if got := GetInt(ctx, "key"); got != 42 {
		t.Errorf("GetInt() = %d, want 42", got)
	}

	if got := GetInt(context.Background(), "missing"); got != 0 {
		t.Errorf("GetInt() on missing key = %d, want 0", got)
	}

	wrongType := Set(context.Background(), "key", "not-an-int")
	if got := GetInt(wrongType, "key"); got != 0 {
		t.Errorf("GetInt() on wrong type = %d, want 0", got)
	}
}

// TestGetBool covers a stored bool, a missing key, and a wrong type.
func TestGetBool(t *testing.T) {
	ctx := Set(context.Background(), "key", true)
	if got := GetBool(ctx, "key"); !got {
		t.Error("GetBool() = false, want true")
	}

	if got := GetBool(context.Background(), "missing"); got {
		t.Error("GetBool() on missing key = true, want false")
	}

	wrongType := Set(context.Background(), "key", "not-a-bool")
	if got := GetBool(wrongType, "key"); got {
		t.Error("GetBool() on wrong type = true, want false")
	}
}

// TestKeyIsolation verifies keys placed via this package's unexported ctxKey
// type do not collide with plain string keys set directly on the context.
func TestKeyIsolation(t *testing.T) {
	//lint:ignore SA1029 intentionally using a raw string key to prove isolation from ctxKey
	ctx := context.WithValue(context.Background(), "key", "raw-string-key-value")

	if _, ok := Get(ctx, "key"); ok {
		t.Error("Get() found a value set with a raw string key; ctxKey isolation broken")
	}
}
