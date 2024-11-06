package main

import (
	"database/sql"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const prReviewTableSQL = `
CREATE TABLE IF NOT EXISTS pr_reviews (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pr_url TEXT NOT NULL,
    description TEXT,
    channel_id TEXT NOT NULL,
    message_ts TEXT NOT NULL,
    reviewers TEXT NOT NULL,  -- Comma-separated user IDs
    status TEXT DEFAULT 'pending',
	team_id TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    approved_at TIMESTAMP,
    approved_by TEXT
);`

const workspaceTableSQL = `
CREATE TABLE IF NOT EXISTS workspaces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id TEXT NOT NULL UNIQUE,
    team_name TEXT NOT NULL,
    access_token TEXT NOT NULL,
    bot_user_id TEXT NOT NULL,
    installed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`

type PRReview struct {
	ID          int64
	PRUrl       string
	Description string
	ChannelID   string
	// message_ts (timestamp) serves as the unique identifier for a message
	MessageTS  string
	Reviewers  []string
	Status     string
	TeamId     string
	CreatedAt  time.Time
	ApprovedAt sql.NullTime
	ApprovedBy sql.NullString
}

type Workspace struct {
	TeamID      string
	TeamName    string
	AccessToken string
	BotUserID   string
}

func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./pr_reviews.db")
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(prReviewTableSQL); err != nil {
		return nil, err
	}
	if _, err := db.Exec(workspaceTableSQL); err != nil {
		return nil, err
	}

	return db, nil
}

func storePRReview(db *sql.DB, pr *PRReview) error {
	query := `
        INSERT INTO pr_reviews (
            pr_url, description, channel_id, message_ts, team_id, reviewers, status
        ) VALUES (?, ?, ?, ?, ?, ?, ?)`

	reviewers := strings.Join(pr.Reviewers, ",")

	_, err := db.Exec(query,
		pr.PRUrl,
		pr.Description,
		pr.ChannelID,
		pr.MessageTS,
		pr.TeamId,
		reviewers,
		"pending",
	)

	return err
}

func updatePRStatus(db *sql.DB, messageTS string, status string, approvedBy string) error {
	query := `
        UPDATE pr_reviews 
        SET status = ?, 
            approved_at = CASE WHEN ? = 'approved' THEN CURRENT_TIMESTAMP ELSE NULL END,
            approved_by = CASE WHEN ? = 'approved' THEN ? ELSE NULL END
        WHERE message_ts = ?`

	_, err := db.Exec(query, status, status, status, approvedBy, messageTS)
	return err
}

// Add this new database function
func addReviewer(db *sql.DB, messageTS string, reviewerID string) error {
	// First get current reviewers
	var reviewersStr string
	err := db.QueryRow("SELECT reviewers FROM pr_reviews WHERE message_ts = ?", messageTS).Scan(&reviewersStr)
	if err != nil {
		return err
	}

	// Convert to slice
	reviewers := strings.Split(reviewersStr, ",")

	// Check if reviewer already exists
	for _, r := range reviewers {
		if r == reviewerID {
			return nil // Already a reviewer
		}
	}

	// Add new reviewer
	reviewers = append(reviewers, reviewerID)
	newReviewersStr := strings.Join(reviewers, ",")

	// Update database
	_, err = db.Exec("UPDATE pr_reviews SET reviewers = ? WHERE message_ts = ?",
		newReviewersStr, messageTS)
	return err
}

// Add these database functions
func getPendingPRs(db *sql.DB) ([]PRReview, error) {
	query := `
        SELECT id, pr_url, description, channel_id, message_ts, reviewers, team_id, status, created_at 
        FROM pr_reviews 
        WHERE status = 'pending'`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []PRReview
	for rows.Next() {
		var pr PRReview
		var reviewersStr string
		err := rows.Scan(
			&pr.ID,
			&pr.PRUrl,
			&pr.Description,
			&pr.ChannelID,
			&pr.MessageTS,
			&reviewersStr,
			&pr.TeamId,
			&pr.Status,
			&pr.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		pr.Reviewers = strings.Split(reviewersStr, ",")
		prs = append(prs, pr)
	}
	return prs, nil
}

func saveWorkspace(db *sql.DB, ws *Workspace) error {
	query := `
        INSERT INTO workspaces (team_id, team_name, access_token, bot_user_id)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(team_id) DO UPDATE SET
            team_name = excluded.team_name,
            access_token = excluded.access_token,
            bot_user_id = excluded.bot_user_id`

	_, err := db.Exec(query, ws.TeamID, ws.TeamName, ws.AccessToken, ws.BotUserID)
	return err
}

func getWorkspaceToken(db *sql.DB, teamID string) (string, error) {
	var token string
	err := db.QueryRow("SELECT access_token FROM workspaces WHERE team_id = ?", teamID).Scan(&token)
	return token, err
}
