package gmailclient

import (
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/nabetse28/golang-mail-billing/logging"
	gmail "google.golang.org/api/gmail/v1"
)

// getMessageDate extracts the Date header from the message and parses it.
func getMessageDate(msg *gmail.Message) time.Time {
	var dateStr string
	for _, h := range msg.Payload.Headers {
		if strings.EqualFold(h.Name, "Date") {
			dateStr = h.Value
			break
		}
	}

	if dateStr == "" {
		logging.Errorf("Message %s has no Date header, using current time", msg.Id)
		return time.Now()
	}

	t, err := mail.ParseDate(dateStr)
	if err != nil {
		logging.Errorf("Failed to parse Date %q for message %s: %v; using current time", dateStr, msg.Id, err)
		return time.Now()
	}
	return t
}

// GetMessageDateOnly fetches the message and returns its Date header as time.Time,
// without modifying labels or read status.
func GetMessageDateOnly(
	srv *gmail.Service,
	user string,
	messageID string,
) (time.Time, error) {
	msg, err := srv.Users.Messages.Get(user, messageID).Format("full").Do()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get message %s: %w", messageID, err)
	}
	return getMessageDate(msg), nil
}

// OrganizeMessageByDate applies a label like BaseLabel/YYYY/MM and marks the message as read.
// It returns the parsed message date so it can be reused for filesystem paths.
// If the message is already read, removing UNREAD is harmless.
func OrganizeMessageByDate(
	srv *gmail.Service,
	user string,
	baseLabel string,
	labelService *LabelService,
	messageID string,
) (time.Time, error) {
	// Get full message to read headers (including Date)
	msg, err := srv.Users.Messages.Get(user, messageID).Format("full").Do()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get message %s: %w", messageID, err)
	}

	msgDate := getMessageDate(msg)
	year := msgDate.Year()
	month := int(msgDate.Month())

	labelName := fmt.Sprintf("%s/%04d/%02d", baseLabel, year, month)
	labelID, err := labelService.EnsureLabel(labelName)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to ensure label %s: %w", labelName, err)
	}

	// Prepare modify request: add our label, remove UNREAD.
	modReq := &gmail.ModifyMessageRequest{
		AddLabelIds:    []string{labelID},
		RemoveLabelIds: []string{"UNREAD"},
	}

	_, err = srv.Users.Messages.Modify(user, messageID, modReq).Do()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to modify message %s: %w", messageID, err)
	}

	logging.Infof("Organized message %s -> label=%s (date=%s) and marked as read",
		messageID, labelName, msgDate.Format(time.RFC3339))

	return msgDate, nil
}
