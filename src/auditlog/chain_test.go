package auditlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func auditTestDir(t *testing.T) string {
	t.Helper()
	root := filepath.Join(os.TempDir(), "webappsgo")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "wthr-audit-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func readAuditEntries(t *testing.T, path string) []map[string]json.RawMessage {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	var entries []map[string]json.RawMessage
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("audit entry is invalid JSON: %v", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestOpenAppendReopenAndClose(t *testing.T) {
	path := filepath.Join(auditTestDir(t), "nested", "audit.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logger.AppendJSON(map[string]any{"event": "first", "actor": "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("repeated Close() = %v", err)
	}

	logger, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logger.AppendJSON(map[string]any{"event": "second"}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	entries := readAuditEntries(t, path)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	var firstPrev, secondPrev string
	_ = json.Unmarshal(entries[0]["prev"], &firstPrev)
	_ = json.Unmarshal(entries[1]["prev"], &secondPrev)
	if firstPrev != genesisHash || secondPrev != hashLine(mustMarshal(t, entries[0])) {
		t.Fatalf("broken chain predecessors: first=%q second=%q", firstPrev, secondPrev)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("audit permissions = %o, want 640", info.Mode().Perm())
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSharedLoggerReferencesAndConcurrentAppends(t *testing.T) {
	path := filepath.Join(auditTestDir(t), "shared.jsonl")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.state != second.state {
		t.Fatal("canonical path did not reuse shared logger state")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := second.AppendJSON(map[string]any{"event": "still-open"}); err != nil {
		t.Fatalf("closing one reference closed the shared handle: %v", err)
	}

	const count = 24
	var group sync.WaitGroup
	errors := make(chan error, count)
	for i := 0; i < count; i++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			_, appendErr := second.AppendJSON(map[string]any{"event": "parallel", "index": index})
			errors <- appendErr
		}(i)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	entries := readAuditEntries(t, path)
	if len(entries) != count+1 {
		t.Fatalf("got %d entries, want %d", len(entries), count+1)
	}
	verified, err := verifyFile(path, genesisHash, filepath.Base(path), filepath.Base(path))
	if err != nil || verified.issueLine != 0 {
		t.Fatalf("concurrent appends broke chain: verification=%+v err=%v", verified, err)
	}
}

func TestAppendRejectsInvalidEntriesAndClosedLogger(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("Open(empty path) succeeded")
	}
	if _, err := (*Logger)(nil).AppendJSON(map[string]any{"event": "x"}); err == nil {
		t.Fatal("nil logger append succeeded")
	}
	path := filepath.Join(auditTestDir(t), "entries.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{nil, "not an object", 7, []string{"array"}} {
		if _, err := logger.AppendJSON(value); err == nil {
			t.Errorf("AppendJSON(%#v) succeeded", value)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := logger.AppendJSON(map[string]any{"event": "closed"}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("AppendJSON after Close() error = %v", err)
	}
	var zero Logger
	if err := zero.Rotate(); err == nil {
		t.Fatal("Rotate() on unopened logger succeeded")
	}
	if err := zero.Close(); err != nil {
		t.Fatalf("Close() on unopened logger = %v", err)
	}
}

func TestRotateAnchorsNewFileAndEmptyRotateIsNoop(t *testing.T) {
	path := filepath.Join(auditTestDir(t), "rotate.jsonl")
	logger, err := Open(path, Options{Keep: "forever"})
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Rotate(); err != nil {
		t.Fatal(err)
	}
	if _, err := logger.AppendJSON(map[string]any{"event": "before-rotate"}); err != nil {
		t.Fatal(err)
	}
	oldEntries := readAuditEntries(t, path)
	oldHash := hashLine(mustMarshal(t, oldEntries[0]))
	if err := logger.Rotate(); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(path + ".20*")
	if err != nil || len(archives) != 1 {
		t.Fatalf("archives=%v err=%v, want one archive", archives, err)
	}
	if _, err := logger.AppendJSON(map[string]any{"event": "after-rotate"}); err != nil {
		t.Fatal(err)
	}
	newEntries := readAuditEntries(t, path)
	var predecessor string
	_ = json.Unmarshal(newEntries[0]["prev"], &predecessor)
	if predecessor != oldHash {
		t.Fatalf("new file predecessor = %q, want archived hash %q", predecessor, oldHash)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	logger, err = Open(path)
	if err != nil {
		t.Fatalf("reopen rotated file: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRotateDefaultDeletesArchiveAndUsesCheckpoint(t *testing.T) {
	path := filepath.Join(auditTestDir(t), "default-retention.jsonl")
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logger.AppendJSON(map[string]any{"event": "before-rotate"}); err != nil {
		t.Fatal(err)
	}
	oldEntries := readAuditEntries(t, path)
	oldHash := hashLine(mustMarshal(t, oldEntries[0]))
	if err := logger.Rotate(); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(path + ".20*")
	if err != nil || len(archives) != 0 {
		t.Fatalf("default retention kept archives %v: %v", archives, err)
	}
	checkpoint, err := readCheckpoint(path)
	if err != nil || checkpoint != oldHash {
		t.Fatalf("checkpoint=%q err=%v, want %q", checkpoint, err, oldHash)
	}
	if _, err := logger.AppendJSON(map[string]any{"event": "after-rotate"}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	logger, err = Open(path)
	if err != nil {
		t.Fatalf("reopen checkpoint-anchored log: %v", err)
	}
	verification, err := verifyFile(path, checkpoint, filepath.Base(path), filepath.Base(path))
	if err != nil || verification.issueLine != 0 {
		t.Fatalf("checkpoint-anchored file did not verify: %+v err=%v", verification, err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCompressedAuditArchivePreservesPredecessor(t *testing.T) {
	path := filepath.Join(auditTestDir(t), "compressed.jsonl")
	logger, err := Open(path, Options{Keep: "forever", Compress: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logger.AppendJSON(map[string]any{"event": "compressed"}); err != nil {
		t.Fatal(err)
	}
	entries := readAuditEntries(t, path)
	want := hashLine(mustMarshal(t, entries[0]))
	if err := logger.Rotate(); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(path + ".20*.gz")
	if err != nil || len(archives) != 1 {
		t.Fatalf("compressed archives=%v err=%v", archives, err)
	}
	got, err := lastLineHash(archives[0])
	if err != nil || got != want {
		t.Fatalf("compressed predecessor=%q err=%v, want %q", got, err, want)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAuditRetentionKeepsNewestCountAndExpiresDates(t *testing.T) {
	path := filepath.Join(auditTestDir(t), "retention.jsonl")
	var archives []string
	for _, name := range []string{"20260101-a", "20260102-b", "20260103-c"} {
		archive := path + "." + name
		if err := os.WriteFile(archive, []byte("entry\n"), 0o640); err != nil {
			t.Fatal(err)
		}
		archives = append(archives, archive)
	}
	if err := applyRetention(path, "2"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(archives[0]); !os.IsNotExist(err) {
		t.Fatalf("oldest archive remains after count retention: %v", err)
	}
	oldTime := time.Now().AddDate(0, 0, -3)
	if err := os.Chtimes(archives[1], oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := applyRetention(path, "1d"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(archives[1]); !os.IsNotExist(err) {
		t.Fatalf("expired archive remains: %v", err)
	}
	if err := validateKeep("forever"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"-1d", "1y", "daily", ""} {
		if err := validateKeep(invalid); err == nil {
			t.Errorf("validateKeep(%q) succeeded", invalid)
		}
	}
}

func TestTamperedLogCreatesSingleIntegrityAlert(t *testing.T) {
	path := filepath.Join(auditTestDir(t), "tampered.jsonl")
	if err := os.WriteFile(path, []byte("{\"event\":\"forged\",\"prev\":\"bad\"}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	verification, err := verifyFile(path, genesisHash, filepath.Base(path), filepath.Base(path))
	if err != nil || verification.issueLine != 1 || verification.issueReported {
		t.Fatalf("initial verification = %+v, err=%v", verification, err)
	}
	logger, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	entries := readAuditEntries(t, path)
	if len(entries) != 2 || eventName(entries[1]) != "security.audit_chain_broken" {
		t.Fatalf("integrity alert missing: entries=%v", entries)
	}
	name, line := integrityAlertDetails(entries[1])
	if name != filepath.Base(path) || line != 1 {
		t.Fatalf("integrity alert details=(%q,%d)", name, line)
	}
	verification, err = verifyFile(path, genesisHash, filepath.Base(path), filepath.Base(path))
	if err != nil || !verification.issueReported {
		t.Fatalf("alert was not recognized on verification: %+v err=%v", verification, err)
	}
	logger, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if got := len(readAuditEntries(t, path)); got != 2 {
		t.Fatalf("reopening duplicated integrity alert; entries=%d", got)
	}
}

func TestInvalidCheckpointKeepsLoggerRunningAndReportsOnce(t *testing.T) {
	path := filepath.Join(auditTestDir(t), "checkpoint.jsonl")
	if err := os.WriteFile(path, nil, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checkpointPath(path), []byte("not-a-hash\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	logger, err := Open(path)
	if err != nil {
		t.Fatalf("invalid checkpoint prevented startup: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	entries := readAuditEntries(t, path)
	if len(entries) != 1 || eventName(entries[0]) != "security.audit_chain_broken" {
		t.Fatalf("checkpoint integrity alert missing: %v", entries)
	}
	name, line := integrityAlertDetails(entries[0])
	if name != filepath.Base(path)+".chain" || line != 1 {
		t.Fatalf("checkpoint alert details=(%q,%d)", name, line)
	}
	verification, err := verifyFile(path, genesisHash, filepath.Base(path)+".chain", filepath.Base(path))
	if err != nil || !verification.checkpointAlertReported {
		t.Fatalf("checkpoint alert not recognized: %+v err=%v", verification, err)
	}
	logger, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if got := len(readAuditEntries(t, path)); got != 1 {
		t.Fatalf("checkpoint integrity alert duplicated: entries=%d", got)
	}
}

func TestAuditChainHelpersAndArchivedPredecessor(t *testing.T) {
	dir := auditTestDir(t)
	path := filepath.Join(dir, "audit.jsonl")
	first := []byte("{\"event\":\"one\",\"prev\":\"" + genesisHash + "\"}")
	archive := path + ".20260101-000000.000000000"
	if err := os.WriteFile(archive, append(first, '\n'), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".20260102-directory", 0o700); err != nil {
		t.Fatal(err)
	}
	predecessor, err := archivedPredecessorHash(path)
	if err != nil || predecessor != hashLine(first) {
		t.Fatalf("archived predecessor=%q err=%v", predecessor, err)
	}
	if got := trimLineEnding([]byte("line\r\n")); string(got) != "line" {
		t.Fatalf("trimLineEnding = %q", got)
	}
	if !validHash(genesisHash) || validHash(strings.ToUpper(hashLine(first))) || validHash("bad") || !validHash(hashLine(first)) {
		t.Fatal("validHash accepted or rejected an unexpected value")
	}
	if eventName(map[string]json.RawMessage{"event": json.RawMessage(`"event-name"`)}) != "event-name" {
		t.Fatal("eventName did not decode event")
	}
	if name, line := integrityAlertDetails(map[string]json.RawMessage{"details": json.RawMessage(`{"file_name":"audit.jsonl","line_number":3}`)}); name != "audit.jsonl" || line != 3 {
		t.Fatalf("integrityAlertDetails=(%q,%d)", name, line)
	}
	if name, line := integrityAlertDetails(map[string]json.RawMessage{"details": json.RawMessage(`null`)}); name != "" || line != 0 {
		t.Fatalf("malformed integrityAlertDetails=(%q,%d)", name, line)
	}
	missing, err := verifyFile(filepath.Join(dir, "missing.jsonl"), genesisHash, "missing", "missing")
	if err != nil || missing.lineCount != 0 {
		t.Fatalf("missing file verification=%+v err=%v", missing, err)
	}
}
