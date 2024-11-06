package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"

	"github.com/slack-go/slack"
)

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func generateState() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func handleOAuth(db *sql.DB, config OAuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Handle installation completion
		if code := r.URL.Query().Get("code"); code != "" {
			httpClient := &http.Client{}

			// Exchange code for token
			oauthResp, err := slack.GetOAuthV2Response(
				httpClient,
				config.ClientID,
				config.ClientSecret,
				code,
				config.RedirectURL,
			)
			if err != nil {
				http.Error(w, "Failed to complete OAuth flow", http.StatusInternalServerError)
				log.Printf("OAuth error: %v", err)
				return
			}

			// Save workspace info
			workspace := &Workspace{
				TeamID:      oauthResp.Team.ID,
				TeamName:    oauthResp.Team.Name,
				AccessToken: oauthResp.AccessToken,
				BotUserID:   oauthResp.BotUserID,
			}

			if err := saveWorkspace(db, workspace); err != nil {
				http.Error(w, "Failed to save workspace", http.StatusInternalServerError)
				log.Printf("Database error: %v", err)
				return
			}

			// Show success page
			w.Write([]byte("Installation successful! You can close this window."))
			return
		}

		// Start installation
		state, err := generateState()
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		// TODO
		// Store state in session/db if needed

		// Redirect to Slack OAuth page
		authURL := fmt.Sprintf(
			"https://slack.com/oauth/v2/authorize?client_id=%s&scope=commands,chat:write,reactions:read&redirect_uri=%s&state=%s",
			config.ClientID,
			config.RedirectURL,
			state,
		)
		http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
	}
}
