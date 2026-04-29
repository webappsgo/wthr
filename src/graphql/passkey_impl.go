package graphql

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/apimgr/weather/src/server/handler"
)

// graphQLPasskeyEnvelope reads the host/scheme stored on the request context
// (see graphql.buildGraphQLAuthContext) and returns the envelope expected by
// the passkey helper functions in the handler package.
func graphQLPasskeyEnvelope(ctx context.Context) (handler.PasskeyEnvelope, error) {
	host, _ := ctx.Value("request_host").(string)
	host = strings.TrimSpace(host)
	if host == "" {
		return handler.PasskeyEnvelope{}, fmt.Errorf("missing request host")
	}

	scheme, _ := ctx.Value("request_scheme").(string)
	scheme = strings.TrimSpace(scheme)
	https := strings.EqualFold(scheme, "https")

	return handler.PasskeyEnvelope{Host: host, HTTPS: https}, nil
}

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
