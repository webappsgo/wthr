package auditlog

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

const genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

var errInvalidCheckpoint = errors.New("invalid audit chain checkpoint")

var (
	registryMu sync.Mutex
	registry   = make(map[string]*sharedLog)
)

// Logger appends JSON objects to one shared, hash-chained JSONL file.
type Logger struct {
	state   *sharedLog
	closeMu sync.Mutex
	closed  bool
}

// Options controls retention for rotated audit files. Keep accepts none,
// forever, a count, or a duration in days, weeks, or months.
type Options struct {
	Keep     string
	Compress bool
}

type sharedLog struct {
	path        string
	mu          sync.Mutex
	file        *os.File
	lastHash    string
	initialHash string
	keep        string
	compress    bool
	refs        int
}

type chainVerification struct {
	lastHash                string
	issueLine               int
	issueReported           bool
	checkpointAlertReported bool
	lineCount               int
}

// Open opens the append-only audit log. Loggers for the same canonical path
// share one file handle and lock, including their hash-chain state.
func Open(path string, options ...Options) (*Logger, error) {
	if path == "" {
		return nil, errors.New("audit log path is empty")
	}
	option := Options{Keep: "none"}
	if len(options) > 0 {
		option = options[0]
		if option.Keep == "" {
			option.Keep = "none"
		}
	}
	if err := validateKeep(option.Keep); err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve audit log path: %w", err)
	}
	absPath = filepath.Clean(absPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return nil, fmt.Errorf("create audit log directory: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("canonicalize audit log path: %w", err)
		}
		resolvedDir, dirErr := filepath.EvalSymlinks(filepath.Dir(absPath))
		if dirErr != nil {
			return nil, fmt.Errorf("canonicalize audit log directory: %w", dirErr)
		}
		canonical = filepath.Join(resolvedDir, filepath.Base(absPath))
	}
	absPath = filepath.Clean(canonical)

	registryMu.Lock()
	if state := registry[absPath]; state != nil {
		state.refs++
		state.mu.Lock()
		state.keep = option.Keep
		state.compress = option.Compress
		state.mu.Unlock()
		registryMu.Unlock()
		return &Logger{state: state}, nil
	}

	initialHash, err := archivedPredecessorHash(absPath)
	checkpointBroken := errors.Is(err, errInvalidCheckpoint)
	if err != nil {
		if !checkpointBroken {
			registryMu.Unlock()
			return nil, fmt.Errorf("read audit log predecessor: %w", err)
		}
		initialHash = genesisHash
	}
	state := &sharedLog{path: absPath, refs: 1, initialHash: initialHash, keep: option.Keep, compress: option.Compress}
	if state.initialHash == "" {
		state.initialHash = genesisHash
	}

	verify, err := verifyFile(absPath, state.initialHash, filepath.Base(absPath), filepath.Base(absPath))
	if err != nil {
		registryMu.Unlock()
		return nil, fmt.Errorf("verify audit log: %w", err)
	}
	if checkpointBroken && verify.issueLine == 0 {
		verify.issueLine = 1
		verify.issueReported = verify.checkpointAlertReported
	}
	state.lastHash = verify.lastHash
	if state.lastHash == "" {
		state.lastHash = state.initialHash
	}

	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		registryMu.Unlock()
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	if err := file.Chmod(0640); err != nil {
		_ = file.Close()
		registryMu.Unlock()
		return nil, fmt.Errorf("set audit log permissions: %w", err)
	}
	state.file = file
	registry[absPath] = state
	registryMu.Unlock()
	logger := &Logger{state: state}
	if verify.issueLine > 0 && !verify.issueReported {
		state.mu.Lock()
		reportName := filepath.Base(absPath)
		if checkpointBroken {
			reportName += ".chain"
		}
		err := appendIntegrityAlertLocked(state, reportName, verify.issueLine)
		state.mu.Unlock()
		if err != nil {
			if closeErr := logger.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close audit logger after verification failure: %w", closeErr))
			}
			return nil, fmt.Errorf("record audit chain verification failure: %w", err)
		}
	}
	return logger, nil
}

// Configure updates retention policy for the shared log used by all callers.
func (l *Logger) Configure(options Options) error {
	if l == nil || l.state == nil {
		return errors.New("audit logger is not open")
	}
	if options.Keep == "" {
		options.Keep = "none"
	}
	if err := validateKeep(options.Keep); err != nil {
		return err
	}
	l.state.mu.Lock()
	l.state.keep = options.Keep
	l.state.compress = options.Compress
	l.state.mu.Unlock()
	return nil
}

// AppendJSON appends one compact JSON object with its prev field set to the
// SHA-256 of the preceding exact line. It returns the exact bytes written.
func (l *Logger) AppendJSON(value any) ([]byte, error) {
	if l == nil || l.state == nil {
		return nil, errors.New("audit logger is not open")
	}
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.closed {
		return nil, errors.New("audit logger is closed")
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal audit entry: %w", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil || object == nil {
		return nil, errors.New("audit entry must be a JSON object")
	}

	state := l.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.file == nil {
		return nil, errors.New("audit log file is closed")
	}
	prev, err := json.Marshal(state.lastHash)
	if err != nil {
		return nil, fmt.Errorf("marshal audit chain hash: %w", err)
	}
	object["prev"] = prev
	line, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("marshal chained audit entry: %w", err)
	}
	output := append(append([]byte(nil), line...), '\n')
	written, err := state.file.Write(output)
	if err != nil {
		return nil, fmt.Errorf("append audit entry: %w", err)
	}
	if written != len(output) {
		return nil, io.ErrShortWrite
	}
	state.lastHash = hashLine(line)
	return output, nil
}

// Rotate verifies and archives the closed log, then starts a new file anchored
// to the previous file's last line hash.
func (l *Logger) Rotate() error {
	if l == nil || l.state == nil {
		return errors.New("audit logger is not open")
	}
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.closed {
		return errors.New("audit logger is closed")
	}

	state := l.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.file == nil {
		return errors.New("audit log file is closed")
	}
	if err := state.file.Sync(); err != nil {
		return fmt.Errorf("sync audit log before rotation: %w", err)
	}
	info, err := state.file.Stat()
	if err != nil {
		return fmt.Errorf("stat audit log before rotation: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}
	if err := state.file.Close(); err != nil {
		state.file = nil
		return fmt.Errorf("close audit log before rotation: %w", err)
	}
	state.file = nil

	archive := fmt.Sprintf("%s.%s", state.path, time.Now().UTC().Format("20060102-150405.000000000"))
	verification, err := verifyFile(state.path, state.initialHash, filepath.Base(archive), filepath.Base(state.path))
	if err != nil {
		state.file, _ = openAppend(state.path)
		return fmt.Errorf("verify closed audit log: %w", err)
	}
	if err := os.Rename(state.path, archive); err != nil {
		state.file, _ = openAppend(state.path)
		return fmt.Errorf("archive audit log: %w", err)
	}

	file, err := openAppend(state.path)
	if err != nil {
		return fmt.Errorf("open new audit log: %w", err)
	}
	state.file = file
	state.initialHash = verification.lastHash
	if state.initialHash == "" {
		state.initialHash = genesisHash
	}
	state.lastHash = state.initialHash
	if err := writeCheckpoint(state.path, state.initialHash); err != nil {
		return fmt.Errorf("write audit chain checkpoint: %w", err)
	}
	if state.compress {
		if err := compressArchive(archive); err != nil {
			return fmt.Errorf("compress audit archive: %w", err)
		}
	}
	if err := applyRetention(state.path, state.keep); err != nil {
		return fmt.Errorf("apply audit retention: %w", err)
	}
	if verification.issueLine > 0 && !verification.issueReported {
		if err := appendIntegrityAlertLocked(state, filepath.Base(archive), verification.issueLine); err != nil {
			return fmt.Errorf("record audit chain verification failure: %w", err)
		}
	}
	return nil
}

// Close releases one caller's reference to the shared file handle.
func (l *Logger) Close() error {
	if l == nil || l.state == nil {
		return nil
	}
	l.closeMu.Lock()
	if l.closed {
		l.closeMu.Unlock()
		return nil
	}
	l.closed = true
	l.closeMu.Unlock()

	registryMu.Lock()
	defer registryMu.Unlock()
	state := l.state
	state.refs--
	if state.refs > 0 {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(registry, state.path)
	if state.file == nil {
		return nil
	}
	file := state.file
	state.file = nil
	return file.Close()
}

func openAppend(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0640); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func (l *Logger) appendLocked(value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil || object == nil {
		return errors.New("audit entry must be a JSON object")
	}
	prev, err := json.Marshal(l.state.lastHash)
	if err != nil {
		return err
	}
	object["prev"] = prev
	line, err := json.Marshal(object)
	if err != nil {
		return err
	}
	output := append(append([]byte(nil), line...), '\n')
	written, err := l.state.file.Write(output)
	if err != nil {
		return err
	}
	if written != len(output) {
		return io.ErrShortWrite
	}
	l.state.lastHash = hashLine(line)
	return nil
}

func appendIntegrityAlertLocked(state *sharedLog, fileName string, lineNumber int) error {
	now := time.Now().UTC()
	entry := map[string]any{
		"id":       ulid.MustNew(ulid.Timestamp(now), rand.Reader).String(),
		"time":     now.Format("2006-01-02T15:04:05.000Z"),
		"event":    "security.audit_chain_broken",
		"category": "security",
		"severity": "critical",
		"actor":    map[string]string{"type": "system", "id": "audit_verifier", "ip": "", "user_agent": ""},
		"details":  map[string]any{"file_name": fileName, "line_number": lineNumber},
		"result":   "failure",
	}
	return (&Logger{state: state}).appendLocked(entry)
}

func verifyFile(path, initialHash, reportName, stableName string) (result chainVerification, returnErr error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close audit log after verification: %w", closeErr))
		}
	}()

	reader := bufio.NewReader(file)
	var previous []byte
	reported := make(map[int]bool)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			line = trimLineEnding(line)
			result.lineCount++
			var object map[string]json.RawMessage
			valid := json.Unmarshal(line, &object) == nil && object != nil
			prev := ""
			if valid {
				var value string
				if raw := object["prev"]; len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
					valid = false
				} else {
					prev = value
				}
				if eventName(object) == "security.audit_chain_broken" {
					fileName, alertLine := integrityAlertDetails(object)
					if (fileName == reportName || fileName == stableName || fileName == stableName+".chain") && alertLine > 0 {
						reported[alertLine] = true
						if fileName == stableName+".chain" && alertLine == 1 {
							result.checkpointAlertReported = true
						}
					}
				}
			}

			expected := initialHash
			if result.lineCount > 1 {
				expected = hashLine(previous)
			}
			if !valid || !validHash(prev) || prev != expected {
				if result.issueLine == 0 {
					result.issueLine = result.lineCount
				}
			}
			previous = append(previous[:0], line...)
			result.lastHash = hashLine(line)
		}
		if readErr != nil {
			if readErr != io.EOF {
				return result, readErr
			}
			break
		}
	}
	if result.issueLine > 0 {
		result.issueReported = reported[result.issueLine]
	}
	return result, nil
}

func eventName(object map[string]json.RawMessage) string {
	var name string
	_ = json.Unmarshal(object["event"], &name)
	return name
}

func integrityAlertDetails(object map[string]json.RawMessage) (string, int) {
	var details map[string]json.RawMessage
	if json.Unmarshal(object["details"], &details) != nil {
		return "", 0
	}
	var name string
	var line int
	_ = json.Unmarshal(details["file_name"], &name)
	_ = json.Unmarshal(details["line_number"], &line)
	return name, line
}

func archivedPredecessorHash(path string) (string, error) {
	files, err := filepath.Glob(path + ".20*")
	if err != nil {
		return "", err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	for _, archive := range files {
		info, err := os.Stat(archive)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		last, err := lastLineHash(archive)
		if err != nil {
			return "", err
		}
		if last != "" {
			return last, nil
		}
	}
	return readCheckpoint(path)
}

func lastLineHash(path string) (lastHash string, returnErr error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close audit archive: %w", closeErr))
		}
	}()
	var source io.Reader = file
	if strings.HasSuffix(path, ".gz") {
		compressed, err := gzip.NewReader(file)
		if err != nil {
			return "", err
		}
		defer func() {
			if closeErr := compressed.Close(); closeErr != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("close compressed audit archive: %w", closeErr))
			}
		}()
		source = compressed
	}
	reader := bufio.NewReader(source)
	var last []byte
	hasLast := false
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			last = trimLineEnding(line)
			hasLast = true
		}
		if readErr != nil {
			if readErr != io.EOF {
				return "", readErr
			}
			break
		}
	}
	if !hasLast {
		return "", nil
	}
	return hashLine(last), nil
}

func validateKeep(keep string) error {
	if keep == "none" || keep == "forever" {
		return nil
	}
	if keep == "" {
		return errors.New("audit retention is empty")
	}
	unit := keep[len(keep)-1]
	number := keep
	if unit < '0' || unit > '9' {
		number = keep[:len(keep)-1]
		if unit != 'd' && unit != 'w' && unit != 'm' {
			return fmt.Errorf("invalid audit retention %q", keep)
		}
	}
	count, err := strconv.Atoi(number)
	if err != nil || count < 0 {
		return fmt.Errorf("invalid audit retention %q", keep)
	}
	return nil
}

func applyRetention(path, keep string) error {
	files, err := filepath.Glob(path + ".20*")
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	if keep == "forever" {
		return nil
	}
	if keep == "none" {
		for _, file := range files {
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	unit := keep[len(keep)-1]
	if unit >= '0' && unit <= '9' {
		count, err := strconv.Atoi(keep)
		if err != nil {
			return err
		}
		if count > len(files) {
			count = len(files)
		}
		for _, file := range files[count:] {
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	count, _ := strconv.Atoi(keep[:len(keep)-1])
	cutoff := time.Now().UTC()
	switch unit {
	case 'd':
		cutoff = cutoff.AddDate(0, 0, -count)
	case 'w':
		cutoff = cutoff.AddDate(0, 0, -7*count)
	case 'm':
		cutoff = cutoff.AddDate(0, -count, 0)
	}
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func compressArchive(path string) error {
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	target := path + ".gz"
	destination, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		if closeErr := source.Close(); closeErr != nil {
			return errors.Join(err, fmt.Errorf("close audit log after archive open failure: %w", closeErr))
		}
		return err
	}
	writer := gzip.NewWriter(destination)
	_, copyErr := io.Copy(writer, source)
	closeWriterErr := writer.Close()
	closeFileErr := destination.Close()
	closeSourceErr := source.Close()
	archiveErr := errors.Join(copyErr, closeWriterErr, closeFileErr)
	if archiveErr != nil {
		archiveErr = errors.Join(archiveErr, closeSourceErr)
		if removeErr := os.Remove(target); removeErr != nil && !os.IsNotExist(removeErr) {
			archiveErr = errors.Join(archiveErr, fmt.Errorf("remove incomplete audit archive: %w", removeErr))
		}
		return archiveErr
	}
	if closeSourceErr != nil {
		if removeErr := os.Remove(target); removeErr != nil && !os.IsNotExist(removeErr) {
			closeSourceErr = errors.Join(closeSourceErr, fmt.Errorf("remove incomplete audit archive: %w", removeErr))
		}
		return fmt.Errorf("close source audit log after archive: %w", closeSourceErr)
	}
	return os.Remove(path)
}

func checkpointPath(path string) string { return path + ".chain" }

func readCheckpoint(path string) (string, error) {
	data, err := os.ReadFile(checkpointPath(path))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if !validHash(value) {
		return "", fmt.Errorf("%w: %s", errInvalidCheckpoint, checkpointPath(path))
	}
	return value, nil
}

func writeCheckpoint(path, hash string) (returnErr error) {
	if !validHash(hash) {
		return errors.New("invalid audit chain checkpoint hash")
	}
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, ".audit-chain-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !os.IsNotExist(removeErr) {
			returnErr = errors.Join(returnErr, fmt.Errorf("remove temporary audit checkpoint: %w", removeErr))
		}
	}()
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := io.WriteString(temporary, hash+"\n"); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, checkpointPath(path))
}

func trimLineEnding(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte{'\n'})
	return bytes.TrimSuffix(line, []byte{'\r'})
}

func hashLine(line []byte) string {
	hash := sha256.Sum256(line)
	return hex.EncodeToString(hash[:])
}

func validHash(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
