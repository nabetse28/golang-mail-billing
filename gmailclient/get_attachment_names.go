package gmailclient

import (
	gmail "google.golang.org/api/gmail/v1"
)

// GetAttachmentNames extracts the filenames of all attachments for a message,
// walking through nested payload parts.
func GetAttachmentNames(srv *gmail.Service, user, msgID string) ([]string, error) {
	msg, err := srv.Users.Messages.Get(user, msgID).Format("full").Do()
	if err != nil {
		return nil, err
	}

	var names []string

	var walk func(parts []*gmail.MessagePart)
	walk = func(parts []*gmail.MessagePart) {
		for _, p := range parts {
			if p.Filename != "" {
				names = append(names, p.Filename)
			}
			if p.Parts != nil {
				walk(p.Parts)
			}
		}
	}

	walk(msg.Payload.Parts)

	return names, nil
}
