// Package models defines the MongoDB-backed data structures used across the
// SWAGGYMUSIC GitHub Controller Bot.
//
// All sensitive fields (OAuth tokens, PATs) are stored encrypted at rest
// and tagged with `json:"-"` so they never appear in API responses or logs.
package models

import "time"

// User represents a Telegram user linked to one or more GitHub accounts.
type User struct {
	TelegramID int64     `bson:"_id"             json:"telegram_id"`
	Username   string    `bson:"username"        json:"username,omitempty"`
	FirstName  string    `bson:"first_name"      json:"first_name,omitempty"`
	LastName   string    `bson:"last_name"       json:"last_name,omitempty"`
	IsOwner    bool      `bson:"is_owner"        json:"is_owner"`
	CreatedAt  time.Time `bson:"created_at"      json:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at"      json:"updated_at"`
	LastSeenAt time.Time `bson:"last_seen_at"    json:"last_seen_at"`
}

// GitHubAccount represents a GitHub account connected by a Telegram user.
// A user may have multiple GitHub accounts (e.g. personal + work).
type GitHubAccount struct {
	ID              string     `bson:"_id"             json:"id"`
	TelegramID      int64      `bson:"telegram_id"     json:"telegram_id"`
	GitHubUserID    int64      `bson:"github_user_id"  json:"github_user_id"`
	GitHubUsername  string     `bson:"github_username" json:"github_username"`
	GitHubAvatarURL string     `bson:"avatar_url"      json:"avatar_url,omitempty"`
	AuthMethod      string     `bson:"auth_method"     json:"auth_method"` // "oauth" | "pat"
	EncryptedToken  string     `bson:"encrypted_token" json:"-"`
	TokenScopes     []string   `bson:"scopes"          json:"scopes"`
	APIURL          string     `bson:"api_url"         json:"api_url"`
	IsDefault       bool       `bson:"is_default"      json:"is_default"`
	CreatedAt       time.Time  `bson:"created_at"      json:"created_at"`
	UpdatedAt       time.Time  `bson:"updated_at"      json:"updated_at"`
	ExpiresAt       *time.Time `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
	LastValidatedAt time.Time  `bson:"last_validated_at" json:"last_validated_at"`
}

// RepoLink represents a link between a Telegram chat and a GitHub repo,
// along with per-chat notification settings.
type RepoLink struct {
	RepoFullName string   `bson:"repo_full_name"     json:"repo_full_name"`
	WebhookID    int64    `bson:"webhook_id,omitempty" json:"webhook_id,omitempty"`
	Events       []string `bson:"events"             json:"events"` // enabled events
	Muted        bool     `bson:"muted"              json:"muted"`
}

// Chat represents a Telegram chat (group, supergroup, channel, or private).
type Chat struct {
	ID           int64      `bson:"_id"               json:"chat_id"`
	ChatType     string     `bson:"chat_type"         json:"chat_type"`
	Title        string     `bson:"title"             json:"title"`
	Username     string     `bson:"username,omitempty" json:"username,omitempty"`
	Links        []RepoLink `bson:"links"             json:"links"`
	MutedThreads []int64    `bson:"muted_threads,omitempty" json:"muted_threads,omitempty"`
	CreatedAt    time.Time  `bson:"created_at"        json:"created_at"`
	UpdatedAt    time.Time  `bson:"updated_at"        json:"updated_at"`
}

// OAuthState tracks an in-flight OAuth flow. Stored both in DB and in cache.
type OAuthState struct {
	State      string    `bson:"_id"          json:"state"`
	TelegramID int64     `bson:"telegram_id"  json:"telegram_id"`
	ChatID     int64     `bson:"chat_id,omitempty" json:"chat_id,omitempty"`
	CreatedAt  time.Time `bson:"created_at"   json:"created_at"`
	ExpiresAt  time.Time `bson:"expires_at"   json:"expires_at"`
	Used       bool      `bson:"used"         json:"used"`
}

// WebhookConfig stores per-chat/repo webhook metadata.
type WebhookConfig struct {
	ID           string    `bson:"_id"           json:"id"`
	ChatID       int64     `bson:"chat_id"       json:"chat_id"`
	TopicID      int32     `bson:"topic_id,omitempty" json:"topic_id,omitempty"`
	RepoFullName string    `bson:"repo_full_name" json:"repo_full_name"`
	WebhookID    int64     `bson:"webhook_id"    json:"webhook_id"`
	CreatedAt    time.Time `bson:"created_at"    json:"created_at"`
}

// MessageContext maps a Telegram (chat_id, message_id) pair to a GitHub
// object (issue/PR/comment) so that replies in Telegram can be forwarded
// back to GitHub as comments.
type MessageContext struct {
	ID          string    `bson:"_id"          json:"id"`
	ChatID      int64     `bson:"chat_id"      json:"chat_id"`
	MessageID   int64     `bson:"message_id"   json:"message_id"`
	Owner       string    `bson:"owner"        json:"owner"`
	Repo        string    `bson:"repo"         json:"repo"`
	IssueNumber int       `bson:"issue_number" json:"issue_number"`
	CommentID   int64     `bson:"comment_id,omitempty" json:"comment_id,omitempty"`
	Type        string    `bson:"type"         json:"type"` // issue, pr, issue_comment, pr_review, pr_review_comment
	CreatedAt   time.Time `bson:"created_at"   json:"created_at"`
	ExpiresAt   time.Time `bson:"expires_at"   json:"expires_at"`
}

// AuditLog records security-sensitive actions.
type AuditLog struct {
	ID        string    `bson:"_id"      json:"id"`
	ActorID   int64     `bson:"actor_id" json:"actor_id"`
	ActorName string    `bson:"actor_name,omitempty" json:"actor_name,omitempty"`
	Action    string    `bson:"action"   json:"action"`
	Target    string    `bson:"target"   json:"target"`
	ChatID    int64     `bson:"chat_id,omitempty" json:"chat_id,omitempty"`
	Result    string    `bson:"result"   json:"result"` // success, failure, denied
	Detail    string    `bson:"detail,omitempty" json:"detail,omitempty"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

// WebhookRoute is a database-backed random route ID for GitHub webhook URLs.
// Stored in the `webhook_routes` collection.
//
// The URL /webhook/{route_id} is opaque to GitHub — it contains no chat ID
// or topic ID. A DB lookup on each delivery resolves the route to its
// destination. Route IDs can be rotated or revoked independently of the
// GitHub webhook configuration.
type WebhookRoute struct {
	RouteID      string    `bson:"route_id"       json:"route_id"`
	ChatID       int64     `bson:"chat_id"        json:"chat_id"`
	TopicID      int32     `bson:"topic_id,omitempty" json:"topic_id,omitempty"`
	RepoFullName string    `bson:"repo_full_name" json:"repo_full_name"`
	Revoked      bool      `bson:"revoked"        json:"revoked"`
	CreatedAt    time.Time `bson:"created_at"     json:"created_at"`
	RotatedAt    time.Time `bson:"rotated_at"     json:"rotated_at"`
}
