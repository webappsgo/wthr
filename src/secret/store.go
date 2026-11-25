// Package secret holds the project-level cryptographic secrets of AI.md PART 11
// "Cryptographic Keys": installation_secret, cookie_signing_key and csrf_token_secret.
// They live in server.db (table app_secrets), are included in every backup and are
// never logged or returned in plaintext by any API; cluster enrollment carries
// the rows only inside the token-authenticated encrypted bootstrap payload.
package secret

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/webappsgo/wthr/src/database"
)

// Names of the app_secrets rows.
const (
	InstallationSecret = "installation_secret"
	CookieSigningKey   = "cookie_signing_key"
	CSRFTokenSecret    = "csrf_token_secret"
)

const (
	// keyBytes is the length of every secret: 32 bytes (256 bits) of crypto/rand
	keyBytes = 32
	// previousSuffix and untilSuffix name the rows that keep the previous value and its expiry
	previousSuffix = "_previous"
	untilSuffix    = "_previous_until"
	// InstallationGrace and CookieGrace keep the previous value valid for 7 days (PART 11 "Rotation contract")
	InstallationGrace = 7 * 24 * time.Hour
	CookieGrace       = 7 * 24 * time.Hour
	// timestampLayout is the text form of created_at and rotated_at, the layout SQLite's own
	// CURRENT_TIMESTAMP produces, so every row parses the same way
	timestampLayout = "2006-01-02 15:04:05"
	// CookieMaxAge and CSRFMaxAge are the automatic rotation intervals: 90 and 180 days
	CookieMaxAge = 90 * 24 * time.Hour
	CSRFMaxAge   = 180 * 24 * time.Hour
)

// ErrRotationPending is returned when a second rotation is requested while rows that
// depend on the previous value have not all been re-encrypted yet.
var ErrRotationPending = errors.New("a previous value is still in use by unfinished rows")

// ErrUnknownSecret is returned for a name that is not a project-level secret.
var ErrUnknownSecret = errors.New("unknown secret")

// Store reads and rotates the secrets in app_secrets.
type Store struct {
	db    *database.Store
	now   func() time.Time
	mutex sync.Mutex
}

// NewStore returns a store over the server database.
func NewStore(db *database.Store) *Store {
	return &Store{db: db, now: time.Now}
}

var (
	defaultMutex sync.RWMutex
	defaultStore *Store
)

// SetDefault installs the process-wide store used by middleware that has no handle.
func SetDefault(store *Store) {
	defaultMutex.Lock()
	defaultStore = store
	defaultMutex.Unlock()
}

// Default returns the process-wide store, or nil before startup installed one.
func Default() *Store {
	defaultMutex.RLock()
	defer defaultMutex.RUnlock()
	return defaultStore
}

// isKnown reports whether name is a project-level secret handled by this store.
func isKnown(name string) bool {
	switch name {
	case InstallationSecret, CookieSigningKey, CSRFTokenSecret:
		return true
	}
	return false
}

// graceFor returns how long a previous value of name stays valid after rotation.
func graceFor(name string) time.Duration {
	switch name {
	case InstallationSecret:
		return InstallationGrace
	case CookieSigningKey:
		return CookieGrace
	}
	return 0
}

// generate returns 32 bytes from crypto/rand.
func generate() ([]byte, error) {
	value := make([]byte, keyBytes)
	if _, err := rand.Read(value); err != nil {
		return nil, fmt.Errorf("generating secret: %w", err)
	}
	return value, nil
}

// read returns the decoded value of one row, or sql.ErrNoRows.
func (s *Store) read(name string) ([]byte, error) {
	var encoded string
	err := database.QueryRowContext(context.Background(), s.db, database.TimeoutSimpleSelect,
		`SELECT value FROM app_secrets WHERE name = ?`, name).Scan(&encoded)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(encoded)
}

// write stores one row, replacing any value and stamping rotated_at.
func (s *Store) write(name string, value []byte) error {
	_, err := database.ExecContext(context.Background(), s.db, database.TimeoutWrite,
		`INSERT INTO app_secrets (name, value, created_at, rotated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET value = excluded.value, rotated_at = excluded.rotated_at`,
		name, base64.StdEncoding.EncodeToString(value), s.timestamp(), s.timestamp())
	return err
}

// timestamp returns the current UTC time in the stored text layout.
func (s *Store) timestamp() string {
	return s.now().UTC().Format(timestampLayout)
}

// writeText stores a plain text row such as an expiry timestamp.
func (s *Store) writeText(name, value string) error {
	_, err := database.ExecContext(context.Background(), s.db, database.TimeoutWrite,
		`INSERT INTO app_secrets (name, value, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET value = excluded.value`,
		name, value, s.timestamp())
	return err
}

// remove deletes one row.
func (s *Store) remove(name string) error {
	_, err := database.ExecContext(context.Background(), s.db, database.TimeoutWrite,
		`DELETE FROM app_secrets WHERE name = ?`, name)
	return err
}

// EnsureAll generates every missing secret on first start, before any user-visible
// operation. INSERT OR IGNORE keeps the first writer's value when two nodes race.
func (s *Store) EnsureAll() error {
	for _, name := range []string{InstallationSecret, CookieSigningKey, CSRFTokenSecret} {
		if _, err := s.Current(name); err == nil {
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		value, err := generate()
		if err != nil {
			return err
		}
		if _, err := database.ExecContext(context.Background(), s.db, database.TimeoutWrite,
			`INSERT OR IGNORE INTO app_secrets (name, value, created_at) VALUES (?, ?, ?)`,
			name, base64.StdEncoding.EncodeToString(value), s.timestamp()); err != nil {
			return err
		}
	}
	return nil
}

// Current returns the current value of a secret.
func (s *Store) Current(name string) ([]byte, error) {
	if !isKnown(name) {
		return nil, ErrUnknownSecret
	}
	return s.read(name)
}

// Candidates returns the current value first, then the previous value while its grace
// period has not passed: a value that fails under the current key is tried with the
// previous key (PART 11 "Reads during grace").
func (s *Store) Candidates(name string) ([][]byte, error) {
	current, err := s.Current(name)
	if err != nil {
		return nil, err
	}
	candidates := [][]byte{current}

	var untilText string
	err = database.QueryRowContext(context.Background(), s.db, database.TimeoutSimpleSelect,
		`SELECT value FROM app_secrets WHERE name = ?`, name+untilSuffix).Scan(&untilText)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return candidates, nil
		}
		return nil, err
	}
	until, err := time.Parse(time.RFC3339, untilText)
	if err != nil || !s.now().Before(until) {
		return candidates, nil
	}
	previous, err := s.read(name + previousSuffix)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return candidates, nil
		}
		return nil, err
	}
	return append(candidates, previous), nil
}

// RotateOptions steer one rotation.
type RotateOptions struct {
	// Pending counts the rows still readable only with the previous value; a second
	// rotation is refused while it is above zero. Nil means no dependent rows exist.
	Pending func() (int, error)
	// Rebase re-encrypts or re-bases every dependent row from the old value to the new
	// one, eagerly. It must be safe to run again: a row already done is skipped.
	Rebase func(oldValue, newValue []byte) error
}

// Rotate replaces a secret following the PART 11 "Rotation contract": (1) write the
// previous-value rows, (2) generate and persist the new value, (3) rebase every dependent
// row eagerly. The previous value is stored before the new one is used, so a crash or a
// failed step loses nothing.
func (s *Store) Rotate(name string, options RotateOptions) error {
	if !isKnown(name) {
		return ErrUnknownSecret
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	oldValue, err := s.read(name)
	if err != nil {
		return err
	}

	if _, err := s.read(name + previousSuffix); err == nil && options.Pending != nil {
		pending, err := options.Pending()
		if err != nil {
			return err
		}
		if pending > 0 {
			return ErrRotationPending
		}
	}

	if grace := graceFor(name); grace > 0 {
		if err := s.write(name+previousSuffix, oldValue); err != nil {
			return err
		}
		if err := s.writeText(name+untilSuffix, s.now().UTC().Add(grace).Format(time.RFC3339)); err != nil {
			return err
		}
	}

	newValue, err := generate()
	if err != nil {
		return err
	}
	if err := s.write(name, newValue); err != nil {
		return err
	}

	if options.Rebase != nil {
		if err := options.Rebase(oldValue, newValue); err != nil {
			return fmt.Errorf("rebasing rows after rotating %s: %w", name, err)
		}
	}
	return nil
}

// RetireExpired deletes every previous value whose grace period has passed (PART 11
// "Retirement"). It returns the names that were retired.
func (s *Store) RetireExpired() ([]string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var retired []string
	for _, name := range []string{InstallationSecret, CookieSigningKey} {
		var untilText string
		err := database.QueryRowContext(context.Background(), s.db, database.TimeoutSimpleSelect,
			`SELECT value FROM app_secrets WHERE name = ?`, name+untilSuffix).Scan(&untilText)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return retired, err
		}
		if until, parseErr := time.Parse(time.RFC3339, untilText); parseErr == nil && s.now().Before(until) {
			continue
		}
		if err := s.remove(name + previousSuffix); err != nil {
			return retired, err
		}
		if err := s.remove(name + untilSuffix); err != nil {
			return retired, err
		}
		retired = append(retired, name)
	}
	return retired, nil
}

// parseTimestamp reads a stored time in either layout the SQLite driver returns: the
// CURRENT_TIMESTAMP text, or RFC 3339 for a DATETIME column.
func parseTimestamp(text string) (time.Time, error) {
	if parsed, err := time.Parse(timestampLayout, text); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, text)
}

// Age returns how long ago a secret was created or last rotated.
func (s *Store) Age(name string) (time.Duration, error) {
	if !isKnown(name) {
		return 0, ErrUnknownSecret
	}
	var created string
	var rotated sql.NullString
	err := database.QueryRowContext(context.Background(), s.db, database.TimeoutSimpleSelect,
		`SELECT created_at, rotated_at FROM app_secrets WHERE name = ?`, name).Scan(&created, &rotated)
	if err != nil {
		return 0, err
	}
	since, err := parseTimestamp(created)
	if err != nil {
		return 0, fmt.Errorf("reading created_at of %s: %w", name, err)
	}
	if rotated.Valid {
		if rotatedAt, parseErr := parseTimestamp(rotated.String); parseErr == nil && rotatedAt.After(since) {
			since = rotatedAt
		}
	}
	return s.now().Sub(since), nil
}

// Maintain is the periodic upkeep: it retires expired previous values and rotates the
// keys that reached their automatic interval (cookie_signing_key every 90 days,
// csrf_token_secret every 180 days). It returns the names it rotated.
func (s *Store) Maintain() (rotated []string, err error) {
	if _, err := s.RetireExpired(); err != nil {
		return nil, err
	}
	for name, maxAge := range map[string]time.Duration{CookieSigningKey: CookieMaxAge, CSRFTokenSecret: CSRFMaxAge} {
		age, ageErr := s.Age(name)
		if ageErr != nil {
			return rotated, ageErr
		}
		if age < maxAge {
			continue
		}
		if err := s.Rotate(name, RotateOptions{}); err != nil {
			return rotated, err
		}
		rotated = append(rotated, name)
	}
	return rotated, nil
}
