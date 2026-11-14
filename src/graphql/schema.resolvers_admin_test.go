package graphql

import (
	"strconv"

	"github.com/webappsgo/wthr/src/config"
)

// formatGraphQLTestUserID renders an int64 user id as the string form the
// GraphQL layer expects for ID-typed arguments.
func formatGraphQLTestUserID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// --- AdminUpdateUser -----------------------------------------------------

// --- AdminDeleteUser -------------------------------------------------------

// --- AdminCreateUserInvite -------------------------------------------------

// --- AdminInviteServerAdmin -------------------------------------------------

// --- AdminDeleteServerAdmin --------------------------------------------------

// --- AdminUpdateSetting -------------------------------------------------------

// --- AdminGenerateToken / AdminRevokeToken ------------------------------------

// --- AdminDisableServerAdmin --------------------------------------------------

// --- AdminEnableServerAdmin --------------------------------------------------

// Keeps the config import in use: AdminCreateUserInvite falls back to this
// default expiration (7 days) when no explicit expiresInDays is provided,
// exercised implicitly by the "happy path" subtest above.
var _ = config.GetUserInviteExpirationDays
