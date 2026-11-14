package graphql

import (
	"strconv"

	"github.com/webappsgo/wthr/src/server/handler"
)

// mapGraphQLPasskey converts a handler.PasskeySummary into the GraphQL
// UserPasskey type. ID is stringified to match the graphql ID! contract used
// elsewhere in this schema.
func mapGraphQLPasskey(summary *handler.PasskeySummary) *UserPasskey {
	if summary == nil || summary.Raw == nil {
		return nil
	}
	out := &UserPasskey{
		ID:        strconv.FormatInt(summary.Raw.ID, 10),
		Name:      summary.Raw.Name,
		CreatedAt: summary.Raw.CreatedAt,
	}
	if summary.Raw.LastUsedAt != nil {
		v := *summary.Raw.LastUsedAt
		out.LastUsedAt = &v
	}
	return out
}

// mapGraphQLAdminPasskey converts a handler.AdminPasskeySummary into the
// GraphQL AdminPasskey type.
func mapGraphQLAdminPasskey(summary *handler.AdminPasskeySummary) *AdminPasskey {
	if summary == nil {
		return nil
	}
	out := &AdminPasskey{
		ID:        strconv.FormatInt(summary.ID, 10),
		Name:      summary.Name,
		CreatedAt: summary.CreatedAt,
	}
	if summary.LastUsedAt != nil {
		v := *summary.LastUsedAt
		out.LastUsedAt = &v
	}
	return out
}
