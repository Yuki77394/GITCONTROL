// Package audit provides a thin wrapper around the database audit log
// collection, plus helpers that ensure sensitive fields are never logged.
package audit

import (
	"context"
	"time"

	"github.com/swaggymusic/github-bot/internal/database"
	"github.com/swaggymusic/github-bot/internal/models"
)

// Result values.
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
	ResultDenied  = "denied"
)

// Logger wraps a DB handle.
type Logger struct {
	db *database.DB
}

// New creates a new audit Logger.
func New(db *database.DB) *Logger {
	return &Logger{db: db}
}

// Log records an audit entry. detail must NOT contain secrets.
func (l *Logger) Log(ctx context.Context, actorID int64, actorName, action, target, result, detail string, chatID int64) {
	if l == nil || l.db == nil {
		return
	}
	entry := &models.AuditLog{
		ActorID:   actorID,
		ActorName: actorName,
		Action:    action,
		Target:    target,
		ChatID:    chatID,
		Result:    result,
		Detail:    detail,
		CreatedAt: time.Now().UTC(),
	}
	// InsertAsync — fire and forget, but log on failure.
	go func(e *models.AuditLog) {
		_ = l.db.InsertAuditLog(ctx, e)
	}(entry)
}

// LogSync records an audit entry synchronously.
func (l *Logger) LogSync(ctx context.Context, actorID int64, actorName, action, target, result, detail string, chatID int64) error {
	if l == nil || l.db == nil {
		return nil
	}
	return l.db.InsertAuditLog(ctx, &models.AuditLog{
		ActorID:   actorID,
		ActorName: actorName,
		Action:    action,
		Target:    target,
		ChatID:    chatID,
		Result:    result,
		Detail:    detail,
		CreatedAt: time.Now().UTC(),
	})
}
