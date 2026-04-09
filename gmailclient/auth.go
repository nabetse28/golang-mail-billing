// gmailclient/auth.go
package gmailclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/nabetse28/golang-mail-billing/logging"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// TokenFile is the default file used to store the OAuth token.
const TokenFile = "token.json"

// RedirectURL is the local URL used for the OAuth redirect.
// It must match the redirect URI configured in the Google Cloud console.
const RedirectURL = "http://localhost:8080/oauth2callback"

// NewService creates a new Gmail service using OAuth2 credentials and scopes.
func NewService(ctx context.Context, credentialsPath string, scopes ...string) (*gmail.Service, error) {
	credentialsBytes, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file %s: %w", credentialsPath, err)
	}

	config, err := google.ConfigFromJSON(credentialsBytes, scopes...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse credentials JSON: %w", err)
	}

	// Override redirect URL to use our local HTTP server.
	config.RedirectURL = RedirectURL

	client, err := getClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth2 client: %w", err)
	}

	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gmail service: %w", err)
	}

	return srv, nil
}

// getClient returns an HTTP client authenticated with OAuth2.
// It loads a token from TokenFile or starts the web-based authorization flow
// if the token does not exist yet.
func getClient(config *oauth2.Config) (*http.Client, error) {
	tok, err := tokenFromFile(TokenFile)
	if err == nil {
		refreshedTok, err := refreshToken(config, tok)
		if err != nil {
			if isInvalidGrant(err) {
				logging.Infof("Token expirado o revocado (invalid_grant). Eliminando %s y reautenticando...", TokenFile)
				if rmErr := os.Remove(TokenFile); rmErr != nil && !os.IsNotExist(rmErr) {
					return nil, fmt.Errorf("failed to remove invalid token file: %w", rmErr)
				}
				tok = nil
			} else {
				return nil, fmt.Errorf("failed to refresh OAuth token: %w", err)
			}
		} else {
			tok = refreshedTok
			if err := saveToken(TokenFile, tok); err != nil {
				return nil, fmt.Errorf("failed to save token to file: %w", err)
			}
		}
	}

	if tok == nil {
		logging.Infof("No existing token found, starting OAuth flow...")
		tok, err = getTokenFromWeb(config)
		if err != nil {
			return nil, fmt.Errorf("failed to get token from web: %w", err)
		}
		if err := saveToken(TokenFile, tok); err != nil {
			return nil, fmt.Errorf("failed to save token to file: %w", err)
		}
	}

	return config.Client(context.Background(), tok), nil
}

// tokenFromFile reads an OAuth2 token from a local file.
func tokenFromFile(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tok oauth2.Token
	if err := json.NewDecoder(f).Decode(&tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

// getTokenFromWeb starts the OAuth2 authorization flow using a local HTTP server
// to receive the authorization code via the redirect URI.
func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	codeCh := make(chan string)
	errCh := make(chan error, 1)

	// HTTP handler that receives the OAuth2 redirect with the authorization code.
	http.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		code := query.Get("code")
		if code == "" {
			http.Error(w, "Missing 'code' in query parameters", http.StatusBadRequest)
			errCh <- fmt.Errorf("missing code in OAuth callback")
			return
		}

		// Show a friendly page so the user knows the flow is complete.
		fmt.Fprintln(w, `
<html>
  <head><title>Gmail Organizer</title></head>
  <body>
    <h1>Authorization completed</h1>
    <p>You can now return to the application.</p>
    <p>You may close this window.</p>
  </body>
</html>`)

		logging.Infof("Received authorization code from browser redirect")
		codeCh <- code
	})

	// Start the local HTTP server in a goroutine.
	server := &http.Server{Addr: ":8080"}

	go func() {
		logging.Infof("Starting local OAuth callback server on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Errorf("OAuth callback server error: %v", err)
			errCh <- err
		}
	}()

	// Create the authorization URL (uses config.RedirectURL).
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Open this URL in your browser and authorize the application:\n%v\n", authURL)

	// Wait until we either get a code or an error.
	var code string
	select {
	case c := <-codeCh:
		code = c
	case err := <-errCh:
		// Try to shut down the server if there was an error.
		_ = server.Shutdown(context.Background())
		return nil, fmt.Errorf("failed during OAuth callback: %w", err)
	}

	// Stop the HTTP server gracefully once we have the code.
	if err := server.Shutdown(context.Background()); err != nil {
		logging.Errorf("Failed to shutdown OAuth callback server gracefully: %v", err)
	}

	// Exchange the authorization code for a token.
	tok, err := config.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange auth code for token: %w", err)
	}

	logging.Infof("OAuth token obtained successfully")
	return tok, nil
}

// saveToken writes the OAuth2 token to a local file.
func saveToken(path string, token *oauth2.Token) error {
	logging.Infof("Saving OAuth token to %s", path)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create token file: %w", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(token); err != nil {
		return fmt.Errorf("failed to encode token to JSON: %w", err)
	}
	return nil
}

func refreshToken(config *oauth2.Config, token *oauth2.Token) (*oauth2.Token, error) {
	ts := config.TokenSource(context.Background(), token)
	return ts.Token()
}

func isInvalidGrant(err error) bool {
	return strings.Contains(err.Error(), "invalid_grant")
}

// IsInvalidGrant exposes invalid_grant detection for API call recovery.
func IsInvalidGrant(err error) bool {
	return isInvalidGrant(err)
}

// RemoveTokenFile deletes the stored OAuth token file if present.
func RemoveTokenFile() error {
	if err := os.Remove(TokenFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
