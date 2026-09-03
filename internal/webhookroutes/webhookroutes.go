// Package webhookroutes provides a database-backed random route ID system for
// GitHub webhook URLs.
//
// # DESIGN
//
// The original SWAGGYMUSIC webhook routing used an encrypted token in the URL
// path: /webhook/{AES-GCM-encrypted "<chatID>:<topicID>"}. This is secure
// (the chat ID is encrypted with the server's key) but has two drawbacks:
//
//  1. The chat ID is encoded in the URL, so if the encryption key ever leaks,
//     all chat IDs are exposed.
//  2. Webhook URLs cannot be rotated without re-creating the webhook on GitHub
//     (because the URL is derived from the chat ID, not from a separate
//     revocable identifier).
//
// This package provides an alternative: random 32-byte route IDs stored in
// MongoDB. The URL becomes /webhook/{random_route_id}, where the route ID is
// opaque to GitHub and to anyone who sees the URL. A DB lookup on each
// delivery resolves the route ID to (chatID, topicID, repoFullName).
//
// Route IDs support:
//   - Rotation: a new route ID can be generated for an existing (chat, repo)
//     pair, and the old one is invalidated.
//   - Revocation: a route ID can be deleted, immediately blocking deliveries.
//   - No chat ID exposure: the URL contains only a random opaque token.
//
// # BACKWARD COMPATIBILITY
//
// The webhook server still accepts the old encrypted-token format. The
// handler first tries to look up the path as a route ID; if not found, it
// falls back to decrypting the path as an encrypted token. This means
// existing webhooks created with the old format continue to work, and new
// webhooks use the route ID format.
package webhookroutes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/swaggymusic/github-bot/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Store manages webhook route IDs in MongoDB.
type Store struct {
	coll *mongo.Collection
}

// New creates a Store backed by the given database. It creates the
// `webhook_routes` collection and its indexes.
func New(db *mongo.Database) (*Store, error) {
	if db == nil {
		return nil, errors.New("webhookroutes: nil database")
	}
	coll := db.Collection("webhook_routes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "chat_id", Value: 1}, {Key: "repo_full_name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil && !isIndexExistsErr(err) {
		return nil, fmt.Errorf("webhookroutes: create index: %w", err)
	}
	// Index on route_id for fast lookups (not unique because revoked routes
	// may share a route_id slot — but in practice route IDs are unique).
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "route_id", Value: 1}},
	})
	if err != nil && !isIndexExistsErr(err) {
		return nil, fmt.Errorf("webhookroutes: create route_id index: %w", err)
	}
	return &Store{coll: coll}, nil
}

// Create generates a new random route ID for the given (chatID, topicID,
// repoFullName) tuple and stores it. If a route already exists for the same
// (chatID, repoFullName), it is replaced.
//
// Returns the route ID (32-byte hex string, 64 chars).
func (s *Store) Create(ctx context.Context, chatID int64, topicID int32, repoFullName string) (string, error) {
	if chatID == 0 || repoFullName == "" {
		return "", errors.New("webhookroutes: chatID and repoFullName required")
	}
	routeID, err := generateRouteID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	filter := bson.M{"chat_id": chatID, "repo_full_name": repoFullName}
	update := bson.M{
		"$set": bson.M{
			"route_id":   routeID,
			"topic_id":   topicID,
			"rotated_at": now,
		},
		"$setOnInsert": bson.M{
			"revoked":    false,
			"created_at": now,
		},
	}
	_, err = s.coll.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return "", fmt.Errorf("webhookroutes: upsert: %w", err)
	}
	return routeID, nil
}

// Lookup resolves a route ID to its destination. Returns the stored route
// record, or mongo.ErrNoDocuments if not found or revoked.
func (s *Store) Lookup(ctx context.Context, routeID string) (*models.WebhookRoute, error) {
	if routeID == "" {
		return nil, errors.New("webhookroutes: empty routeID")
	}
	var r models.WebhookRoute
	err := s.coll.FindOne(ctx, bson.M{"route_id": routeID, "revoked": false}).Decode(&r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Rotate generates a new route ID for an existing (chatID, repoFullName)
// pair. The old route ID is immediately revoked. Returns the new route ID.
//
// Note: after rotation, the GitHub repository webhook URL must be updated
// to the new URL. Callers are responsible for that side effect.
func (s *Store) Rotate(ctx context.Context, chatID int64, repoFullName string) (string, error) {
	_, err := s.coll.UpdateOne(ctx,
		bson.M{"chat_id": chatID, "repo_full_name": repoFullName, "revoked": false},
		bson.M{"$set": bson.M{"revoked": true, "rotated_at": time.Now().UTC()}},
	)
	if err != nil {
		return "", fmt.Errorf("webhookroutes: revoke old: %w", err)
	}
	return s.Create(ctx, chatID, 0, repoFullName)
}

// Revoke marks a route ID as revoked, immediately blocking deliveries.
func (s *Store) Revoke(ctx context.Context, routeID string) error {
	if routeID == "" {
		return errors.New("webhookroutes: empty routeID")
	}
	_, err := s.coll.UpdateOne(ctx,
		bson.M{"route_id": routeID},
		bson.M{"$set": bson.M{"revoked": true, "rotated_at": time.Now().UTC()}},
	)
	return err
}

// Delete removes a route entirely (used when a repo is unlinked).
func (s *Store) Delete(ctx context.Context, chatID int64, repoFullName string) error {
	_, err := s.coll.DeleteOne(ctx, bson.M{"chat_id": chatID, "repo_full_name": repoFullName})
	return err
}

// generateRouteID returns a 32-byte random hex string (64 chars).
// Uses crypto/rand for cryptographic security.
func generateRouteID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("webhookroutes: rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateRouteID is exported for tests.
func GenerateRouteID() (string, error) { return generateRouteID() }

func isIndexExistsErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return contains(s, "already exists") || contains(s, "IndexOptionsConflict") || contains(s, "An equivalent index already exists")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
