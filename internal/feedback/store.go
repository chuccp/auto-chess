// Package feedback stores visitor feedback and gates submissions with an
// ordered-character captcha.
package feedback

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chuccp/go-web-frame/config"
	"github.com/chuccp/go-web-frame/db"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	// MaxMessageRunes bounds the feedback text.
	MaxMessageRunes = 2000
	maxPhoneRunes   = 40
	maxEmailRunes   = 200

	defaultFeedbackFile = "data/feedback.jsonl"
	feedbackTable       = "feedback"
)

var (
	phonePattern = regexp.MustCompile(`^[0-9+\-() ]{5,40}$`)
	emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]{2,}$`)
)

// Entry is one feedback submission. Time is serialized as RFC 3339 in the file
// fallback and stored as a datetime in the database.
type Entry struct {
	Time    time.Time `json:"time"`
	Phone   string    `json:"phone,omitempty"`
	Email   string    `json:"email,omitempty"`
	Message string    `json:"message"`
}

// Store persists feedback. The service uses the configured web.db when one is
// available and otherwise appends JSONL to a local file.
type Store interface {
	Append(Entry) error
}

// Validate checks the optional contact fields and the required message. Phone
// and email are deliberately loose: a number or address is enough to reply.
func Validate(phone, email, message string) error {
	phone = strings.TrimSpace(phone)
	email = strings.TrimSpace(email)
	message = strings.TrimSpace(message)
	if message == "" {
		return errors.New("请填写意见或建议")
	}
	if utf8.RuneCountInString(message) > MaxMessageRunes {
		return fmt.Errorf("意见最多 %d 个字", MaxMessageRunes)
	}
	if phone != "" {
		if utf8.RuneCountInString(phone) > maxPhoneRunes || !phonePattern.MatchString(phone) {
			return errors.New("手机号 / 联系方式格式不正确")
		}
	}
	if email != "" {
		if utf8.RuneCountInString(email) > maxEmailRunes || !emailPattern.MatchString(email) {
			return errors.New("邮箱格式不正确")
		}
	}
	return nil
}

// NewStoreFromConfig prefers the web.db configuration. When no database is
// configured it falls back to a JSONL file. If a database is configured but
// cannot be opened, the fallback store is returned together with the error so
// the caller can log the misconfiguration and keep the service running.
func NewStoreFromConfig(cfg config.IConfig, fallbackFile string) (Store, error) {
	if cfg != nil {
		conn, err := db.CreateDB(cfg)
		if err == nil {
			store, migrateErr := NewDBStore(conn)
			if migrateErr == nil {
				return store, nil
			}
			return NewFileStore(fallbackFile), migrateErr
		}
		if !errors.Is(err, db.NoConfigDBError) {
			return NewFileStore(fallbackFile), err
		}
	}
	return NewFileStore(fallbackFile), nil
}

// FileStore appends one JSON object per line. The file and its directory are
// created with owner-only permissions because submissions may contain contact
// details.
type FileStore struct {
	mu   sync.Mutex
	path string
}

// NewFileStore returns a JSONL store. An empty path uses data/feedback.jsonl.
func NewFileStore(path string) *FileStore {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultFeedbackFile
	}
	return &FileStore{path: path}
}

// Path returns the JSONL file location.
func (s *FileStore) Path() string { return s.path }

// Append writes a single feedback line.
func (s *FileStore) Append(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("创建反馈目录：%w", err)
		}
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("打开反馈文件：%w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(entry); err != nil {
		_ = f.Close()
		return fmt.Errorf("写入反馈文件：%w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("关闭反馈文件：%w", err)
	}
	return nil
}

// dbEntry is the GORM model for the feedback table. Contact fields use
// varchar sizes; the message is TEXT.
type dbEntry struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time `gorm:"index;not null"`
	Phone     string    `gorm:"size:40"`
	Email     string    `gorm:"size:200"`
	Message   string    `gorm:"type:text;not null"`
}

// TableName keeps the table stable if the struct is renamed.
func (dbEntry) TableName() string { return feedbackTable }

// DBStore writes submissions to the configured database.
type DBStore struct {
	conn *db.DB
}

// NewDBStore ensures the feedback table exists and returns a database store.
func NewDBStore(conn *db.DB) (*DBStore, error) {
	if conn == nil {
		return nil, errors.New("数据库连接为空")
	}
	// Keep GORM's SQL logger quiet: insert statements contain phone numbers,
	// email addresses and message text, which must not reach the application log.
	session := conn.GetGorm().Session(&gorm.Session{Logger: logger.Discard})
	if err := session.AutoMigrate(&dbEntry{}); err != nil {
		return nil, fmt.Errorf("创建反馈表：%w", err)
	}
	return &DBStore{conn: conn}, nil
}

// Append inserts one feedback row.
func (s *DBStore) Append(entry Entry) error {
	row := dbEntry{
		CreatedAt: entry.Time,
		Phone:     entry.Phone,
		Email:     entry.Email,
		Message:   entry.Message,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now()
	}
	session := s.conn.GetGorm().Session(&gorm.Session{Logger: logger.Discard})
	if err := session.Create(&row).Error; err != nil {
		return fmt.Errorf("写入反馈表：%w", err)
	}
	return nil
}
