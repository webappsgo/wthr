package secret

import (
	"bytes"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/webappsgo/wthr/src/database"
	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE app_secrets (
		name TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		rotated_at DATETIME
	)`); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	store := NewStore(database.NewStore(db))
	store.now = func() time.Time { return clock }
	return store, &clock
}

func TestEnsureAllGeneratesEverySecretOnce(t *testing.T) {
	store, _ := newTestStore(t)
	if err := store.EnsureAll(); err != nil {
		t.Fatal(err)
	}

	first := map[string][]byte{}
	for _, name := range []string{InstallationSecret, CookieSigningKey, CSRFTokenSecret} {
		value, err := store.Current(name)
		if err != nil || len(value) != 32 {
			t.Fatalf("%s: %v, %d bytes", name, err, len(value))
		}
		first[name] = value
	}
	if bytes.Equal(first[InstallationSecret], first[CookieSigningKey]) {
		t.Error("each secret must be independent")
	}

	if err := store.EnsureAll(); err != nil {
		t.Fatal(err)
	}
	for name, value := range first {
		again, _ := store.Current(name)
		if !bytes.Equal(value, again) {
			t.Errorf("%s changed on the second start", name)
		}
	}
}

func TestCurrentRejectsUnknownNames(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Current("something_else"); !errors.Is(err, ErrUnknownSecret) {
		t.Errorf("err = %v", err)
	}
	if err := store.Rotate("something_else", RotateOptions{}); !errors.Is(err, ErrUnknownSecret) {
		t.Errorf("err = %v", err)
	}
}

func TestRotateKeepsThePreviousValueDuringGrace(t *testing.T) {
	store, clock := newTestStore(t)
	if err := store.EnsureAll(); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Current(CookieSigningKey)

	var rebasedFrom, rebasedTo []byte
	err := store.Rotate(CookieSigningKey, RotateOptions{Rebase: func(oldValue, newValue []byte) error {
		rebasedFrom, rebasedTo = oldValue, newValue
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := store.Current(CookieSigningKey)
	if bytes.Equal(before, after) {
		t.Fatal("rotation must change the value")
	}
	if !bytes.Equal(rebasedFrom, before) || !bytes.Equal(rebasedTo, after) {
		t.Error("the rebase callback must receive the old and the new value")
	}

	candidates, err := store.Candidates(CookieSigningKey)
	if err != nil || len(candidates) != 2 || !bytes.Equal(candidates[0], after) || !bytes.Equal(candidates[1], before) {
		t.Fatalf("during grace the previous value must follow the current one, got %d, %v", len(candidates), err)
	}

	*clock = clock.Add(7*24*time.Hour + time.Minute)
	candidates, _ = store.Candidates(CookieSigningKey)
	if len(candidates) != 1 {
		t.Errorf("after grace only the current value is valid, got %d", len(candidates))
	}
}

func TestCSRFSecretRotationKeepsNoPreviousValue(t *testing.T) {
	store, _ := newTestStore(t)
	_ = store.EnsureAll()
	if err := store.Rotate(CSRFTokenSecret, RotateOptions{}); err != nil {
		t.Fatal(err)
	}
	candidates, _ := store.Candidates(CSRFTokenSecret)
	if len(candidates) != 1 {
		t.Errorf("csrf_token_secret has no grace period, got %d candidates", len(candidates))
	}
}

func TestRotateRefusesWhileRowsStillNeedThePreviousValue(t *testing.T) {
	store, _ := newTestStore(t)
	_ = store.EnsureAll()
	if err := store.Rotate(InstallationSecret, RotateOptions{}); err != nil {
		t.Fatal(err)
	}

	err := store.Rotate(InstallationSecret, RotateOptions{Pending: func() (int, error) { return 3, nil }})
	if !errors.Is(err, ErrRotationPending) {
		t.Errorf("err = %v, want ErrRotationPending", err)
	}
	if err := store.Rotate(InstallationSecret, RotateOptions{Pending: func() (int, error) { return 0, nil }}); err != nil {
		t.Errorf("a rotation with nothing pending must succeed: %v", err)
	}
}

func TestRotateReportsAFailedRebaseButKeepsThePreviousValue(t *testing.T) {
	store, _ := newTestStore(t)
	_ = store.EnsureAll()
	before, _ := store.Current(InstallationSecret)

	err := store.Rotate(InstallationSecret, RotateOptions{Rebase: func(_, _ []byte) error { return errors.New("disk full") }})
	if err == nil {
		t.Fatal("a failed rebase must be reported")
	}
	candidates, _ := store.Candidates(InstallationSecret)
	if len(candidates) != 2 || !bytes.Equal(candidates[1], before) {
		t.Error("the previous value must survive a failed rebase so nothing is lost")
	}
}

func TestRetireExpiredDeletesOnlyExpiredPreviousValues(t *testing.T) {
	store, clock := newTestStore(t)
	_ = store.EnsureAll()
	_ = store.Rotate(InstallationSecret, RotateOptions{})

	retired, err := store.RetireExpired()
	if err != nil || len(retired) != 0 {
		t.Fatalf("a value inside its grace period must stay, got %v, %v", retired, err)
	}

	*clock = clock.Add(8 * 24 * time.Hour)
	retired, err = store.RetireExpired()
	if err != nil || len(retired) != 1 || retired[0] != InstallationSecret {
		t.Fatalf("an expired previous value must be retired, got %v, %v", retired, err)
	}
	candidates, _ := store.Candidates(InstallationSecret)
	if len(candidates) != 1 {
		t.Errorf("only the current value remains, got %d", len(candidates))
	}
}

func TestMaintainRotatesByAge(t *testing.T) {
	store, clock := newTestStore(t)
	_ = store.EnsureAll()

	rotated, err := store.Maintain()
	if err != nil || len(rotated) != 0 {
		t.Fatalf("nothing is due on day zero, got %v, %v", rotated, err)
	}

	*clock = clock.Add(91 * 24 * time.Hour)
	rotated, err = store.Maintain()
	if err != nil || len(rotated) != 1 || rotated[0] != CookieSigningKey {
		t.Fatalf("cookie_signing_key is due after 90 days, got %v, %v", rotated, err)
	}

	*clock = clock.Add(100 * 24 * time.Hour)
	rotated, err = store.Maintain()
	if err != nil || len(rotated) != 2 {
		t.Fatalf("both keys are due after 181 days in total, got %v, %v", rotated, err)
	}
}

func TestDefaultStore(t *testing.T) {
	SetDefault(nil)
	if Default() != nil {
		t.Error("no store is installed yet")
	}
	store, _ := newTestStore(t)
	SetDefault(store)
	t.Cleanup(func() { SetDefault(nil) })
	if Default() != store {
		t.Error("the installed store must be returned")
	}
}
