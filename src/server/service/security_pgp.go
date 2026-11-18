package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"

	"github.com/webappsgo/wthr/src/secret"
	"github.com/webappsgo/wthr/src/util"
)

// pgpKeyLifetime is the validity of a generated project keypair (PART 11: 2 years).
const pgpKeyLifetime = 2 * 365 * 24 * time.Hour

// researcherKeyMaxBytes caps a researcher public key, pasted or fetched.
const researcherKeyMaxBytes = 16384

// ErrNoKeypair is returned when an operation needs the project keypair and none exists.
var ErrNoKeypair = errors.New("no project keypair")

// PGPKeyInfo is the keypair metadata shown in the admin panel and security.txt.
type PGPKeyInfo struct {
	Fingerprint         string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	LastRotatedAt       *time.Time
	KeyserversPublished map[string]time.Time
	Revoked             bool
}

// PGPKeyManager owns the single project-level PGP keypair (PART 11 "GPG Keypair
// Management"). The public key lives in {config_dir}/security/pgp.pub.asc and the
// private key, encrypted under a key derived from installation_secret, in
// pgp.priv.asc.enc. Only metadata is stored in the database.
type PGPKeyManager struct {
	db  *sql.DB
	dir string
}

// NewPGPKeyManager creates a manager that keeps its files in dir.
func NewPGPKeyManager(db *sql.DB, dir string) *PGPKeyManager {
	return &PGPKeyManager{db: db, dir: dir}
}

func (m *PGPKeyManager) pubPath() string  { return filepath.Join(m.dir, "pgp.pub.asc") }
func (m *PGPKeyManager) privPath() string { return filepath.Join(m.dir, "pgp.priv.asc.enc") }

// privateKeyKeyFor derives the base64 AES key that protects the private key at rest from
// one installation_secret value.
func privateKeyKeyFor(installationSecret []byte) string {
	sum := sha256.Sum256(append([]byte("wthr-pgp-private-key\x00"), installationSecret...))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// privateKeyKey derives the key from the current installation_secret.
func (m *PGPKeyManager) privateKeyKey() (string, error) {
	installationSecret, err := InstallationSecret(m.db)
	if err != nil {
		return "", err
	}
	return privateKeyKeyFor(installationSecret), nil
}

// decryptUnder opens sealed data with the key derived from one installation_secret value.
func decryptUnder(installationSecret []byte, sealed string) (string, error) {
	return util.DecryptAtRest(privateKeyKeyFor(installationSecret), sealed)
}

// installationSecretCandidates returns the current installation_secret first, then the
// previous one while its 7 day grace period lasts (PART 11 "Reads during grace").
func (m *PGPKeyManager) installationSecretCandidates() ([][]byte, error) {
	store := secret.Default()
	if store == nil {
		current, err := InstallationSecret(m.db)
		if err != nil {
			return nil, err
		}
		return [][]byte{current}, nil
	}
	return store.Candidates(secret.InstallationSecret)
}

// Rebase re-encrypts the private key file from the old installation_secret to the new one.
// It is safe to run again: a file that already opens under the new secret is left alone. The
// file is replaced atomically through a temporary file, so a crash never leaves a half
// written key.
func (m *PGPKeyManager) Rebase(oldSecret, newSecret []byte) error {
	data, err := os.ReadFile(m.privPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := util.DecryptAtRest(privateKeyKeyFor(newSecret), string(data)); err == nil {
		return nil
	}
	armored, err := util.DecryptAtRest(privateKeyKeyFor(oldSecret), string(data))
	if err != nil {
		return fmt.Errorf("private key opens under neither installation secret: %w", err)
	}
	sealed, err := util.EncryptAtRest(privateKeyKeyFor(newSecret), armored)
	if err != nil {
		return err
	}
	temp := m.privPath() + ".tmp"
	if err := os.WriteFile(temp, []byte(sealed), 0o600); err != nil {
		return err
	}
	return os.Rename(temp, m.privPath())
}

// pendingRebase reports 1 when the private key file opens only with the previous installation
// secret, so a second rotation is refused until it has been re-encrypted.
func (m *PGPKeyManager) pendingRebase() (int, error) {
	data, err := os.ReadFile(m.privPath())
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	current, err := InstallationSecret(m.db)
	if err != nil {
		return 0, err
	}
	if _, err := util.DecryptAtRest(privateKeyKeyFor(current), string(data)); err != nil {
		return 1, nil
	}
	return 0, nil
}

// Exists reports whether a usable (not revoked) keypair is present.
func (m *PGPKeyManager) Exists() bool {
	info, err := m.Info()
	if err != nil || info.Revoked {
		return false
	}
	_, statErr := os.Stat(m.pubPath())
	return statErr == nil
}

// Info returns the stored keypair metadata.
func (m *PGPKeyManager) Info() (*PGPKeyInfo, error) {
	var (
		info             PGPKeyInfo
		created, expires sql.NullTime
		rotated          sql.NullTime
		published        string
		revoked          int
	)
	err := m.db.QueryRow(`SELECT fingerprint, created_at, expires_at, last_rotated_at, keyservers_published, revoked FROM security_keypair WHERE id = 1`).
		Scan(&info.Fingerprint, &created, &expires, &rotated, &published, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoKeypair
	}
	if err != nil {
		return nil, err
	}
	info.CreatedAt, info.ExpiresAt, info.Revoked = created.Time, expires.Time, revoked != 0
	if rotated.Valid {
		t := rotated.Time
		info.LastRotatedAt = &t
	}
	info.KeyserversPublished = map[string]time.Time{}
	_ = json.Unmarshal([]byte(published), &info.KeyserversPublished)
	return &info, nil
}

// Generate creates a new Ed25519 + Curve25519 keypair, replacing any existing one.
func (m *PGPKeyManager) Generate(appName, contactEmail string, rotate bool) (*PGPKeyInfo, error) {
	cfg := &packet.Config{Algorithm: packet.PubKeyAlgoEdDSA, KeyLifetimeSecs: uint32(pgpKeyLifetime.Seconds())}
	entity, err := openpgp.NewEntity(appName+" Security", "", contactEmail, cfg)
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}
	if err := m.save(entity); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	fingerprint := strings.ToUpper(hex.EncodeToString(entity.PrimaryKey.Fingerprint))
	var rotated any
	if rotate {
		rotated = now
	}
	_, err = m.db.Exec(`INSERT INTO security_keypair (id, fingerprint, created_at, expires_at, last_rotated_at, keyservers_published, revoked)
		VALUES (1, ?, ?, ?, ?, '{}', 0)
		ON CONFLICT(id) DO UPDATE SET fingerprint = excluded.fingerprint, created_at = excluded.created_at,
		expires_at = excluded.expires_at, last_rotated_at = excluded.last_rotated_at, keyservers_published = '{}', revoked = 0`,
		fingerprint, now, now.Add(pgpKeyLifetime), rotated)
	if err != nil {
		return nil, err
	}
	return m.Info()
}

// save writes the armored public key and the encrypted armored private key.
func (m *PGPKeyManager) save(entity *openpgp.Entity) error {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return err
	}
	var pub, priv bytes.Buffer
	pubW, err := armor.Encode(&pub, openpgp.PublicKeyType, nil)
	if err != nil {
		return err
	}
	if err := entity.Serialize(pubW); err != nil {
		return err
	}
	if err := pubW.Close(); err != nil {
		return err
	}
	privW, err := armor.Encode(&priv, openpgp.PrivateKeyType, nil)
	if err != nil {
		return err
	}
	if err := entity.SerializePrivate(privW, nil); err != nil {
		return err
	}
	if err := privW.Close(); err != nil {
		return err
	}
	key, err := m.privateKeyKey()
	if err != nil {
		return err
	}
	sealed, err := util.EncryptAtRest(key, priv.String())
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.pubPath(), pub.Bytes(), 0o644); err != nil {
		return err
	}
	return os.WriteFile(m.privPath(), []byte(sealed), 0o600)
}

// PublicKey returns the ASCII-armored public key.
func (m *PGPKeyManager) PublicKey() (string, error) {
	data, err := os.ReadFile(m.pubPath())
	if err != nil {
		return "", ErrNoKeypair
	}
	return string(data), nil
}

// PrivateKey returns the decrypted ASCII-armored private key (sensitive operation). A file that
// fails under the current installation_secret is tried with the previous one during the grace
// period, and a success re-encrypts it under the current one.
func (m *PGPKeyManager) PrivateKey() (string, error) {
	data, err := os.ReadFile(m.privPath())
	if err != nil {
		return "", ErrNoKeypair
	}
	candidates, err := m.installationSecretCandidates()
	if err != nil {
		return "", err
	}
	var lastErr error
	for index, candidate := range candidates {
		armored, decryptErr := util.DecryptAtRest(privateKeyKeyFor(candidate), string(data))
		if decryptErr != nil {
			lastErr = decryptErr
			continue
		}
		if index > 0 {
			_ = m.Rebase(candidate, candidates[0])
		}
		return armored, nil
	}
	return "", lastErr
}

func (m *PGPKeyManager) entity() (*openpgp.Entity, error) {
	armored, err := m.PrivateKey()
	if err != nil {
		return nil, err
	}
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(armored))
	if err != nil || len(ring) == 0 {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	return ring[0], nil
}

// ImportPrivate installs an existing armored private key. A mismatch with the expected
// identity is reported through the returned warning, never an error.
func (m *PGPKeyManager) ImportPrivate(armored, expectedIdentity string) (warning string, err error) {
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(armored))
	if err != nil || len(ring) == 0 || ring[0].PrivateKey == nil {
		return "", errors.New("not a private key")
	}
	entity := ring[0]
	matched := false
	for name := range entity.Identities {
		if strings.Contains(name, expectedIdentity) {
			matched = true
		}
	}
	if !matched {
		warning = "identity_mismatch"
	}
	if err := m.save(entity); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	var expires any
	if life, ok := entityExpiry(entity); ok {
		expires = life
	}
	_, err = m.db.Exec(`INSERT INTO security_keypair (id, fingerprint, created_at, expires_at, keyservers_published, revoked)
		VALUES (1, ?, ?, ?, '{}', 0)
		ON CONFLICT(id) DO UPDATE SET fingerprint = excluded.fingerprint, created_at = excluded.created_at,
		expires_at = excluded.expires_at, keyservers_published = '{}', revoked = 0`,
		strings.ToUpper(hex.EncodeToString(entity.PrimaryKey.Fingerprint)), now, expires)
	return warning, err
}

func entityExpiry(e *openpgp.Entity) (time.Time, bool) {
	for _, id := range e.Identities {
		if id.SelfSignature != nil && id.SelfSignature.KeyLifetimeSecs != nil {
			return e.PrimaryKey.CreationTime.Add(time.Duration(*id.SelfSignature.KeyLifetimeSecs) * time.Second), true
		}
	}
	return time.Time{}, false
}

// Delete removes both key files and marks the keypair revoked; the fingerprint stays for history.
func (m *PGPKeyManager) Delete() error {
	_ = os.Remove(m.pubPath())
	_ = os.Remove(m.privPath())
	_, err := m.db.Exec(`UPDATE security_keypair SET revoked = 1 WHERE id = 1`)
	return err
}

// Encrypt encrypts plaintext to the project key and returns an ASCII-armored message.
func (m *PGPKeyManager) Encrypt(plaintext []byte) ([]byte, error) {
	armoredPub, err := m.PublicKey()
	if err != nil {
		return nil, err
	}
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(armoredPub))
	if err != nil {
		return nil, err
	}
	return pgpEncrypt(ring, plaintext)
}

// Decrypt decrypts an ASCII-armored message addressed to the project key.
func (m *PGPKeyManager) Decrypt(message []byte) ([]byte, error) {
	entity, err := m.entity()
	if err != nil {
		return nil, err
	}
	block, err := armor.Decode(bytes.NewReader(message))
	if err != nil {
		return nil, err
	}
	md, err := openpgp.ReadMessage(block.Body, openpgp.EntityList{entity}, nil, nil)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(md.UnverifiedBody, 1<<20))
}

func pgpEncrypt(recipients openpgp.EntityList, plaintext []byte) ([]byte, error) {
	var out bytes.Buffer
	armorW, err := armor.Encode(&out, "PGP MESSAGE", nil)
	if err != nil {
		return nil, err
	}
	w, err := openpgp.Encrypt(armorW, recipients, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	if err := armorW.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// EncryptToArmoredKey encrypts plaintext to an armored public key (a researcher or admin key).
func EncryptToArmoredKey(armoredPub string, plaintext []byte) ([]byte, error) {
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(armoredPub))
	if err != nil || len(ring) == 0 {
		return nil, errors.New("invalid public key")
	}
	return pgpEncrypt(ring, plaintext)
}

// ValidatePublicKey checks that armored holds an encryption-capable public key and
// returns it re-armored (private material, if any was pasted by mistake, is dropped).
func ValidatePublicKey(armored string) (string, error) {
	if len(armored) > researcherKeyMaxBytes {
		return "", errors.New("key too large")
	}
	ring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(armored))
	if err != nil || len(ring) == 0 {
		return "", errors.New("invalid public key")
	}
	if _, ok := ring[0].EncryptionKey(time.Now()); !ok {
		return "", errors.New("key cannot encrypt")
	}
	var out bytes.Buffer
	w, err := armor.Encode(&out, openpgp.PublicKeyType, nil)
	if err != nil {
		return "", err
	}
	if err := ring[0].Serialize(w); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// FetchResearcherKey resolves a researcher key given either a pasted armored block or an
// https URL, and returns the validated armored public key. URL fetches refuse private,
// loopback and link-local addresses, do not follow redirects and are size and time capped.
func FetchResearcherKey(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}
	if !strings.HasPrefix(input, "https://") {
		return ValidatePublicKey(input)
	}
	if len(input) > 2048 {
		return "", errors.New("url too long")
	}
	u, err := url.Parse(input)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errors.New("invalid url")
	}
	body, err := safeHTTPSGet(ctx, u.String(), researcherKeyMaxBytes)
	if err != nil {
		return "", err
	}
	return ValidatePublicKey(string(body))
}

// blockedAddr reports whether ip must never be contacted on behalf of a visitor.
func blockedAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified()
}

// safeHTTPSGet performs a bounded GET whose connections are checked after DNS resolution.
func safeHTTPSGet(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error) {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || blockedAddr(ip) {
				return errors.New("address not allowed")
			}
			return nil
		},
	}
	client := &http.Client{
		Timeout:       10 * time.Second,
		Transport:     &http.Transport{DialContext: dialer.DialContext, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
}

// PublishToKeyservers POSTs the public key to every keyserver and records which accepted it.
func (m *PGPKeyManager) PublishToKeyservers(ctx context.Context, keyservers []string) (map[string]error, error) {
	armoredPub, err := m.PublicKey()
	if err != nil {
		return nil, err
	}
	info, err := m.Info()
	if err != nil {
		return nil, err
	}
	results := map[string]error{}
	for _, ks := range keyservers {
		ks = strings.TrimRight(strings.TrimSpace(ks), "/")
		if ks == "" {
			continue
		}
		results[ks] = publishKey(ctx, ks, armoredPub)
		if results[ks] == nil {
			info.KeyserversPublished[ks] = time.Now().UTC()
		}
	}
	raw, _ := json.Marshal(info.KeyserversPublished)
	_, err = m.db.Exec(`UPDATE security_keypair SET keyservers_published = ? WHERE id = 1`, string(raw))
	if err == nil {
		_ = os.WriteFile(filepath.Join(m.dir, "keyservers.state"), raw, 0o600)
	}
	return results, err
}

func publishKey(ctx context.Context, keyserver, armoredPub string) error {
	u, err := url.Parse(keyserver)
	if err != nil || u.Scheme != "https" {
		return errors.New("keyserver must be https")
	}
	var req *http.Request
	if strings.Contains(u.Host, "keys.openpgp.org") {
		body, _ := json.Marshal(map[string]string{"keytext": armoredPub})
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, keyserver+"/vks/v1/upload", bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	} else {
		form := url.Values{"keytext": {armoredPub}}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, keyserver+"/pks/add", strings.NewReader(form.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
