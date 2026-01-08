package gmailclient

import (
	"fmt"

	gmail "google.golang.org/api/gmail/v1"
)

func GetFromAndSubject(srv *gmail.Service, user, messageID string) (from string, subject string, err error) {
	msg, err := srv.Users.Messages.Get(user, messageID).Format("metadata").Do()
	if err != nil {
		return "", "", fmt.Errorf("get metadata: %w", err)
	}

	if msg.Payload == nil {
		return "", "", nil
	}

	for _, h := range msg.Payload.Headers {
		switch h.Name {
		case "From":
			from = h.Value
		case "Subject":
			subject = h.Value
		}
	}

	return from, subject, nil
}
