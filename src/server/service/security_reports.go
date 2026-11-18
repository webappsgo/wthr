package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/webappsgo/wthr/src/secret"
	"github.com/webappsgo/wthr/src/util"
)

// securityIDWindow is how long one {security_id} stays current (PART 11: 48 hours).
const securityIDWindow = 172800

// Report statuses in their only allowed order; wont_fix is reachable from any state.
var securityReportStatuses = []string{"received", "triaged", "confirmed", "patching", "disclosed", "wont_fix"}

// ErrReportNotFound is returned for an unknown tracking id or a wrong or expired token.
var ErrReportNotFound = errors.New("report not found")

// ErrBadStatusMove is returned when a status update would move a report backward.
var ErrBadStatusMove = errors.New("status cannot move backward")

// InstallationSecret returns the 32-byte root secret (PART 11). The secret store creates it on
// first start; this also creates it when called before startup installed the store.
func InstallationSecret(db *sql.DB) ([]byte, error) {
	store := secret.Default()
	if store == nil {
		store = secret.NewStore(db)
	}
	value, err := store.Current(secret.InstallationSecret)
	if errors.Is(err, sql.ErrNoRows) {
		if err := store.EnsureAll(); err != nil {
			return nil, err
		}
		return store.Current(secret.InstallationSecret)
	}
	return value, err
}

// securityIDFor computes the id for the 48 hour window that contains t.
func securityIDFor(secret []byte, t time.Time) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%d", t.Unix()/securityIDWindow)
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

// CurrentSecurityID returns the {security_id} for now.
func CurrentSecurityID(db *sql.DB) (string, error) {
	secret, err := InstallationSecret(db)
	if err != nil {
		return "", err
	}
	return securityIDFor(secret, time.Now()), nil
}

// ValidSecurityID reports whether id is the current or the previous window's id,
// compared in constant time.
func ValidSecurityID(db *sql.DB, id string) bool {
	if len(id) != 16 {
		return false
	}
	secret, err := InstallationSecret(db)
	if err != nil {
		return false
	}
	now := time.Now()
	current := securityIDFor(secret, now)
	previous := securityIDFor(secret, now.Add(-securityIDWindow*time.Second))
	ok := subtle.ConstantTimeCompare([]byte(id), []byte(current)) | subtle.ConstantTimeCompare([]byte(id), []byte(previous))
	return ok == 1
}

// SecurityIDFingerprint returns the first 8 hex characters of SHA-256(id), the only
// form of a supplied id that may be logged.
func SecurityIDFingerprint(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])[:8]
}

// SecurityReport is one decrypted disclosure submission body.
type SecurityReport struct {
	ResearcherName   string `json:"researcher_name"`
	Component        string `json:"component"`
	Endpoint         string `json:"endpoint"`
	Severity         string `json:"severity"`
	Summary          string `json:"summary"`
	Steps            string `json:"steps"`
	Impact           string `json:"impact"`
	SuggestedFix     string `json:"suggested_fix"`
	CVERequested     bool   `json:"cve_requested"`
	DisclosureDays   int    `json:"disclosure_days"`
	CreditPreference string `json:"credit_preference"`
	CreditHandle     string `json:"credit_handle"`
	AppVersion       string `json:"app_version"`
	CommitHash       string `json:"commit_hash"`
	RequestUserAgent string `json:"request_user_agent"`
	SubmittedAt      string `json:"timestamp"`
}

// ReportContact is the researcher contact stored encrypted next to a report.
type ReportContact struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	PGPKey string `json:"pgp_key,omitempty"`
}

// SecurityReportRow is the plaintext metadata of a stored report.
type SecurityReportRow struct {
	TrackingID        string
	Severity          string
	Component         string
	Status            string
	ResearcherComment string
	DisclosureDate    string
	CVERequested      bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ClosedAt          *time.Time
	BodyAlg           string
}

// SecurityReportStore persists encrypted reports (PART 11 "Security Report Storage").
type SecurityReportStore struct {
	DB *sql.DB
	// EncryptionKey is server.security.encryption_key
	EncryptionKey string
	PGP           *PGPKeyManager
}

// seal encrypts plaintext with the project PGP key when one exists, else AES-256-GCM.
func (s *SecurityReportStore) seal(plaintext []byte) (data []byte, alg string, err error) {
	if s.PGP != nil && s.PGP.Exists() {
		data, err = s.PGP.Encrypt(plaintext)
		return data, "pgp", err
	}
	sealed, err := util.EncryptAtRest(s.EncryptionKey, string(plaintext))
	return []byte(sealed), "aes-256-gcm", err
}

func (s *SecurityReportStore) open(data []byte, alg string) ([]byte, error) {
	if alg == "pgp" {
		return s.PGP.Decrypt(data)
	}
	plain, err := util.DecryptAtRest(s.EncryptionKey, string(data))
	return []byte(plain), err
}

// Create stores a new report and returns its tracking id and the one-shot status token.
func (s *SecurityReportStore) Create(securityID string, report SecurityReport, contact ReportContact) (trackingID, token string, err error) {
	rawBody, err := json.Marshal(report)
	if err != nil {
		return "", "", err
	}
	body, alg, err := s.seal(rawBody)
	if err != nil {
		return "", "", err
	}
	var contactEnc []byte
	if contact.Email != "" {
		rawContact, err := json.Marshal(contact)
		if err != nil {
			return "", "", err
		}
		if contactEnc, _, err = s.seal(rawContact); err != nil {
			return "", "", err
		}
	}
	idBytes, tokenBytes := make([]byte, 8), make([]byte, 32)
	if _, err = rand.Read(idBytes); err != nil {
		return "", "", err
	}
	if _, err = rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	trackingID = "sec_" + hex.EncodeToString(idBytes)
	token = hex.EncodeToString(tokenBytes)
	tokenHash := sha256.Sum256([]byte(token))
	now := time.Now().UTC()
	_, err = s.DB.Exec(`INSERT INTO security_reports (tracking_id, security_id, severity, component, status, body_enc, body_alg, contact_enc, cve_requested, token_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'received', ?, ?, ?, ?, ?, ?, ?)`,
		trackingID, SecurityIDFingerprint(securityID), report.Severity, report.Component, body, alg, contactEnc, report.CVERequested,
		hex.EncodeToString(tokenHash[:]), now, now)
	return trackingID, token, err
}

const reportColumns = `tracking_id, severity, component, status, researcher_comment, COALESCE(disclosure_date, ''), cve_requested, created_at, updated_at, closed_at, body_alg`

func scanReport(scan func(...any) error) (*SecurityReportRow, error) {
	var r SecurityReportRow
	var closed sql.NullTime
	if err := scan(&r.TrackingID, &r.Severity, &r.Component, &r.Status, &r.ResearcherComment, &r.DisclosureDate, &r.CVERequested, &r.CreatedAt, &r.UpdatedAt, &closed, &r.BodyAlg); err != nil {
		return nil, err
	}
	if closed.Valid {
		t := closed.Time
		r.ClosedAt = &t
	}
	r.DisclosureDate = strings.TrimSuffix(strings.SplitN(r.DisclosureDate, "T", 2)[0], " 00:00:00")
	return &r, nil
}

// List returns reports newest first, optionally filtered by status.
func (s *SecurityReportStore) List(status string) ([]*SecurityReportRow, error) {
	query := `SELECT ` + reportColumns + ` FROM security_reports`
	var args []any
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	rows, err := s.DB.Query(query+` ORDER BY created_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SecurityReportRow
	for rows.Next() {
		r, err := scanReport(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get returns one report's metadata.
func (s *SecurityReportStore) Get(trackingID string) (*SecurityReportRow, error) {
	r, err := scanReport(s.DB.QueryRow(`SELECT `+reportColumns+` FROM security_reports WHERE tracking_id = ?`, trackingID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReportNotFound
	}
	return r, err
}

// Body decrypts a report body and the researcher contact for an authenticated admin view.
func (s *SecurityReportStore) Body(trackingID string) (*SecurityReport, *ReportContact, error) {
	var body, contact []byte
	var alg string
	err := s.DB.QueryRow(`SELECT body_enc, body_alg, contact_enc FROM security_reports WHERE tracking_id = ?`, trackingID).Scan(&body, &alg, &contact)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrReportNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	plain, err := s.open(body, alg)
	if err != nil {
		return nil, nil, err
	}
	var report SecurityReport
	if err := json.Unmarshal(plain, &report); err != nil {
		return nil, nil, err
	}
	var rc *ReportContact
	if len(contact) > 0 {
		calg := "aes-256-gcm"
		if strings.HasPrefix(string(contact), "-----BEGIN PGP") {
			calg = "pgp"
		}
		if plainContact, err := s.open(contact, calg); err == nil {
			rc = &ReportContact{}
			if json.Unmarshal(plainContact, rc) != nil {
				rc = nil
			}
		}
	}
	return &report, rc, nil
}

func statusIndex(status string) int {
	for i, s := range securityReportStatuses {
		if s == status {
			return i
		}
	}
	return -1
}

// ValidStatus reports whether status is one of the six report states.
func ValidStatus(status string) bool { return statusIndex(status) >= 0 }

// Update changes status, researcher comment and disclosure date. Status only moves
// forward, or to wont_fix; disclosed and wont_fix close the report.
func (s *SecurityReportStore) Update(trackingID, status, comment, disclosureDate string) (oldStatus string, err error) {
	cur, err := s.Get(trackingID)
	if err != nil {
		return "", err
	}
	if !ValidStatus(status) {
		return cur.Status, errors.New("invalid status")
	}
	if status != cur.Status && status != "wont_fix" && statusIndex(status) < statusIndex(cur.Status) {
		return cur.Status, ErrBadStatusMove
	}
	if cur.Status == "wont_fix" && status != "wont_fix" {
		return cur.Status, ErrBadStatusMove
	}
	if len([]rune(comment)) > 2000 {
		return cur.Status, errors.New("comment too long")
	}
	var date any
	if disclosureDate != "" {
		if _, perr := time.Parse("2006-01-02", disclosureDate); perr != nil {
			return cur.Status, errors.New("invalid date")
		}
		date = disclosureDate
	}
	now := time.Now().UTC()
	var closed any
	if cur.ClosedAt != nil {
		closed = *cur.ClosedAt
	} else if status == "disclosed" || status == "wont_fix" {
		closed = now
	}
	_, err = s.DB.Exec(`UPDATE security_reports SET status = ?, researcher_comment = ?, disclosure_date = ?, updated_at = ?, closed_at = ? WHERE tracking_id = ?`,
		status, comment, date, now, closed, trackingID)
	return cur.Status, err
}

// ResearcherView validates the one-shot token for the public status page. The token is
// compared in constant time, works once per calendar day, and stops working 30 days
// after the report was closed.
func (s *SecurityReportStore) ResearcherView(trackingID, token string) (*SecurityReportRow, error) {
	var hash string
	var lastUsed sql.NullString
	err := s.DB.QueryRow(`SELECT token_hash, token_last_used FROM security_reports WHERE tracking_id = ?`, trackingID).Scan(&hash, &lastUsed)
	if err != nil {
		return nil, ErrReportNotFound
	}
	sum := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare([]byte(hash), []byte(hex.EncodeToString(sum[:]))) != 1 {
		return nil, ErrReportNotFound
	}
	row, err := s.Get(trackingID)
	if err != nil {
		return nil, err
	}
	if row.ClosedAt != nil && time.Since(*row.ClosedAt) > 30*24*time.Hour {
		return nil, ErrReportNotFound
	}
	today := time.Now().UTC().Format("2006-01-02")
	if lastUsed.Valid && strings.HasPrefix(lastUsed.String, today) {
		return nil, ErrReportNotFound
	}
	if _, err := s.DB.Exec(`UPDATE security_reports SET token_last_used = ? WHERE tracking_id = ?`, today, trackingID); err != nil {
		return nil, err
	}
	return row, nil
}

// Purge deletes closed reports older than retentionDays after closure; open reports are never purged.
func (s *SecurityReportStore) Purge(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 365
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	res, err := s.DB.Exec(`DELETE FROM security_reports WHERE closed_at IS NOT NULL AND closed_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ThanksEntry is one public hall-of-fame line.
type ThanksEntry struct {
	ID       int64  `json:"-"`
	Credit   string `json:"credit"`
	Date     string `json:"date"`
	Severity string `json:"severity"`
	Visible  bool   `json:"-"`
}

// AddThanks records the credit for a disclosed report according to the researcher's
// preference: real name, handle, anonymous numbered, or none.
func (s *SecurityReportStore) AddThanks(trackingID string) error {
	row, err := s.Get(trackingID)
	if err != nil {
		return err
	}
	report, _, err := s.Body(trackingID)
	if err != nil {
		return err
	}
	var credit string
	switch report.CreditPreference {
	case "real_name":
		credit = report.ResearcherName
	case "handle":
		credit = report.CreditHandle
	case "anonymous":
		var n int
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM security_thanks WHERE credit LIKE 'Anonymous Researcher #%'`).Scan(&n)
		credit = fmt.Sprintf("Anonymous Researcher #%d", n+1)
	default:
		return nil
	}
	if strings.TrimSpace(credit) == "" {
		return nil
	}
	_, err = s.DB.Exec(`INSERT OR IGNORE INTO security_thanks (tracking_id, credit, severity, year) VALUES (?, ?, ?, ?)`,
		trackingID, credit, row.Severity, time.Now().UTC().Year())
	return err
}

// Thanks lists the entries; onlyVisible hides the ones an admin switched off.
func (s *SecurityReportStore) Thanks(onlyVisible bool) ([]ThanksEntry, error) {
	query := `SELECT id, credit, year, severity, visible FROM security_thanks`
	if onlyVisible {
		query += ` WHERE visible = 1`
	}
	rows, err := s.DB.Query(query + ` ORDER BY sort_order, year DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ThanksEntry
	for rows.Next() {
		var e ThanksEntry
		var year, visible int
		if err := rows.Scan(&e.ID, &e.Credit, &year, &e.Severity, &visible); err != nil {
			return nil, err
		}
		e.Date, e.Visible = fmt.Sprintf("%d", year), visible != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpdateThanks edits one hall-of-fame entry.
func (s *SecurityReportStore) UpdateThanks(id int64, credit string, visible bool, sortOrder int) error {
	if strings.TrimSpace(credit) == "" || len([]rune(credit)) > 100 {
		return errors.New("invalid credit")
	}
	_, err := s.DB.Exec(`UPDATE security_thanks SET credit = ?, visible = ?, sort_order = ? WHERE id = ?`, credit, visible, sortOrder, id)
	return err
}
