package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/server/model"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

func TestAdminIdentityContext(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  interface{}
		want int64
	}{
		{name: "int id", key: "admin_id", val: 17, want: 17},
		{name: "int64 id", key: "admin_id", val: int64(18), want: 18},
		{name: "admin record fallback", key: "admin_id", val: "invalid", want: 19},
		{name: "unexpected id type", key: "admin_id", val: "20"},
		{name: "nil admin", key: "admin", val: (*model.Admin)(nil)},
		{name: "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tt.key != "" {
				r = r.WithContext(reqctx.SetValue(r.Context(), tt.key, tt.val))
			}
			if tt.name == "admin record fallback" {
				r = r.WithContext(reqctx.SetValue(r.Context(), "admin", &model.Admin{ID: tt.want}))
			}
			if got := AdminID(r); got != tt.want {
				t.Fatalf("AdminID() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAdminUsernameContextPrecedenceAndFallback(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]interface{}
		want   string
	}{
		{name: "username key", values: map[string]interface{}{adminUsernameKey: "direct", "admin": &model.Admin{Username: "record"}}, want: "direct"},
		{name: "admin record", values: map[string]interface{}{"admin": &model.Admin{Username: "record"}}, want: "record"},
		{name: "empty username key falls through", values: map[string]interface{}{adminUsernameKey: "", "admin": &model.Admin{Username: "record"}}, want: "record"},
		{name: "no database", values: map[string]interface{}{"admin_id": 21}},
		{name: "unsupported id type", values: map[string]interface{}{"admin_id": uint(1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			ctx := r.Context()
			for key, value := range tt.values {
				ctx = reqctx.SetValue(ctx, key, value)
			}
			r = r.WithContext(ctx)
			if got := AdminUsername(r); got != tt.want {
				t.Fatalf("AdminUsername() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAdminSelfMatches(t *testing.T) {
	for _, tt := range []struct {
		name, requested, current string
		want                     bool
	}{
		{name: "same", requested: "owner", current: "owner", want: true},
		{name: "different", requested: "other", current: "owner"},
		{name: "missing route value", current: "owner"},
		{name: "missing identity", requested: "owner"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			ctx := context.Background()
			if tt.current != "" {
				ctx = reqctx.SetValue(ctx, adminUsernameKey, tt.current)
			}
			rctx := chi.NewRouteContext()
			if tt.requested != "" {
				rctx.URLParams.Add("admin_username", tt.requested)
			}
			ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
			r = r.WithContext(ctx)
			if got := adminSelfMatches(r); got != tt.want {
				t.Fatalf("adminSelfMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}
