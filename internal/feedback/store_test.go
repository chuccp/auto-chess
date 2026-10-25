package feedback

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chuccp/go-web-frame/db"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		phone   string
		email   string
		message string
		wantErr bool
	}{
		{"message only", "", "", "希望增加棋种", false},
		{"phone and email", "138 0013 8000", "user@example.com", "ok", false},
		{"blank message", "", "", "   ", true},
		{"long message", "", "", strings.Repeat("棋", MaxMessageRunes+1), true},
		{"bad phone", "abc", "", "ok", true},
		{"short phone", "123", "", "ok", true},
		{"bad email", "", "not-an-email", "ok", true},
		{"empty email is fine", "", "  ", "ok", false},
	}
	for _, tc := range cases {
		err := Validate(tc.phone, tc.email, tc.message)
		if tc.wantErr && err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func TestFileStoreAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "feedback.jsonl")
	store := NewFileStore(path)
	entry := Entry{
		Time:    time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC),
		Phone:   "13800138000",
		Email:   "user@example.com",
		Message: "希望增加残局模式",
	}
	if err := store.Append(entry); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(Entry{Time: time.Now(), Message: "第二条"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"phone":"13800138000"`, `"email":"user@example.com"`, "希望增加残局模式", "第二条"} {
		if !strings.Contains(text, want) {
			t.Fatalf("file missing %q:\n%s", want, text)
		}
	}
	if lines := strings.Count(strings.TrimSpace(text), "\n"); lines != 1 {
		t.Fatalf("expected 2 lines, got %d:\n%s", lines+1, text)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("file mode = %o, want 600", mode)
		}
	}
}

func TestNewFileStoreDefaultPath(t *testing.T) {
	if got := NewFileStore(" ").Path(); got != defaultFeedbackFile {
		t.Fatalf("default path = %q", got)
	}
}

func TestDBStoreAppend(t *testing.T) {
	conn, err := db.ConnectionSQLite(filepath.Join(t.TempDir(), "feedback.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if raw, err := conn.GetGorm().DB(); err == nil {
			_ = raw.Close()
		}
	})
	store, err := NewDBStore(conn)
	if err != nil {
		t.Fatal(err)
	}
	entry := Entry{
		Time:    time.Now(),
		Phone:   "13800138000",
		Email:   "user@example.com",
		Message: "数据库反馈",
	}
	if err := store.Append(entry); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := conn.GetGorm().Model(&dbEntry{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("feedback rows = %d, want 1", count)
	}
	var row dbEntry
	if err := conn.GetGorm().First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Message != entry.Message || row.Phone != entry.Phone || row.Email != entry.Email {
		t.Fatalf("stored row = %+v", row)
	}
}
