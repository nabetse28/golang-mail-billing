// gmailclient/labels.go
package gmailclient

import (
	"fmt"

	"github.com/nabetse28/golang-mail-billing/logging"
	gmail "google.golang.org/api/gmail/v1"
)

// LabelService caches Gmail labels and can ensure labels by name.
type LabelService struct {
	srv      *gmail.Service
	user     string
	byName   map[string]*gmail.Label
	byID     map[string]*gmail.Label
}

// NewLabelService creates a LabelService and loads existing labels.
func NewLabelService(srv *gmail.Service, user string) (*LabelService, error) {
	ls := &LabelService{
		srv:    srv,
		user:   user,
		byName: make(map[string]*gmail.Label),
		byID:   make(map[string]*gmail.Label),
	}
	if err := ls.refresh(); err != nil {
		return nil, err
	}
	return ls, nil
}

// refresh loads all labels from Gmail into the cache.
func (ls *LabelService) refresh() error {
	resp, err := ls.srv.Users.Labels.List(ls.user).Do()
	if err != nil {
		return fmt.Errorf("failed to list labels: %w", err)
	}

	ls.byName = make(map[string]*gmail.Label)
	ls.byID = make(map[string]*gmail.Label)

	for _, l := range resp.Labels {
		ls.byName[l.Name] = l
		ls.byID[l.Id] = l
	}

	logging.Infof("Loaded %d labels from Gmail", len(ls.byName))
	return nil
}

// EnsureLabel ensures a label with the given name exists.
// If it already exists, returns its ID. Otherwise it creates it.
func (ls *LabelService) EnsureLabel(name string) (string, error) {
	if existing, ok := ls.byName[name]; ok {
		return existing.Id, nil
	}

	logging.Infof("Label %q not found, creating...", name)

	label := &gmail.Label{
		Name:                 name,
		LabelListVisibility:  "labelShow",
		MessageListVisibility: "show",
	}

	created, err := ls.srv.Users.Labels.Create(ls.user, label).Do()
	if err != nil {
		return "", fmt.Errorf("failed to create label %q: %w", name, err)
	}

	ls.byName[created.Name] = created
	ls.byID[created.Id] = created

	logging.Infof("Created label %q with ID %s", created.Name, created.Id)
	return created.Id, nil
}
