package graphql

// This file contains helper functions for resolver implementations
// The actual resolver implementations are in schema.resolvers.go

import (
	"context"
)

// Helper function to get user ID from context. Must read the same typed
// contextKey (ctxKeyUserID, defined in graphql.go) that production request
// handling stores the value under - a bare string key here would never
// match the typed key's interface identity, silently returning 0 (i.e.
// always unauthorized) for every real authenticated request.
func getUserIDFromContext(ctx context.Context) int {
	userID, ok := ctx.Value(ctxKeyUserID).(int)
	if !ok {
		return 0
	}
	return userID
}

// Helper function to create a string pointer
func stringPtr(s string) *string {
	return &s
}
