// Package reqctx provides typed, string-keyed request-context storage used
// by chi middleware and handlers, replacing gin.Context's Set/Get/MustGet
// per-request value store.
package reqctx

import "context"

// ctxKey is an unexported type so keys placed here never collide with
// context keys set by other packages.
type ctxKey string

// Set stores value under key in a derived context, mirroring the semantics
// of gin.Context.Set.
func Set(ctx context.Context, key string, value interface{}) context.Context {
	return context.WithValue(ctx, ctxKey(key), value)
}

// Get retrieves the value stored under key, mirroring gin.Context.Get.
// The second return value reports whether the key was present.
func Get(ctx context.Context, key string) (interface{}, bool) {
	v := ctx.Value(ctxKey(key))
	if v == nil {
		return nil, false
	}
	return v, true
}

// MustGet retrieves the value stored under key, panicking if the key is
// absent, mirroring gin.Context.MustGet.
func MustGet(ctx context.Context, key string) interface{} {
	v, ok := Get(ctx, key)
	if !ok {
		panic("reqctx: key \"" + key + "\" does not exist")
	}
	return v
}

// GetString retrieves a string value stored under key, returning "" when
// the key is absent or not a string, mirroring gin.Context.GetString.
func GetString(ctx context.Context, key string) string {
	v, ok := Get(ctx, key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// GetInt retrieves an int value stored under key, returning 0 when the key
// is absent or not an int, mirroring gin.Context.GetInt.
func GetInt(ctx context.Context, key string) int {
	v, ok := Get(ctx, key)
	if !ok {
		return 0
	}
	i, _ := v.(int)
	return i
}

// GetBool retrieves a bool value stored under key, returning false when the
// key is absent or not a bool, mirroring gin.Context.GetBool.
func GetBool(ctx context.Context, key string) bool {
	v, ok := Get(ctx, key)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}
