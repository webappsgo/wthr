package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/reqctx"
)

type ipBlockRecord struct {
	ID           int64   `json:"id"`
	IP           string  `json:"ip,omitempty"`
	CIDR         string  `json:"cidr,omitempty"`
	Type         string  `json:"type"`
	Reason       string  `json:"reason"`
	BlockedAt    string  `json:"blocked_at"`
	ExpiresAt    *string `json:"expires_at,omitempty"`
	OffenseCount int     `json:"offense_count"`
	AutoBlocked  bool    `json:"auto_blocked"`
	BlockedBy    string  `json:"blocked_by,omitempty"`
}

type ipBlockInput struct {
	IP        string `json:"ip"`
	CIDR      string `json:"cidr"`
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	ExpiresAt string `json:"expires_at"`
}

// ListIPBlocks returns active and expired IP/CIDR block records for admins.
func ListIPBlocks(w http.ResponseWriter, r *http.Request) {
	records, err := readIPBlocks(r.Context(), database.GetServerStore(), false)
	if err != nil {
		InternalError(w, r, err.Error())
		return
	}
	RespondData(w, r, records)
}

// GetIPBlock returns one block record matching an IP address or CIDR.
func GetIPBlock(w http.ResponseWriter, r *http.Request) {
	value := chi.URLParam(r, "ip")
	records, err := readIPBlocks(r.Context(), database.GetServerStore(), true)
	if err != nil {
		InternalError(w, r, err.Error())
		return
	}
	for _, record := range records {
		if record.IP == value || record.CIDR == value {
			RespondData(w, r, record)
			return
		}
	}
	RespondError(w, r, http.StatusNotFound, "IP_BLOCK_NOT_FOUND", "Blocked IP address was not found")
}

// CreateIPBlock adds a manual IP or CIDR block from a browser form or JSON API.
func CreateIPBlock(w http.ResponseWriter, r *http.Request) {
	var input ipBlockInput
	if wantsFormSubmission(r) {
		if err := r.ParseForm(); err != nil {
			redirectAdminForm(w, r, adminPagePath(r, "/security/firewall"), "flash.settings_saved", "flash.settings_save_failed", err)
			return
		}
		input = ipBlockInput{IP: r.PostForm.Get("ip"), CIDR: r.PostForm.Get("cidr"), Type: r.PostForm.Get("block_type"), Reason: r.PostForm.Get("reason"), ExpiresAt: r.PostForm.Get("expires_at")}
	} else if !DecodeAndValidate(w, r, &input) {
		return
	}
	value := strings.TrimSpace(input.CIDR)
	if value == "" {
		value = strings.TrimSpace(input.IP)
	}
	kind, err := classifyIPBlockValue(value)
	blockType := strings.TrimSpace(input.Type)
	if err == nil && blockType == "" {
		blockType = "permanent"
	}
	if err == nil && blockType != "temporary" && blockType != "extended" && blockType != "permanent" {
		err = fmt.Errorf("block type must be temporary, extended, or permanent")
	}
	reason := strings.TrimSpace(input.Reason)
	if err == nil && (reason == "" || len(reason) > 500) {
		err = fmt.Errorf("reason is required and must be 500 characters or fewer")
	}
	var expiresAt interface{}
	if err == nil {
		expiresAt, err = parseBlockExpiry(input.ExpiresAt, blockType)
	}
	db := database.GetServerStore()
	if err == nil && db == nil {
		err = fmt.Errorf("server database is not available")
	}
	if err == nil {
		adminName := reqctxString(r, "admin_username")
		_, err = database.ExecContext(r.Context(), db, database.TimeoutWrite,
			`INSERT INTO server_blocklists (type, value, reason, source, added_by, expires_at, block_type, offense_count, auto_blocked, blocked_by)
			 VALUES (?, ?, ?, 'manual', ?, ?, ?, 0, 0, ?)`,
			kind, value, reason, adminName, expiresAt, blockType, adminName)
		if err == nil {
			logSecurityAudit(r, "security.ip_block_added", value, "")
		}
	}
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/security/firewall"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "IP_BLOCK_INVALID", err.Error())
		return
	}
	RespondCreated(w, r, "IP block added", value)
}

// DeleteIPBlock removes a block by its IP/CIDR value or numeric database ID.
func DeleteIPBlock(w http.ResponseWriter, r *http.Request) {
	db := database.GetServerStore()
	value := chi.URLParam(r, "ip")
	var result sql.Result
	var err error
	if db == nil {
		err = fmt.Errorf("server database is not available")
	} else if id, parseErr := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); parseErr == nil {
		result, err = database.ExecContext(r.Context(), db, database.TimeoutWrite, `DELETE FROM server_blocklists WHERE id = ?`, id)
		value = strconv.FormatInt(id, 10)
	} else {
		result, err = database.ExecContext(r.Context(), db, database.TimeoutWrite, `DELETE FROM server_blocklists WHERE type IN ('ip', 'cidr') AND value = ?`, value)
	}
	if err == nil {
		count, _ := result.RowsAffected()
		if count == 0 {
			err = fmt.Errorf("blocked IP address was not found")
		} else {
			logSecurityAudit(r, "security.ip_block_removed", value, "")
		}
	}
	if wantsFormSubmission(r) {
		redirectAdminForm(w, r, adminPagePath(r, "/security/firewall"), "flash.settings_saved", "flash.settings_save_failed", err)
		return
	}
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "IP_BLOCK_NOT_FOUND", err.Error())
		return
	}
	RespondSuccess(w, r, "IP block removed")
}

// PurgeExpiredIPBlocks removes expired block records from the audit table.
func PurgeExpiredIPBlocks(w http.ResponseWriter, r *http.Request) {
	db := database.GetServerStore()
	if db == nil {
		InternalError(w, r, "server database is not available")
		return
	}
	expiredIDs, err := expiredIPBlockIDs(r.Context(), db, time.Now())
	if err != nil {
		InternalError(w, r, err.Error())
		return
	}
	var removed int64
	for _, id := range expiredIDs {
		result, err := database.ExecContext(r.Context(), db, database.TimeoutWrite, `DELETE FROM server_blocklists WHERE id = ?`, id)
		if err != nil {
			InternalError(w, r, err.Error())
			return
		}
		count, _ := result.RowsAffected()
		removed += count
	}
	logSecurityAudit(r, "security.expired_ip_blocks_purged", "", fmt.Sprintf("count=%d", removed))
	RespondSuccess(w, r, "Expired IP blocks purged", map[string]interface{}{"removed": removed})
}

func readIPBlocks(ctx context.Context, db *database.Store, activeOnly bool) ([]ipBlockRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("server database is not available")
	}
	rows, err := database.QueryContext(ctx, db, database.TimeoutSimpleSelect,
		`SELECT id, type, value, reason, added_at, expires_at, block_type, offense_count, auto_blocked, blocked_by
		 FROM server_blocklists WHERE type IN ('ip', 'cidr') ORDER BY added_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("WARNING: failed to close IP block rows: %v", err)
		}
	}()
	var records []ipBlockRecord
	now := time.Now()
	for rows.Next() {
		var record ipBlockRecord
		var kind, value string
		var reason, blockedBy sql.NullString
		var blockedAt, expiresAt sql.NullString
		if err := rows.Scan(&record.ID, &kind, &value, &reason, &blockedAt, &expiresAt, &record.Type, &record.OffenseCount, &record.AutoBlocked, &blockedBy); err != nil {
			return nil, err
		}
		if kind == "cidr" {
			record.CIDR = value
		} else {
			record.IP = value
		}
		record.Reason = reason.String
		record.BlockedBy = blockedBy.String
		if parsed, ok := dbtime.ParseStoredTimestamp(blockedAt.String); ok {
			record.BlockedAt = parsed.UTC().Format(time.RFC3339)
		} else {
			record.BlockedAt = blockedAt.String
		}
		if expiresAt.Valid {
			if parsed, ok := dbtime.ParseStoredTimestamp(expiresAt.String); ok {
				formatted := parsed.UTC().Format(time.RFC3339)
				record.ExpiresAt = &formatted
				if activeOnly && !parsed.After(now) {
					continue
				}
			}
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func expiredIPBlockIDs(ctx context.Context, db *database.Store, now time.Time) ([]int64, error) {
	rows, err := database.QueryContext(ctx, db, database.TimeoutSimpleSelect, `SELECT id, expires_at FROM server_blocklists WHERE expires_at IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("WARNING: failed to close expired IP block rows: %v", err)
		}
	}()
	var ids []int64
	for rows.Next() {
		var id int64
		var expiresAt string
		if err := rows.Scan(&id, &expiresAt); err != nil {
			return nil, err
		}
		if expiry, ok := dbtime.ParseStoredTimestamp(expiresAt); ok && !expiry.After(now) {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func classifyIPBlockValue(value string) (string, error) {
	if prefix, err := netip.ParsePrefix(value); err == nil {
		if prefix != prefix.Masked() {
			return "", fmt.Errorf("CIDR must have host bits cleared")
		}
		return "cidr", nil
	}
	if _, err := netip.ParseAddr(value); err != nil {
		return "", fmt.Errorf("a valid IP address or CIDR is required")
	}
	return "ip", nil
}

func parseBlockExpiry(value, blockType string) (interface{}, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		switch blockType {
		case "temporary":
			return dbtime.FormatSQLTimestamp(time.Now().Add(time.Hour)), nil
		case "extended":
			return dbtime.FormatSQLTimestamp(time.Now().Add(24 * time.Hour)), nil
		default:
			return nil, nil
		}
	}
	var expires time.Time
	var err error
	if expires, err = time.Parse(time.RFC3339, value); err != nil {
		expires, err = time.ParseInLocation("2006-01-02T15:04", value, time.Local)
	}
	if err != nil || !expires.After(time.Now()) {
		return nil, fmt.Errorf("expiry must be a future RFC 3339 timestamp or local date and time")
	}
	if blockType == "permanent" {
		return nil, fmt.Errorf("permanent blocks cannot have an expiry")
	}
	return dbtime.FormatSQLTimestamp(expires), nil
}

func reqctxString(r *http.Request, key string) string {
	value, _ := reqctx.GetValue(r.Context(), key)
	username, _ := value.(string)
	return username
}
