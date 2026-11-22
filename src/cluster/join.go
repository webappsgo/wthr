package cluster

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"

	"github.com/webappsgo/wthr/src/common/dbtime"
	"github.com/webappsgo/wthr/src/config"
	"github.com/webappsgo/wthr/src/database"
	"github.com/webappsgo/wthr/src/server/service"
	"github.com/webappsgo/wthr/src/util"
)

const (
	joinTokenLifetime = 15 * time.Minute
	joinTokenLockout  = 90 * 24 * time.Hour
	joinTokenPrefix   = "node_"
	joinTokenAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	joinMarkerName    = ".cluster-joined"
)

var joinTokenPattern = regexp.MustCompile(`^node_[A-Za-z0-9]{32}$`)

var (
	ErrJoinTokenInvalid = errors.New("join token is invalid")
	ErrJoinTokenExpired = errors.New("join token has expired")
	ErrJoinTokenUsed    = errors.New("join token has already been used")
	ErrJoinTokenLocked  = errors.New("join token is locked")
)

// CreateJoinToken creates a one-time join token and stores only its SHA-256 hash.
func CreateJoinToken(ctx context.Context, db *sql.DB, createdBy string) (string, time.Time, error) {
	if db == nil || !database.IsRemoteDatabase(db) {
		return "", time.Time{}, errors.New("cluster joining requires a configured remote database")
	}
	if err := requireSharedCache(); err != nil {
		return "", time.Time{}, err
	}
	token, err := newJoinToken()
	if err != nil {
		return "", time.Time{}, err
	}
	hash := hashJoinToken(token)
	now := time.Now().UTC()
	expires := now.Add(joinTokenLifetime)
	_, err = database.ExecContext(ctx, db, database.TimeoutWrite, `INSERT INTO server_join_tokens (token, token_hash, created_by, created_at, expires_at, lockout_until) VALUES (?, ?, ?, ?, ?, NULL)`, hash, hash, createdBy, dbtime.FormatSQLTimestamp(now), dbtime.FormatSQLTimestamp(expires))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("store cluster join token: %w", err)
	}
	return token, expires, nil
}

func newJoinToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate cluster join token: %w", err)
	}
	var token strings.Builder
	token.WriteString(joinTokenPrefix)
	for _, value := range data {
		token.WriteByte(joinTokenAlphabet[int(value)%len(joinTokenAlphabet)])
	}
	return token.String(), nil
}

func hashJoinToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", hash[:])
}

func deriveJoinKey(token string) []byte {
	salt := sha256.Sum256([]byte("cluster_join_" + token))
	return argon2.IDKey([]byte(token), salt[:16], 3, 64*1024, 4, 32)
}

func encryptJoinPayload(plain []byte, token string) (string, error) {
	block, err := aes.NewCipher(deriveJoinKey(token))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plain, []byte("wthr-cluster-join-v1"))
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptJoinPayload(encoded, token string) ([]byte, error) {
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid encrypted bootstrap field")
	}
	block, err := aes.NewCipher(deriveJoinKey(token))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("encrypted bootstrap field is too short")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte("wthr-cluster-join-v1"))
	if err != nil {
		return nil, errors.New("bootstrap authentication failed")
	}
	return plain, nil
}

type JoinBootstrap struct {
	Cluster struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"cluster"`
	Database   map[string]any    `json:"database"`
	Cache      map[string]any    `json:"cache"`
	Encryption map[string]string `json:"encryption"`
	Secrets    map[string]string `json:"secrets"`
	TLSFiles   map[string]string `json:"tls_files"`
	Node       struct {
		SuggestedID string `json:"suggested_id"`
	} `json:"node"`
}

type joinSecrets struct {
	Shared     map[string]any    `json:"shared"`
	AppSecrets []appSecretRecord `json:"app_secrets"`
}

type appSecretRecord struct {
	Name      string  `json:"name"`
	Value     string  `json:"value"`
	CreatedAt string  `json:"created_at"`
	RotatedAt *string `json:"rotated_at"`
}

type joinFiles map[string]string

func buildJoinBootstrap(ctx context.Context, db *sql.DB, token string) (*JoinBootstrap, error) {
	if !database.IsRemoteDatabase(db) {
		return nil, errors.New("cluster joining requires a configured remote database")
	}
	if err := requireSharedCache(); err != nil {
		return nil, err
	}
	bootstrap := &JoinBootstrap{}
	clusterID, err := getOrCreateClusterID(ctx, db)
	if err != nil {
		return nil, err
	}
	bootstrap.Cluster.ID = clusterID
	bootstrap.Cluster.Name = "wthr cluster"
	cfg, err := config.CurrentConfig()
	if err != nil {
		return nil, err
	}
	bootstrap.Database = map[string]any{
		"driver":   cfg.Server.Database.Driver,
		"host":     cfg.Server.Database.Host,
		"port":     cfg.Server.Database.Port,
		"name":     cfg.Server.Database.Name,
		"username": cfg.Server.Database.Username,
		"sslmode":  cfg.Server.Database.SSLMode,
	}
	databaseURL, _, err := config.ReadSetting("server.database.url")
	if err != nil {
		return nil, err
	}
	if text := settingText(databaseURL); text != "" {
		encoded, err := encryptJoinPayload([]byte(text), token)
		if err != nil {
			return nil, err
		}
		bootstrap.Database["url_encrypted"] = encoded
	} else if password, _, err := config.ReadSetting("server.database.password"); err != nil {
		return nil, err
	} else if text := settingText(password); text != "" {
		encoded, err := encryptJoinPayload([]byte(text), token)
		if err != nil {
			return nil, err
		}
		bootstrap.Database["password_encrypted"] = encoded
	}
	bootstrap.Cache = map[string]any{
		"type": cfg.Server.Cache.Type, "host": cfg.Server.Cache.Host, "port": cfg.Server.Cache.Port,
		"db": cfg.Server.Cache.DB, "username": cfg.Server.Cache.Username, "tls": cfg.Server.Cache.TLS,
		"tls_skip_verify": cfg.Server.Cache.TLSSkipVerify,
	}
	cacheURL, _, err := config.ReadSetting("server.cache.url")
	if err != nil {
		return nil, err
	}
	if text := settingText(cacheURL); text != "" {
		encoded, err := encryptJoinPayload([]byte(text), token)
		if err != nil {
			return nil, err
		}
		bootstrap.Cache["url_encrypted"] = encoded
	} else if password, _, err := config.ReadSetting("server.cache.password"); err != nil {
		return nil, err
	} else if text := settingText(password); text != "" {
		encoded, err := encryptJoinPayload([]byte(text), token)
		if err != nil {
			return nil, err
		}
		bootstrap.Cache["password_encrypted"] = encoded
	}
	if cfg.Server.Security.EncryptionKey != "" {
		encoded, err := encryptJoinPayload([]byte(cfg.Server.Security.EncryptionKey), token)
		if err != nil {
			return nil, err
		}
		bootstrap.Encryption = map[string]string{"key_encrypted": encoded}
	} else {
		bootstrap.Encryption = map[string]string{}
	}
	sharedSecrets, err := config.ReadClusterSharedSecrets()
	if err != nil {
		return nil, err
	}
	secretPayload := joinSecrets{Shared: sharedSecrets, AppSecrets: []appSecretRecord{}}
	rows, err := database.QueryContext(ctx, db, database.TimeoutSimpleSelect, `SELECT name, value, created_at, rotated_at FROM app_secrets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var record appSecretRecord
		var rotatedAt sql.NullString
		if err := rows.Scan(&record.Name, &record.Value, &record.CreatedAt, &rotatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if rotatedAt.Valid {
			record.RotatedAt = &rotatedAt.String
		}
		secretPayload.AppSecrets = append(secretPayload.AppSecrets, record)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	requiredSecrets := map[string]bool{"installation_secret": false, "cookie_signing_key": false, "csrf_token_secret": false}
	for _, record := range secretPayload.AppSecrets {
		if _, required := requiredSecrets[record.Name]; required && record.Value != "" {
			requiredSecrets[record.Name] = true
		}
	}
	for name, present := range requiredSecrets {
		if !present {
			return nil, fmt.Errorf("required shared application secret %s is missing", name)
		}
	}
	encodedSecrets, err := json.Marshal(secretPayload)
	if err != nil {
		return nil, err
	}
	encryptedSecrets, err := encryptJoinPayload(encodedSecrets, token)
	if err != nil {
		return nil, err
	}
	bootstrap.Secrets = map[string]string{"encrypted": encryptedSecrets}
	files, fileNames, err := readConnectionTLSFiles()
	if err != nil {
		return nil, err
	}
	for field, name := range fileNames {
		if strings.HasPrefix(field, "server.database.") {
			bootstrap.Database[strings.TrimPrefix(field, "server.database.")] = name
		}
		if strings.HasPrefix(field, "server.cache.") {
			bootstrap.Cache[strings.TrimPrefix(field, "server.cache.")] = name
		}
	}
	encodedFiles, err := json.Marshal(files)
	if err != nil {
		return nil, err
	}
	encryptedFiles, err := encryptJoinPayload(encodedFiles, token)
	if err != nil {
		return nil, err
	}
	bootstrap.TLSFiles = map[string]string{"encrypted": encryptedFiles}
	hostname, _ := os.Hostname()
	bootstrap.Node.SuggestedID = hostname
	return bootstrap, nil
}

func readConnectionTLSFiles() (joinFiles, map[string]string, error) {
	files := make(joinFiles)
	fileNames := make(map[string]string)
	for _, field := range []string{"server.database.ssl_ca", "server.database.ssl_cert", "server.database.ssl_key", "server.cache.tls_ca", "server.cache.tls_cert", "server.cache.tls_key"} {
		value, exists, err := config.ReadSetting(field)
		if err != nil {
			return nil, nil, err
		}
		if !exists {
			continue
		}
		name := filepath.Base(settingText(value))
		if name == "." || name == string(filepath.Separator) || strings.Contains(name, "..") {
			return nil, nil, fmt.Errorf("invalid connection TLS filename")
		}
		data, err := os.ReadFile(filepath.Join(config.CurrentConfigDir(), "ssl", name))
		if err != nil {
			return nil, nil, fmt.Errorf("read connection TLS file %s: %w", name, err)
		}
		if len(data) == 0 || len(data) > 2<<20 {
			return nil, nil, fmt.Errorf("invalid connection TLS file size")
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		if previous, exists := files[name]; exists && previous != encoded {
			return nil, nil, fmt.Errorf("connection TLS filenames collide")
		}
		files[name] = encoded
		fileNames[field] = name
	}
	return files, fileNames, nil
}

func getOrCreateClusterID(ctx context.Context, db *sql.DB) (string, error) {
	var id string
	if err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect, `SELECT cluster_id FROM srv_config_meta WHERE id = 1`).Scan(&id); err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}
	candidate := "cluster_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	result, err := database.ExecContext(ctx, db, database.TimeoutWrite, `UPDATE srv_config_meta SET cluster_id = ? WHERE id = 1 AND cluster_id = ''`, candidate)
	if err != nil {
		return "", err
	}
	if _, err := result.RowsAffected(); err != nil {
		return "", err
	}
	if err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect, `SELECT cluster_id FROM srv_config_meta WHERE id = 1`).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func validateJoinToken(ctx context.Context, db *sql.DB, token string) error {
	if !joinTokenPattern.MatchString(token) {
		return ErrJoinTokenInvalid
	}
	var expires, usedAt, lockout sql.NullString
	if err := database.QueryRowContext(ctx, db, database.TimeoutSimpleSelect, `SELECT expires_at, used_at, lockout_until FROM server_join_tokens WHERE token_hash = ?`, hashJoinToken(token)).Scan(&expires, &usedAt, &lockout); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrJoinTokenInvalid
		}
		return err
	}
	now := time.Now().UTC()
	if lockout.Valid {
		if expiry, ok := dbtime.ParseStoredTimestamp(lockout.String); ok && now.Before(expiry) {
			return ErrJoinTokenLocked
		}
	}
	if usedAt.Valid {
		return ErrJoinTokenUsed
	}
	if expires.Valid {
		if expiry, ok := dbtime.ParseStoredTimestamp(expires.String); ok && !now.Before(expiry) {
			return ErrJoinTokenExpired
		}
	}
	return nil
}

// JoinHandler serves the public, one-time authenticated cluster bootstrap endpoint.
func JoinHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !secureJoinRequest(r) {
			writeJoinError(w, http.StatusUpgradeRequired, "TLS_REQUIRED", "cluster enrollment requires HTTPS")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var request struct {
			Token  string `json:"token"`
			NodeID string `json:"node_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJoinError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid join request")
			return
		}
		request.NodeID = strings.TrimSpace(request.NodeID)
		if request.NodeID == "" {
			request.NodeID, _ = os.Hostname()
		}
		if len(request.NodeID) > 128 || strings.ContainsAny(request.NodeID, "\x00\r\n") {
			writeJoinError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid node identity")
			return
		}
		if err := validateJoinToken(r.Context(), db, request.Token); err != nil {
			writeTokenFailure(w, err)
			return
		}
		bootstrap, err := buildJoinBootstrap(r.Context(), db, request.Token)
		if err != nil {
			log.Printf("cluster bootstrap payload failed")
			writeJoinError(w, http.StatusInternalServerError, "DATABASE_ERROR", "cluster bootstrap is unavailable")
			return
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeJoinError(w, http.StatusInternalServerError, "DATABASE_ERROR", "cluster bootstrap is unavailable")
			return
		}
		if err := consumeJoinTokenTx(r.Context(), tx, request.Token, request.NodeID); err != nil {
			_ = tx.Rollback()
			writeTokenFailure(w, err)
			return
		}
		if err := registerJoinedNodeTx(r.Context(), tx, request.NodeID, r.RemoteAddr); err != nil {
			_ = tx.Rollback()
			log.Printf("cluster node registration failed")
			writeJoinError(w, http.StatusInternalServerError, "DATABASE_ERROR", "cluster bootstrap is unavailable")
			return
		}
		if err := tx.Commit(); err != nil {
			writeJoinError(w, http.StatusInternalServerError, "DATABASE_ERROR", "cluster bootstrap is unavailable")
			return
		}
		writeJoinJSON(w, http.StatusOK, map[string]any{"ok": true, "data": bootstrap})
	}
}

func registerJoinedNodeTx(ctx context.Context, tx *sql.Tx, nodeID, remoteAddr string) error {
	hostname := nodeID
	if len(hostname) > 253 {
		hostname = hostname[:253]
	}
	ipText, _, splitErr := net.SplitHostPort(remoteAddr)
	if splitErr != nil {
		ipText = remoteAddr
	}
	ip := net.ParseIP(strings.Trim(ipText, "[]"))
	if ip == nil {
		ipText = ""
	} else {
		ipText = canonicalIP(ip)
	}
	joined := dbtime.FormatSQLTimestamp(time.Now().UTC())
	_, err := tx.ExecContext(ctx, `INSERT INTO server_nodes (node_id, hostname, ip_address, status, last_heartbeat, created_at, joined_at) VALUES (?, ?, ?, 'online', ?, ?, ?) ON CONFLICT(node_id) DO UPDATE SET hostname = excluded.hostname, ip_address = excluded.ip_address, status = 'online', last_heartbeat = excluded.last_heartbeat`, nodeID, hostname, ipText, joined, joined, joined)
	return err
}

func consumeJoinTokenTx(ctx context.Context, tx *sql.Tx, token, nodeID string) error {
	if !joinTokenPattern.MatchString(token) {
		return ErrJoinTokenInvalid
	}
	hash := hashJoinToken(token)
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE server_join_tokens SET used_at = ?, used_by = ?, lockout_until = ? WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?`, dbtime.FormatSQLTimestamp(now), nodeID, dbtime.FormatSQLTimestamp(now.Add(joinTokenLockout)), hash, dbtime.FormatSQLTimestamp(now))
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 1 {
		return nil
	}
	var expires, usedAt, lockout sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT expires_at, used_at, lockout_until FROM server_join_tokens WHERE token_hash = ?`, hash).Scan(&expires, &usedAt, &lockout); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrJoinTokenInvalid
		}
		return err
	}
	if lockout.Valid {
		if expiry, ok := dbtime.ParseStoredTimestamp(lockout.String); ok && now.Before(expiry) {
			return ErrJoinTokenLocked
		}
	}
	if usedAt.Valid {
		return ErrJoinTokenUsed
	}
	if expires.Valid {
		if expiry, ok := dbtime.ParseStoredTimestamp(expires.String); ok && !now.Before(expiry) {
			return ErrJoinTokenExpired
		}
	}
	return ErrJoinTokenInvalid
}

// PrepareClusterDatabase creates cluster metadata before the first node starts
// its manager. It is used by the conversion wizard while the running process
// still has its original SQLite handles open.
func PrepareClusterDatabase(db *sql.DB, nodeID, address string) error {
	manager := NewClusterManager(db, nodeID, address, true)
	if err := manager.initializeClusterTables(); err != nil {
		return err
	}
	if err := manager.initializeConfigSyncTables(); err != nil {
		return err
	}
	clusterID, err := getOrCreateClusterID(context.Background(), db)
	if err != nil {
		return err
	}
	if err := manager.writeConfigToSharedDatabase(); err != nil {
		return err
	}
	return config.WriteSetting("server.cluster.id", clusterID)
}

// LeaveCluster removes only the calling node from cluster membership and
// switches its next startup to fresh local SQLite and in-memory cache.
func LeaveCluster(ctx context.Context, db *sql.DB, dataDir, configDir, nodeID string) error {
	if db == nil || !database.IsRemoteDatabase(db) {
		return errors.New("this node is not connected to a cluster database")
	}
	tlsFiles := make(map[string]struct{})
	for _, key := range []string{"server.database.ssl_ca", "server.database.ssl_cert", "server.database.ssl_key", "server.cache.tls_ca", "server.cache.tls_cert", "server.cache.tls_key"} {
		value, _, err := config.ReadSetting(key)
		if err != nil {
			return err
		}
		if name := filepath.Base(settingText(value)); name != "." && name != string(filepath.Separator) && name != "" {
			tlsFiles[name] = struct{}{}
		}
	}
	for _, key := range []string{"server.ssl.cert", "server.ssl.key"} {
		value, _, err := config.ReadSetting(key)
		if err != nil {
			return err
		}
		delete(tlsFiles, filepath.Base(settingText(value)))
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM cluster_nodes WHERE node_id = ?`, nodeID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM server_nodes WHERE node_id = ?`, nodeID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, key := range []string{"server.database.url", "server.database.password", "server.database.driver", "server.database.host", "server.database.port", "server.database.name", "server.database.username", "server.database.sslmode", "server.database.ssl_ca", "server.database.ssl_cert", "server.database.ssl_key", "server.cache.url", "server.cache.password", "server.cache.host", "server.cache.port", "server.cache.username", "server.cache.db", "server.cache.tls", "server.cache.tls_skip_verify", "server.cache.tls_ca", "server.cache.tls_cert", "server.cache.tls_key", "server.cluster.id"} {
		if err := config.DeleteSyncedSetting(key); err != nil {
			return fmt.Errorf("remove cluster setting %s: %w", key, err)
		}
	}
	if err := config.WriteSetting("server.database.driver", "sqlite"); err != nil {
		return err
	}
	if err := config.WriteSetting("server.cache.type", "memory"); err != nil {
		return err
	}
	if err := config.WriteSetting("server.cluster.enabled", false); err != nil {
		return err
	}
	if strings.TrimSpace(dataDir) != "" {
		for _, name := range []string{"server.db", "users.db", "server.db-shm", "server.db-wal", "users.db-shm", "users.db-wal"} {
			if err := os.Remove(filepath.Join(dataDir, "db", name)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove stale local database %s: %w", name, err)
			}
		}
	}
	for name := range tlsFiles {
		if err := os.Remove(filepath.Join(configDir, "ssl", name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale connection TLS file %s: %w", name, err)
		}
	}
	if strings.TrimSpace(configDir) != "" {
		if err := os.Remove(filepath.Join(configDir, joinMarkerName)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove cluster enrollment marker: %w", err)
		}
	}
	return nil
}

func secureJoinRequest(r *http.Request) bool {
	if r.TLS != nil || strings.EqualFold(r.URL.Scheme, "https") {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") && util.IsTrustedProxyPeer(r) {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func writeTokenFailure(w http.ResponseWriter, err error) {
	status, code, message := http.StatusUnauthorized, "TOKEN_INVALID", "join token is invalid or unavailable"
	switch err {
	case ErrJoinTokenExpired:
		code, message = "TOKEN_EXPIRED", "join token has expired"
	case ErrJoinTokenUsed:
		code, message = "TOKEN_USED", "join token has already been used"
	case ErrJoinTokenLocked:
		code, message = "TOKEN_LOCKED", "join token is locked"
	case ErrJoinTokenInvalid: // Keep the generic invalid response.
	default:
		status, code, message = http.StatusInternalServerError, "DATABASE_ERROR", "cluster bootstrap is unavailable"
		log.Printf("cluster join token validation failed: %v", err)
	}
	writeJoinError(w, status, code, message)
}

func writeJoinError(w http.ResponseWriter, status int, code, message string) {
	writeJoinJSON(w, status, map[string]any{"ok": false, "error": code, "message": message})
}

func writeJoinJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requireSharedCache() error {
	cfg, err := config.CurrentConfig()
	if err != nil {
		return err
	}
	cacheType := strings.ToLower(strings.TrimSpace(cfg.Server.Cache.Type))
	if cacheType != "valkey" && cacheType != "redis" {
		return errors.New("cluster mode requires a shared Valkey or Redis cache")
	}
	return nil
}

// JoinFromEnvironment performs first-start cluster enrollment before server.yml
// is generated or the application opens its databases.
func JoinFromEnvironment(configDir, logDir string) error {
	endpoint := strings.TrimSpace(os.Getenv("CLUSTER_JOIN_URL"))
	token := strings.TrimSpace(os.Getenv("CLUSTER_JOIN_TOKEN"))
	if endpoint == "" && token == "" {
		return nil
	}
	if endpoint == "" || !joinTokenPattern.MatchString(token) {
		return errors.New("cluster join requires CLUSTER_JOIN_URL and a valid CLUSTER_JOIN_TOKEN")
	}
	parsedEndpoint, err := url.ParseRequestURI(endpoint)
	if err != nil || parsedEndpoint.Host == "" || parsedEndpoint.User != nil || parsedEndpoint.Fragment != "" || (parsedEndpoint.Scheme != "https" && parsedEndpoint.Scheme != "http") {
		return errors.New("cluster join URL is invalid")
	}
	if parsedEndpoint.Scheme == "http" {
		host := parsedEndpoint.Hostname()
		ip := net.ParseIP(host)
		if (ip == nil || !ip.IsLoopback()) && !strings.EqualFold(host, "localhost") {
			return errors.New("cluster join URL must use HTTPS")
		}
	}
	marker := filepath.Join(configDir, joinMarkerName)
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	nodeID, _ := os.Hostname()
	body, err := json.Marshal(map[string]string{"token": token, "node_id": nodeID})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid cluster join URL")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to cluster join endpoint: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 20<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("cluster join endpoint returned status %d", response.StatusCode)
	}
	var envelope struct {
		OK   bool          `json:"ok"`
		Data JoinBootstrap `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil || !envelope.OK {
		return errors.New("cluster join endpoint returned an invalid bootstrap response")
	}
	if err := installJoinBootstrap(configDir, logDir, envelope.Data, token); err != nil {
		return err
	}
	file, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("record cluster enrollment: %w", err)
	}
	if _, err := file.WriteString(time.Now().UTC().Format(time.RFC3339) + "\n"); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

func installJoinBootstrap(configDir, logDir string, bootstrap JoinBootstrap, token string) error {
	replaced := make(map[string]struct{})
	for _, key := range []string{"driver", "host", "port", "name", "username", "sslmode"} {
		if value, ok := bootstrap.Database[key]; ok {
			if err := config.WriteSetting("server.database."+key, value); err != nil {
				return err
			}
		}
	}
	if encoded := stringValue(bootstrap.Database["url_encrypted"]); encoded != "" {
		plain, err := decryptJoinPayload(encoded, token)
		if err != nil {
			return err
		}
		if err := config.WriteSetting("server.database.url", string(plain)); err != nil {
			return err
		}
		replaced["server.database.url"] = struct{}{}
	} else if encoded := stringValue(bootstrap.Database["password_encrypted"]); encoded != "" {
		plain, err := decryptJoinPayload(encoded, token)
		if err != nil {
			return err
		}
		if err := config.WriteSecretValue("server.database.password", string(plain)); err != nil {
			return err
		}
		replaced["server.database.password"] = struct{}{}
	}
	for _, key := range []string{"type", "host", "port", "db", "username", "tls", "tls_skip_verify"} {
		if value, ok := bootstrap.Cache[key]; ok {
			if err := config.WriteSetting("server.cache."+key, value); err != nil {
				return err
			}
		}
	}
	if encoded := stringValue(bootstrap.Cache["url_encrypted"]); encoded != "" {
		plain, err := decryptJoinPayload(encoded, token)
		if err != nil {
			return err
		}
		if err := config.WriteSetting("server.cache.url", string(plain)); err != nil {
			return err
		}
		replaced["server.cache.url"] = struct{}{}
	} else if encoded := stringValue(bootstrap.Cache["password_encrypted"]); encoded != "" {
		plain, err := decryptJoinPayload(encoded, token)
		if err != nil {
			return err
		}
		if err := config.WriteSecretValue("server.cache.password", string(plain)); err != nil {
			return err
		}
		replaced["server.cache.password"] = struct{}{}
	}
	if encoded := bootstrap.Encryption["key_encrypted"]; encoded != "" {
		plain, err := decryptJoinPayload(encoded, token)
		if err != nil {
			return err
		}
		if err := config.WriteSecretValue("server.security.encryption_key", string(plain)); err != nil {
			return err
		}
		replaced["server.security.encryption_key"] = struct{}{}
	}
	if encoded := bootstrap.Secrets["encrypted"]; encoded != "" {
		plain, err := decryptJoinPayload(encoded, token)
		if err != nil {
			return err
		}
		var values joinSecrets
		if err := json.Unmarshal(plain, &values); err != nil {
			return err
		}
		if len(values.AppSecrets) == 0 {
			return errors.New("cluster bootstrap is missing shared application secrets")
		}
		// These records are already authoritative in the shared SQL database the
		// joining node will open next; never generate or overwrite node-local copies.
		if err := config.WriteClusterSharedSecrets(values.Shared); err != nil {
			return err
		}
		for key := range values.Shared {
			replaced[key] = struct{}{}
		}
	}
	if encoded := bootstrap.TLSFiles["encrypted"]; encoded != "" {
		plain, err := decryptJoinPayload(encoded, token)
		if err != nil {
			return err
		}
		var files joinFiles
		if err := json.Unmarshal(plain, &files); err != nil {
			return err
		}
		sslDir := filepath.Join(configDir, "ssl")
		if err := os.MkdirAll(sslDir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(sslDir, 0o700); err != nil {
			return err
		}
		for name, encodedData := range files {
			if filepath.Base(name) != name || strings.Contains(name, "..") || strings.HasPrefix(name, ".") {
				return errors.New("invalid TLS filename in cluster bootstrap")
			}
			data, err := base64.StdEncoding.DecodeString(encodedData)
			if err != nil || len(data) > 2<<20 {
				return errors.New("invalid TLS data in cluster bootstrap")
			}
			if err := config.WritePrivateFile(filepath.Join(sslDir, name), data); err != nil {
				return err
			}
		}
		for key, value := range bootstrap.Database {
			if strings.HasSuffix(key, "ssl_ca") || strings.HasSuffix(key, "ssl_cert") || strings.HasSuffix(key, "ssl_key") {
				if err := config.WriteSetting("server.database."+key, value); err != nil {
					return err
				}
			}
		}
		for key, value := range bootstrap.Cache {
			if strings.HasSuffix(key, "tls_ca") || strings.HasSuffix(key, "tls_cert") || strings.HasSuffix(key, "tls_key") {
				if err := config.WriteSetting("server.cache."+key, value); err != nil {
					return err
				}
			}
		}
	}
	if err := config.WriteSetting("server.cluster.enabled", true); err != nil {
		return err
	}
	if err := config.WriteSetting("server.cluster.id", bootstrap.Cluster.ID); err != nil {
		return err
	}
	if err := logSecretReplacementAudit(logDir, replaced); err != nil {
		return err
	}
	return nil
}

func logSecretReplacementAudit(logDir string, keys map[string]struct{}) error {
	if len(keys) == 0 {
		return nil
	}
	logger, err := service.NewAuditLogger(logDir)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Close() }()
	for key := range keys {
		if err := logger.Log(service.AuditEvent{Event: "config.secret_replaced", Category: "configuration", Severity: "info", Actor: service.Actor{Type: "system", ID: "cluster_join"}, Details: map[string]any{"key": key}, Result: "success"}); err != nil {
			return err
		}
	}
	return nil
}

func stringValue(value any) string { text, _ := value.(string); return text }

func settingText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
