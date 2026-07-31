package graphql

// This file contains helper functions for resolver implementations
// The actual resolver implementations are in schema.resolvers.go

import (
	"context"
)

// Helper function to get user ID from context.
func getUserIDFromContext(ctx context.Context) int {
	userID, ok := ctx.Value(ctxKeyUserID).(int)
	if !ok {
		return 0
	}
	return userID
}


// Helper function to get IP from context
func getIPFromContext(ctx context.Context) string {
	ip, ok := ctx.Value(ctxKeyClientIP).(string)
	if !ok {
		return "unknown"
	}
	return ip
}

// Helper function to create a string pointer
func stringPtr(s string) *string {
	return &s
}

