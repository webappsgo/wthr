// Package privilege implements the root phase of AI.md PART 8 startup (steps 8a-8i):
// the dedicated service account, ownership of the service directories, the binding
// of privileged ports and the drop from root to that account.
package privilege

// InternalOrg and InternalName are the frozen on-disk identity of this service
// (AI.md PART 4: {internal_org} and {internal_name}). They never change when the
// project is renamed, and they name the service account and every system path.
const (
	InternalOrg  = "webappsgo"
	InternalName = "wthr"
)

// PrivilegedPortLimit is the first port a non-root process may bind.
const PrivilegedPortLimit = 1024

// Account is the dedicated service account. The UID and the GID are always the same
// value (AI.md PART 24 "System User Requirements").
type Account struct {
	Name string
	UID  int
	GID  int
	Home string
}
