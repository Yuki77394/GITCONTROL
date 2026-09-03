// Package database wraps the MongoDB driver, providing typed collections
// and indexes for the SWAGGYMUSIC GitHub Controller Bot.
//
// On startup, Connect() opens the connection, pings the server, and creates
// required indexes. MongoDB is NEVER exposed publicly by the default
// docker-compose configuration.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/swaggymusic/github-bot/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// DB wraps the mongo client and typed collections.
type DB struct {
	Client         *mongo.Client
	Database       *mongo.Database
	Users          *mongo.Collection
	GitHubAccounts *mongo.Collection
	Chats          *mongo.Collection
	OAuthStates    *mongo.Collection
	Webhooks       *mongo.Collection
	MessageCtx     *mongo.Collection
	AuditLogs      *mongo.Collection
}

// Connect dials MongoDB, pings it, and creates indexes.
func Connect(ctx context.Context, uri, dbName string) (*DB, error) {
	if uri == "" {
		return nil, errors.New("database: MONGODB_URI is empty")
	}
	if dbName == "" {
		return nil, errors.New("database: MONGODB_DATABASE is empty")
	}
	clientOpts := options.Client().ApplyURI(uri).
		SetConnectTimeout(10 * time.Second).
		SetServerSelectionTimeout(10 * time.Second)
	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("database: mongo.Connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	db := client.Database(dbName)
	d := &DB{
		Client:         client,
		Database:       db,
		Users:          db.Collection("users"),
		GitHubAccounts: db.Collection("github_accounts"),
		Chats:          db.Collection("chats"),
		OAuthStates:    db.Collection("oauth_states"),
		Webhooks:       db.Collection("webhooks"),
		MessageCtx:     db.Collection("message_contexts"),
		AuditLogs:      db.Collection("audit_logs"),
	}
	if err := d.createIndexes(ctx); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("database: createIndexes: %w", err)
	}
	return d, nil
}

// Disconnect closes the underlying client.
func (d *DB) Disconnect(ctx context.Context) error {
	if d == nil || d.Client == nil {
		return nil
	}
	return d.Client.Disconnect(ctx)
}

func (d *DB) createIndexes(ctx context.Context) error {
	type idx struct {
		coll  string
		model mongo.IndexModel
	}
	indexes := []idx{
		{
			coll: "github_accounts",
			model: mongo.IndexModel{
				Keys: bson.D{
					{Key: "telegram_id", Value: 1},
					{Key: "github_user_id", Value: 1},
				},
				Options: options.Index().SetUnique(true),
			},
		},
		{
			coll: "github_accounts",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "telegram_id", Value: 1}, {Key: "is_default", Value: 1}},
				Options: options.Index().SetSparse(true),
			},
		},
		{
			coll: "oauth_states",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "expires_at", Value: 1}},
				Options: options.Index().SetExpireAfterSeconds(900),
			},
		},
		{
			coll: "oauth_states",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "telegram_id", Value: 1}},
				Options: options.Index().SetSparse(true),
			},
		},
		{
			coll: "chats",
			model: mongo.IndexModel{
				Keys: bson.D{{Key: "links.repo_full_name", Value: 1}},
			},
		},
		{
			coll: "webhooks",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "chat_id", Value: 1}, {Key: "repo_full_name", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
		{
			coll: "message_contexts",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "chat_id", Value: 1}, {Key: "message_id", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
		{
			coll: "message_contexts",
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "expires_at", Value: 1}},
				Options: options.Index().SetExpireAfterSeconds(0),
			},
		},
		{
			coll: "audit_logs",
			model: mongo.IndexModel{
				Keys: bson.D{{Key: "created_at", Value: -1}},
			},
		},
		{
			coll: "audit_logs",
			model: mongo.IndexModel{
				Keys: bson.D{{Key: "actor_id", Value: 1}, {Key: "created_at", Value: -1}},
			},
		},
	}

	for _, ix := range indexes {
		coll := d.Database.Collection(ix.coll)
		_, err := coll.Indexes().CreateOne(ctx, ix.model)
		if err != nil {
			if !isIndexExistsErr(err) {
				return fmt.Errorf("create index on %s: %w", ix.coll, err)
			}
		}
	}
	return nil
}

func isIndexExistsErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return contains(s, "already exists") || contains(s, "IndexOptionsConflict") || contains(s, "An equivalent index already exists")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// User CRUD
// ---------------------------------------------------------------------------

func (d *DB) UpsertUser(ctx context.Context, u *models.User) error {
	if u.TelegramID == 0 {
		return errors.New("database: TelegramID required")
	}
	now := time.Now().UTC()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	filter := bson.M{"_id": u.TelegramID}
	update := bson.M{"$set": u}
	_, err := d.Users.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (d *DB) GetUser(ctx context.Context, telegramID int64) (*models.User, error) {
	var u models.User
	if err := d.Users.FindOne(ctx, bson.M{"_id": telegramID}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// ---------------------------------------------------------------------------
// GitHubAccount CRUD
// ---------------------------------------------------------------------------

func (d *DB) UpsertGitHubAccount(ctx context.Context, acc *models.GitHubAccount) error {
	if acc.TelegramID == 0 || acc.GitHubUserID == 0 {
		return errors.New("database: telegram_id and github_user_id required")
	}
	if acc.ID == "" {
		acc.ID = fmt.Sprintf("%d_%d", acc.TelegramID, acc.GitHubUserID)
	}
	now := time.Now().UTC()
	if acc.CreatedAt.IsZero() {
		acc.CreatedAt = now
	}
	acc.UpdatedAt = now
	filter := bson.M{"_id": acc.ID}
	update := bson.M{"$set": acc}
	_, err := d.GitHubAccounts.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (d *DB) GetGitHubAccount(ctx context.Context, telegramID int64) (*models.GitHubAccount, error) {
	var acc models.GitHubAccount
	err := d.GitHubAccounts.FindOne(ctx, bson.M{
		"telegram_id": telegramID,
		"$or": []bson.M{
			{"is_default": true},
			{"is_default": false},
		},
	}, options.FindOne().SetSort(bson.D{{Key: "is_default", Value: -1}, {Key: "updated_at", Value: -1}})).Decode(&acc)
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

func (d *DB) GetGitHubAccountByGHID(ctx context.Context, telegramID, ghUserID int64) (*models.GitHubAccount, error) {
	var acc models.GitHubAccount
	err := d.GitHubAccounts.FindOne(ctx, bson.M{
		"telegram_id":    telegramID,
		"github_user_id": ghUserID,
	}).Decode(&acc)
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

func (d *DB) ListGitHubAccounts(ctx context.Context, telegramID int64) ([]models.GitHubAccount, error) {
	cursor, err := d.GitHubAccounts.Find(ctx, bson.M{"telegram_id": telegramID}, options.Find().SetSort(bson.D{{Key: "is_default", Value: -1}, {Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var out []models.GitHubAccount
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *DB) SetDefaultGitHubAccount(ctx context.Context, telegramID int64, ghUserID int64) error {
	_, err := d.GitHubAccounts.UpdateMany(ctx,
		bson.M{"telegram_id": telegramID, "is_default": true},
		bson.M{"$set": bson.M{"is_default": false, "updated_at": time.Now().UTC()}},
	)
	if err != nil {
		return err
	}
	_, err = d.GitHubAccounts.UpdateOne(ctx,
		bson.M{"telegram_id": telegramID, "github_user_id": ghUserID},
		bson.M{"$set": bson.M{"is_default": true, "updated_at": time.Now().UTC()}},
	)
	return err
}

func (d *DB) DeleteGitHubAccount(ctx context.Context, telegramID, ghUserID int64) error {
	_, err := d.GitHubAccounts.DeleteOne(ctx, bson.M{
		"telegram_id":    telegramID,
		"github_user_id": ghUserID,
	})
	return err
}

func (d *DB) DeleteAllGitHubAccounts(ctx context.Context, telegramID int64) error {
	_, err := d.GitHubAccounts.DeleteMany(ctx, bson.M{"telegram_id": telegramID})
	return err
}

// ---------------------------------------------------------------------------
// Chat CRUD + repo links
// ---------------------------------------------------------------------------

func (d *DB) UpsertChat(ctx context.Context, c *models.Chat) error {
	if c.ID == 0 {
		return errors.New("database: chat ID required")
	}
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	filter := bson.M{"_id": c.ID}
	update := bson.M{
		"$set": bson.M{
			"chat_type":  c.ChatType,
			"title":      c.Title,
			"username":   c.Username,
			"updated_at": c.UpdatedAt,
		},
		"$setOnInsert": bson.M{
			"created_at": c.CreatedAt,
		},
	}
	_, err := d.Chats.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (d *DB) GetChat(ctx context.Context, chatID int64) (*models.Chat, error) {
	var c models.Chat
	if err := d.Chats.FindOne(ctx, bson.M{"_id": chatID}).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (d *DB) AddRepoLink(ctx context.Context, chatID int64, link models.RepoLink) error {
	_, _ = d.Chats.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{
		"$pull": bson.M{"links": bson.M{"repo_full_name": link.RepoFullName}},
	})
	_, err := d.Chats.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{
		"$push": bson.M{"links": link},
		"$set":  bson.M{"updated_at": time.Now().UTC()},
	})
	return err
}

func (d *DB) RemoveRepoLink(ctx context.Context, chatID int64, repoFullName string) error {
	_, err := d.Chats.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{
		"$pull": bson.M{"links": bson.M{"repo_full_name": repoFullName}},
		"$set":  bson.M{"updated_at": time.Now().UTC()},
	})
	return err
}

func (d *DB) GetChatLinks(ctx context.Context, chatID int64) ([]models.RepoLink, error) {
	c, err := d.GetChat(ctx, chatID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return []models.RepoLink{}, nil
		}
		return nil, err
	}
	if c.Links == nil {
		return []models.RepoLink{}, nil
	}
	return c.Links, nil
}

func (d *DB) GetRepoLink(ctx context.Context, chatID int64, repoFullName string) (*models.RepoLink, error) {
	links, err := d.GetChatLinks(ctx, chatID)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		if l.RepoFullName == repoFullName {
			return &l, nil
		}
	}
	return nil, errors.New("repo link not found")
}

func (d *DB) GetChatsForRepo(ctx context.Context, repoFullName string) ([]models.Chat, error) {
	cursor, err := d.Chats.Find(ctx, bson.M{"links.repo_full_name": repoFullName})
	if err != nil {
		return nil, err
	}
	var out []models.Chat
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (d *DB) UpdateRepoLinkEvents(ctx context.Context, chatID int64, repoFullName string, events []string, muted bool) error {
	filter := bson.M{"_id": chatID, "links.repo_full_name": repoFullName}
	update := bson.M{"$set": bson.M{
		"links.$.events": events,
		"links.$.muted":  muted,
		"updated_at":     time.Now().UTC(),
	}}
	_, err := d.Chats.UpdateOne(ctx, filter, update)
	return err
}

// ToggleRepoLinkMuted atomically flips the muted flag for a repo link.
func (d *DB) ToggleRepoLinkMuted(ctx context.Context, chatID int64, repoFullName string) (bool, error) {
	link, err := d.GetRepoLink(ctx, chatID, repoFullName)
	if err != nil {
		return false, err
	}
	newMuted := !link.Muted
	return newMuted, d.UpdateRepoLinkEvents(ctx, chatID, repoFullName, link.Events, newMuted)
}

// ToggleRepoLinkEvent atomically adds or removes a single event from a repo
// link's enabled-events list. Returns the new "enabled" state.
func (d *DB) ToggleRepoLinkEvent(ctx context.Context, chatID int64, repoFullName, eventName string) (bool, error) {
	link, err := d.GetRepoLink(ctx, chatID, repoFullName)
	if err != nil {
		return false, err
	}
	events := link.Events
	if events == nil {
		events = []string{}
	}
	// Check if already enabled.
	enabled := false
	for i, e := range events {
		if e == eventName {
			// Remove it.
			events = append(events[:i], events[i+1:]...)
			enabled = false
			break
		}
	}
	if !enabled && !containsStr(events, eventName) {
		// Was not in the list → add it.
		events = append(events, eventName)
		enabled = true
	}
	if err := d.UpdateRepoLinkEvents(ctx, chatID, repoFullName, events, link.Muted); err != nil {
		return false, err
	}
	return enabled, nil
}

// SetRepoLinkEventsAll sets all events to enabled (if enable=true) or
// disabled (if enable=false) for a repo link.
func (d *DB) SetRepoLinkEventsAll(ctx context.Context, chatID int64, repoFullName string, allEvents []string, enable bool) error {
	link, err := d.GetRepoLink(ctx, chatID, repoFullName)
	if err != nil {
		return err
	}
	var events []string
	if enable {
		events = append(events, allEvents...)
	} else {
		events = []string{}
	}
	return d.UpdateRepoLinkEvents(ctx, chatID, repoFullName, events, link.Muted)
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func (d *DB) UpdateRepoLinkName(ctx context.Context, chatID int64, webhookID int64, newRepoFullName string) error {
	filter := bson.M{"_id": chatID, "links.webhook_id": webhookID}
	update := bson.M{"$set": bson.M{"links.$.repo_full_name": newRepoFullName}}
	_, err := d.Chats.UpdateOne(ctx, filter, update)
	return err
}

func (d *DB) MuteThread(ctx context.Context, chatID int64, threadID int32) error {
	_, err := d.Chats.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{
		"$addToSet": bson.M{"muted_threads": threadID},
	})
	return err
}

func (d *DB) IsThreadMuted(ctx context.Context, chatID int64, threadID int32) (bool, error) {
	c, err := d.GetChat(ctx, chatID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return false, nil
		}
		return false, err
	}
	for _, id := range c.MutedThreads {
		if id == int64(threadID) {
			return true, nil
		}
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// OAuthState CRUD
// ---------------------------------------------------------------------------

func (d *DB) SaveOAuthState(ctx context.Context, s *models.OAuthState) error {
	if s.State == "" {
		return errors.New("database: state required")
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	if s.ExpiresAt.IsZero() {
		s.ExpiresAt = s.CreatedAt.Add(10 * time.Minute)
	}
	_, err := d.OAuthStates.ReplaceOne(ctx, bson.M{"_id": s.State}, s, options.Replace().SetUpsert(true))
	return err
}

func (d *DB) ConsumeOAuthState(ctx context.Context, state string) (*models.OAuthState, error) {
	filter := bson.M{
		"_id":        state,
		"used":       false,
		"expires_at": bson.M{"$gt": time.Now().UTC()},
	}
	update := bson.M{"$set": bson.M{"used": true}}
	var s models.OAuthState
	err := d.OAuthStates.FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&s)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ---------------------------------------------------------------------------
// Webhook config CRUD
// ---------------------------------------------------------------------------

func (d *DB) SaveWebhookConfig(ctx context.Context, w *models.WebhookConfig) error {
	if w.ID == "" {
		w.ID = fmt.Sprintf("%d_%s", w.ChatID, w.RepoFullName)
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now().UTC()
	}
	_, err := d.Webhooks.ReplaceOne(ctx, bson.M{"_id": w.ID}, w, options.Replace().SetUpsert(true))
	return err
}

func (d *DB) DeleteWebhookConfig(ctx context.Context, chatID int64, repoFullName string) error {
	_, err := d.Webhooks.DeleteOne(ctx, bson.M{"chat_id": chatID, "repo_full_name": repoFullName})
	return err
}

// ---------------------------------------------------------------------------
// MessageContext CRUD
// ---------------------------------------------------------------------------

func (d *DB) SaveMessageContext(ctx context.Context, mc *models.MessageContext) error {
	if mc.ID == "" {
		mc.ID = fmt.Sprintf("%d_%d", mc.ChatID, mc.MessageID)
	}
	if mc.CreatedAt.IsZero() {
		mc.CreatedAt = time.Now().UTC()
	}
	if mc.ExpiresAt.IsZero() {
		mc.ExpiresAt = mc.CreatedAt.Add(48 * time.Hour)
	}
	_, err := d.MessageCtx.ReplaceOne(ctx, bson.M{"_id": mc.ID}, mc, options.Replace().SetUpsert(true))
	return err
}

func (d *DB) GetMessageContext(ctx context.Context, chatID, messageID int64) (*models.MessageContext, error) {
	var mc models.MessageContext
	err := d.MessageCtx.FindOne(ctx, bson.M{"chat_id": chatID, "message_id": messageID}).Decode(&mc)
	if err != nil {
		return nil, err
	}
	return &mc, nil
}

// ---------------------------------------------------------------------------
// Audit log
// ---------------------------------------------------------------------------

func (d *DB) InsertAuditLog(ctx context.Context, a *models.AuditLog) error {
	if a.ID == "" {
		a.ID = fmt.Sprintf("%d_%d", time.Now().UnixNano(), a.ActorID)
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_, err := d.AuditLogs.InsertOne(ctx, a)
	return err
}

func (d *DB) ListAuditLogs(ctx context.Context, limit int64) ([]models.AuditLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	cursor, err := d.AuditLogs.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	var out []models.AuditLog
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
