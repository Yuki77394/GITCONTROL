// Package telegram wraps go-telegram-bot-api to provide a small, consistent
// API for sending messages with inline keyboards, answering callbacks, and
// handling commands. It also abstracts forum/topic support.
//
// Note on forum topics: tgbotapi v5 does not natively expose
// `message_thread_id` on BaseChat. To support forum topics, this package
// defines a small custom MessageConfig that adds the field and implements
// the tgbotapi.Chattable interface.
package telegram

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot wraps the underlying tgbotapi.BotAPI.
type Bot struct {
	api   *tgbotapi.BotAPI
	me    *tgbotapi.User
	debug bool
}

// New creates a Bot using the given token. If debug is true, the underlying
// library will log raw API calls.
func New(token string, debug bool) (*Bot, error) {
	if token == "" {
		return nil, errors.New("telegram: empty token")
	}
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram: NewBotAPI: %w", err)
	}
	api.Debug = debug
	me, err := api.GetMe()
	if err != nil {
		return nil, fmt.Errorf("telegram: GetMe: %w", err)
	}
	return &Bot{api: api, me: &me, debug: debug}, nil
}

// API returns the underlying BotAPI (use sparingly).
func (b *Bot) API() *tgbotapi.BotAPI { return b.api }

// Me returns the bot's own user.
func (b *Bot) Me() *tgbotapi.User { return b.me }

// Username returns the bot's @username (without @).
func (b *Bot) Username() string {
	if b.me == nil {
		return ""
	}
	return b.me.UserName
}

// topicMessageConfig is unused now — we use MakeRequest directly. Kept
// here as documentation of why we don't use tgbotapi.MessageConfig: it
// does not support message_thread_id.
type topicMessageConfig struct{}

// topicEditConfig is also unused for the same reason.
type topicEditConfig struct{}

// (end of unused config types)

// SendMessage sends a text message with optional markup. parseMode may be
// "HTML", "MarkdownV2", or "". topicID != 0 sends to a forum topic.
func (b *Bot) SendMessage(chatID int64, text string, parseMode string, replyToID int32, topicID int32, markup any) (int64, error) {
	params := tgbotapi.Params{}
	params.AddNonZero64("chat_id", chatID)
	params.AddNonZero("reply_to_message_id", int(replyToID))
	params.AddNonZero("message_thread_id", int(topicID))
	params.AddNonEmpty("parse_mode", parseMode)
	params.AddNonEmpty("text", text)
	// Always disable link previews in webhook notifications for cleaner output.
	params.AddBool("disable_web_page_preview", true)
	if err := params.AddInterface("reply_markup", markup); err != nil {
		return 0, fmt.Errorf("telegram: marshal markup: %w", err)
	}
	resp, err := b.api.MakeRequest("sendMessage", params)
	if err != nil {
		return 0, fmt.Errorf("telegram: sendMessage: %w", err)
	}
	var msg tgbotapi.Message
	if err := json.Unmarshal(resp.Result, &msg); err != nil {
		return 0, fmt.Errorf("telegram: unmarshal message: %w", err)
	}
	return int64(msg.MessageID), nil
}

// SendHTML is a convenience wrapper for SendMessage with HTML parse mode.
func (b *Bot) SendHTML(chatID int64, text string) (int64, error) {
	return b.SendMessage(chatID, text, tgbotapi.ModeHTML, 0, 0, nil)
}

// SendMarkdown is a convenience wrapper for MarkdownV2 parse mode.
func (b *Bot) SendMarkdown(chatID int64, text string) (int64, error) {
	return b.SendMessage(chatID, text, tgbotapi.ModeMarkdownV2, 0, 0, nil)
}

// EditText modifies an existing message's text.
func (b *Bot) EditText(chatID int64, messageID int64, text string, parseMode string, markup any) error {
	params := tgbotapi.Params{}
	params.AddNonZero64("chat_id", chatID)
	params.AddNonZero("message_id", int(messageID))
	params.AddNonEmpty("parse_mode", parseMode)
	params.AddNonEmpty("text", text)
	if err := params.AddInterface("reply_markup", markup); err != nil {
		return fmt.Errorf("telegram: marshal markup: %w", err)
	}
	_, err := b.api.MakeRequest("editMessageText", params)
	return err
}

// AnswerCallback answers a callback query, optionally showing a toast.
func (b *Bot) AnswerCallback(callbackID, text string, showAlert bool) error {
	cfg := tgbotapi.CallbackConfig{
		CallbackQueryID: callbackID,
		Text:            text,
		ShowAlert:       showAlert,
	}
	_, err := b.api.Request(cfg)
	return err
}

// DeleteMessage deletes a message.
func (b *Bot) DeleteMessage(chatID int64, messageID int64) error {
	cfg := tgbotapi.NewDeleteMessage(chatID, int(messageID))
	_, err := b.api.Request(cfg)
	return err
}

// GetUpdatesChan starts long-polling for updates.
func (b *Bot) GetUpdatesChan(ctx interface{}) tgbotapi.UpdatesChannel {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	return b.api.GetUpdatesChan(u)
}

// Stop closes the bot's update channel.
func (b *Bot) Stop() {
	b.api.StopReceivingUpdates()
}

// InlineKeyboard builds a 2D inline keyboard from a slice of rows.
// Each row is a slice of (text, callbackData) pairs.
func InlineKeyboard(rows [][]Button) any {
	if len(rows) == 0 {
		return nil
	}
	var kb tgbotapi.InlineKeyboardMarkup
	for _, row := range rows {
		var btns []tgbotapi.InlineKeyboardButton
		for _, b := range row {
			btns = append(btns, tgbotapi.NewInlineKeyboardButtonData(b.Text, b.Data))
		}
		kb.InlineKeyboard = append(kb.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(btns...))
	}
	return kb
}

// Button represents a single inline keyboard button with callback data.
type Button struct {
	Text string
	Data string
}

// MentionUser returns an HTML mention of the user.
func MentionUser(userID int64, name string) string {
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, userID, htmlEscape(name))
}

// htmlEscape escapes the minimal set of HTML characters for safe inclusion
// in HTML-parse-mode messages.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// EscapeMarkdownV2 escapes special characters per Telegram's MarkdownV2 spec.
func EscapeMarkdownV2(s string) string {
	const special = "_*[]()~`>#+-=|{}.!"
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(special, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsPrivate returns true if a chat is a private (1:1) chat.
func IsPrivate(chatType string) bool {
	return chatType == "private"
}

// IsGroup returns true for groups and supergroups.
func IsGroup(chatType string) bool {
	return chatType == "group" || chatType == "supergroup"
}
