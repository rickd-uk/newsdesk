package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookieName      = "newsdesk_session"
	sessionDuration        = 30 * 24 * time.Hour
	passwordHashIterations = 120000
	passwordHashKeyLen     = 32
)

var (
	errInvalidCredentials = errors.New("invalid credentials")
	errEmailTaken         = errors.New("email already exists")
	errUsernameTaken      = errors.New("username already exists")
)

type User struct {
	ID        int
	Username  string
	Email     string
	CreatedAt string
}

type Session struct {
	TokenHash string
	UserID    int
	ExpiresAt time.Time
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func randomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func pbkdf2SHA256(password string, salt []byte, iter, keyLen int) []byte {
	hLen := 32
	numBlocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, numBlocks*hLen)

	for block := 1; block <= numBlocks; block++ {
		u := pbkdf2Block([]byte(password), salt, block)
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iter; i++ {
			u = pbkdf2PRF([]byte(password), u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func pbkdf2Block(password, salt []byte, block int) []byte {
	msg := make([]byte, 0, len(salt)+4)
	msg = append(msg, salt...)
	msg = append(msg, byte(block>>24), byte(block>>16), byte(block>>8), byte(block))
	return pbkdf2PRF(password, msg)
}

func pbkdf2PRF(password, data []byte) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(data)
	return mac.Sum(nil)
}

func hashPassword(password string, salt []byte) string {
	derived := pbkdf2SHA256(password, salt, passwordHashIterations, passwordHashKeyLen)
	return hex.EncodeToString(derived)
}

func verifyPassword(password string, salt []byte, expectedHash string) bool {
	actual := hashPassword(password, salt)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expectedHash)) == 1
}

func validateCredentials(username, password string) error {
	if normalizeUsername(username) == "" {
		return errors.New("username is required")
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	return nil
}

func (db *DB) InitUsersTable() error {
	hasUsername, err := db.tableHasColumn("users", "username")
	if err != nil {
		return err
	}
	if !hasUsername {
		exists, err := db.tableExists("users")
		if err != nil {
			return err
		}
		if exists {
			if _, err := db.Exec(`ALTER TABLE users RENAME TO users_legacy_email_only`); err != nil {
				return fmt.Errorf("migrate users table: %w", err)
			}
			if err := db.createUsersTable(); err != nil {
				return err
			}
			_, err = db.Exec(`INSERT INTO users(id, username, email, password_hash, password_salt, created_at)
				SELECT id, LOWER(email), email, password_hash, password_salt, created_at
				FROM users_legacy_email_only
				WHERE TRIM(email) != ''`)
			if err != nil {
				return fmt.Errorf("copy legacy users: %w", err)
			}
			return nil
		}
	}
	return db.createUsersTable()
}

func (db *DB) createUsersTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		email TEXT,
		password_hash TEXT NOT NULL,
		password_salt TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique
		ON users(email) WHERE email IS NOT NULL AND email != ''`)
	return err
}

func (db *DB) InitSessionsTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS user_sessions (
		token_hash TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	)`)
	return err
}

func (db *DB) CreateUser(username, email, password string) (*User, error) {
	username = normalizeUsername(username)
	email = normalizeEmail(email)
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}

	salt, err := randomBytes(16)
	if err != nil {
		return nil, fmt.Errorf("generate password salt: %w", err)
	}
	res, err := db.Exec(
		`INSERT INTO users(username, email, password_hash, password_salt) VALUES(?,?,?,?)`,
		username,
		email,
		hashPassword(password, salt),
		base64.StdEncoding.EncodeToString(salt),
	)
	if err != nil {
		errText := strings.ToLower(err.Error())
		switch {
		case strings.Contains(errText, "users.username"), strings.Contains(errText, "username"):
			return nil, errUsernameTaken
		case strings.Contains(errText, "idx_users_email_unique"), strings.Contains(errText, "users.email"), strings.Contains(errText, "email"):
			return nil, errEmailTaken
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.GetUserByID(int(id))
}

func (db *DB) GetUserByID(id int) (*User, error) {
	var u User
	err := db.QueryRow(`SELECT id, username, COALESCE(email, ''), created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.Email, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (db *DB) AuthenticateUser(identifier, password string) (*User, error) {
	username := normalizeUsername(identifier)
	email := normalizeEmail(identifier)
	email = normalizeEmail(email)
	var (
		id           int
		storedUser   string
		storedEmail  string
		passwordHash string
		saltText     string
		createdAt    string
	)
	err := db.QueryRow(
		`SELECT id, username, COALESCE(email, ''), password_hash, password_salt, created_at
		FROM users
		WHERE username = ? OR email = ?
		LIMIT 1`,
		username, email,
	).Scan(&id, &storedUser, &storedEmail, &passwordHash, &saltText, &createdAt)
	if err != nil {
		return nil, errInvalidCredentials
	}
	salt, err := base64.StdEncoding.DecodeString(saltText)
	if err != nil {
		return nil, fmt.Errorf("decode password salt: %w", err)
	}
	if !verifyPassword(password, salt, passwordHash) {
		return nil, errInvalidCredentials
	}
	return &User{ID: id, Username: storedUser, Email: storedEmail, CreatedAt: createdAt}, nil
}

func (db *DB) CreateSession(userID int) (string, error) {
	raw, err := randomBytes(32)
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiresAt := time.Now().UTC().Add(sessionDuration)
	_, err = db.Exec(
		`INSERT INTO user_sessions(token_hash, user_id, expires_at) VALUES(?,?,?)`,
		hashSessionToken(token),
		userID,
		expiresAt.Format(time.RFC3339),
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (db *DB) GetUserBySessionToken(token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	var (
		u         User
		expiresAt string
	)
	err := db.QueryRow(`SELECT u.id, u.username, COALESCE(u.email, ''), u.created_at, s.expires_at
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?`,
		hashSessionToken(token),
	).Scan(&u.ID, &u.Username, &u.Email, &u.CreatedAt, &expiresAt)
	if err != nil {
		return nil, nil
	}
	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().UTC().After(expiry) {
		_, _ = db.Exec(`DELETE FROM user_sessions WHERE token_hash = ?`, hashSessionToken(token))
		return nil, nil
	}
	return &u, nil
}

func (db *DB) DeleteSession(token string) error {
	if token == "" {
		return nil
	}
	_, err := db.Exec(`DELETE FROM user_sessions WHERE token_hash = ?`, hashSessionToken(token))
	return err
}

func (s *Server) currentUser(r *http.Request) (*User, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return nil, nil
		}
		return nil, err
	}
	return s.db.GetUserBySessionToken(cookie.Value)
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
