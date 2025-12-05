// gmailclient/attachments.go
package gmailclient

import (
	"encoding/base64"
	"fmt"

	"github.com/nabetse28/golang-mail-billing/logging"
	"github.com/nabetse28/golang-mail-billing/storage"
	gmail "google.golang.org/api/gmail/v1"
)

// collectAttachmentParts recursively collects parts that are attachments.
func collectAttachmentParts(part *gmail.MessagePart, result *[]*gmail.MessagePart) {
	if part == nil {
		return
	}

	if part.Filename != "" && part.Body != nil && part.Body.AttachmentId != "" {
		*result = append(*result, part)
	}

	for _, p := range part.Parts {
		collectAttachmentParts(p, result)
	}
}

// DownloadAttachmentsToDir downloads all attachments of a message into the given directory.
func DownloadAttachmentsToDir(
	srv *gmail.Service,
	user string,
	messageID string,
	targetDir string,
) error {
	msg, err := srv.Users.Messages.Get(user, messageID).Format("full").Do()
	if err != nil {
		return fmt.Errorf("failed to get message %s for attachments: %w", messageID, err)
	}

	var attachmentParts []*gmail.MessagePart
	collectAttachmentParts(msg.Payload, &attachmentParts)

	if len(attachmentParts) == 0 {
		logging.Infof("Message %s has no attachments", messageID)
		return nil
	}

	for _, part := range attachmentParts {
		filename := part.Filename
		if filename == "" {
			filename = "attachment"
		}

		attID := part.Body.AttachmentId
		att, err := srv.Users.Messages.Attachments.Get(user, messageID, attID).Do()
		if err != nil {
			logging.Errorf("Failed to download attachment %s (message %s): %v", filename, messageID, err)
			continue
		}

		data, err := base64.URLEncoding.DecodeString(att.Data)
		if err != nil {
			logging.Errorf("Failed to decode attachment %s (message %s): %v", filename, messageID, err)
			continue
		}

		if _, err := storage.WriteFileUnique(targetDir, filename, data); err != nil {
			logging.Errorf("Failed to save attachment %s (message %s): %v", filename, messageID, err)
			continue
		}
	}

	return nil
}
