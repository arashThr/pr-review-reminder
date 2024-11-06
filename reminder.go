package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jasonlvhit/gocron"
	"github.com/slack-go/slack"
)

type ReminderSetup struct {
	scheduler       *gocron.Scheduler
	cronJob         *gocron.Job
	startReminder   time.Duration
	channelReminder time.Duration
}

func createProdSetup() *ReminderSetup {
	scheduler := gocron.NewScheduler()
	return &ReminderSetup{
		scheduler:       scheduler,
		cronJob:         scheduler.Every(1).Day().At("09:00"),
		startReminder:   24 * time.Hour,
		channelReminder: 3 * 24 * time.Hour,
	}
}

func createTestSetup() *ReminderSetup {
	scheduler := gocron.NewScheduler()
	return &ReminderSetup{
		scheduler:       scheduler,
		cronJob:         scheduler.Every(7).Seconds(),
		startReminder:   10 * time.Second,
		channelReminder: 20 * time.Second,
	}
}

// Add the reminder system
func startReminderSystem(db *sql.DB) {
	isProd := os.Getenv("GO_ENV") == "prod"
	var reminderSetup *ReminderSetup
	if isProd {
		log.Printf("Running in production mode")
		reminderSetup = createProdSetup()
	} else {
		log.Printf("Running in test mode")
		reminderSetup = createTestSetup()
	}

	reminderSetup.cronJob.Do(func() {
		log.Printf("Running reminder system")
		prs, err := getPendingPRs(db)
		if err != nil {
			log.Printf("Error getting pending PRs: %v", err)
			return
		}

		for _, pr := range prs {
			sinceCreation := time.Since(pr.CreatedAt)
			if sinceCreation < reminderSetup.startReminder {
				continue
			}

			api, err := getApi(db, pr.TeamId)
			if err != nil {
				log.Printf("Error getting API in reminder system: %v", err)
				return
			}

			var mentionUsers []string

			if sinceCreation >= reminderSetup.channelReminder {
				mentionUsers = []string{"<!channel>"}
			} else {
				// Get users who reacted with eyes
				reactions, err := api.GetReactions(slack.ItemRef{
					Channel:   pr.ChannelID,
					Timestamp: pr.MessageTS,
				}, slack.NewGetReactionsParameters())

				if err != nil {
					log.Printf("Error getting reactions: %v", err)
					continue
				}

				// Look for eyes reactions
				for _, reaction := range reactions {
					if reaction.Name == "eyes" {
						for _, user := range reaction.Users {
							mentionUsers = append(mentionUsers, fmt.Sprintf("<@%s>", user))
						}
					}
				}

				// If no eyes reactions, mention original reviewers
				if len(mentionUsers) == 0 {
					for _, reviewer := range pr.Reviewers {
						mentionUsers = append(mentionUsers, fmt.Sprintf("<@%s>", reviewer))
					}
				}
			}

			// Create reminder message
			text := fmt.Sprintf("🔔 *Reminder:* PR needs review\n<%s|Open PR>\n", pr.PRUrl)
			if len(mentionUsers) > 0 {
				text += "Hey " + strings.Join(mentionUsers, ", ") + "! "
				if sinceCreation >= reminderSetup.channelReminder {
					text += "This PR has been waiting for review for 3+ days."
				} else {
					text += "This PR is awaiting your review."
				}
			}

			// Post reminder as thread reply
			_, _, err = api.PostMessage(
				pr.ChannelID,
				slack.MsgOptionText(text, false),
				slack.MsgOptionTS(pr.MessageTS),
			)
			if err != nil {
				log.Printf("Error posting reminder: %v", err)
			}
		}
	})

	reminderSetup.scheduler.Start()
}
