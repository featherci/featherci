package models

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "featherci_session"

	// SessionDuration is how long sessions remain valid.
	SessionDuration = 30 * 24 * time.Hour // 30 days

	// sessionIDLength is the byte length of session IDs before base64 encoding.
	sessionIDLength = 32
)

// Session represents an authenticated user session.
type Session struct {
	ID        string    `db:"id"`
	UserID    int64     `db:"user_id"`
	ExpiresAt time.Time `db:"expires_at"`
	CreatedAt time.Time `db:"created_at"`
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// SessionStore defines the interface for session management.
type SessionStore interface {
	Create(ctx context.Context, userID int64) (*Session, error)
	Get(ctx context.Context, sessionID string) (*Session, error)
	Delete(ctx context.Context, sessionID string) error
	DeleteAllForUser(ctx context.Context, userID int64) error
	Cleanup(ctx context.Context) error
}

// SQLiteSessionStore implements SessionStore using SQLite.
type SQLiteSessionStore struct {
	db *sqlx.DB
}

// NewSessionStore creates a new SQLite-backed session store.
func NewSessionStore(db *sqlx.DB) *SQLiteSessionStore {
	return &SQLiteSessionStore{db: db}
}

// Create generates a new session for the given user.
func (s *SQLiteSessionStore) Create(ctx context.Context, userID int64) (*Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	session := &Session{
		ID:        id,
		UserID:    userID,
		ExpiresAt: time.Now().Add(SessionDuration),
		CreatedAt: time.Now(),
	}

	query := `INSERT INTO sessions (id, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, query, session.ID, session.UserID, session.ExpiresAt, session.CreatedAt)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// Get retrieves a session by ID. Returns ErrNotFound if the session doesn't exist.
func (s *SQLiteSessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	var session Session
	query := `SELECT * FROM sessions WHERE id = ?`

	err := s.db.GetContext(ctx, &session, query, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &session, nil
}

// Delete removes a session by ID.
func (s *SQLiteSessionStore) Delete(ctx context.Context, sessionID string) error {
	query := `DELETE FROM sessions WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, sessionID)
	return err
}

// DeleteAllForUser removes all sessions for a given user.
func (s *SQLiteSessionStore) DeleteAllForUser(ctx context.Context, userID int64) error {
	query := `DELETE FROM sessions WHERE user_id = ?`
	_, err := s.db.ExecContext(ctx, query, userID)
	return err
}

// Cleanup removes all expired sessions.
func (s *SQLiteSessionStore) Cleanup(ctx context.Context) error {
	query := `DELETE FROM sessions WHERE expires_at < ?`
	_, err := s.db.ExecContext(ctx, query, time.Now())
	return err
}

// generateSessionID creates a cryptographically secure random session ID.
func generateSessionID() (string, error) {
	b := make([]byte, sessionIDLength)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// SetSessionCookie sets the session cookie on the response.
func SetSessionCookie(w http.ResponseWriter, session *Session, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// GetSessionFromRequest extracts the session ID from the request cookie.
func GetSessionFromRequest(r *http.Request) (string, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
	})
}
