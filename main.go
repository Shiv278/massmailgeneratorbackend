package main

import (
	"bufio"
	"database/sql"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/jordan-wright/email"
	"github.com/lib/pq"
	"net/smtp"
)

var db *sql.DB

// Initialize database connection
func initDB() {
	var err error
	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
	}
}

// Function to validate email
func isValidEmail(email string) bool {
	const emailRegex = `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	match, err := regexp.MatchString(emailRegex, email)
	if err != nil {
		return false
	}
	return match
}

// Save scheduled email to the database
func saveScheduledEmail(recipients []string, subject, body string, scheduledAt time.Time) error {
	recipientsStr := "{" + strings.Join(recipients, ",") + "}"

	query := `INSERT INTO scheduled_emails (recipients, subject, body, schedule_at, status) VALUES ($1, $2, $3, $4, 'pending')`
	_, err := db.Exec(query, recipientsStr, subject, body, scheduledAt)
	return err
}

// Send email function
func sendEmail(recipients []string, subject, body string) error {
	senderEmail := os.Getenv("SENDER_EMAIL")
	senderName := os.Getenv("SENDER_NAME")
	e := email.NewEmail()
	e.From = senderName + " <" + senderEmail + ">"
	e.To = recipients
	e.Subject = subject
	e.HTML = []byte(body)

	return e.Send("smtp.gmail.com:587", smtp.PlainAuth("", senderEmail, os.Getenv("SENDER_PASSWORD"), "smtp.gmail.com"))
}

// Cron job to send scheduled emails
func runEmailScheduler() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		query := `SELECT id, recipients, subject, body FROM scheduled_emails WHERE schedule_at <= $1 AND status = 'pending'`
		rows, err := db.Query(query, now)
		if err != nil {
			log.Println("Error fetching scheduled emails:", err)
			continue
		}

		for rows.Next() {
			var id int
			var recipients pq.StringArray
			var subject, body string
			if err := rows.Scan(&id, &recipients, &subject, &body); err != nil {
				log.Println("Error scanning email record:", err)
				continue
			}

			if err := sendEmail(recipients, subject, body); err != nil {
				log.Printf("Failed to send email ID %d: %v", id, err)
				_, _ = db.Exec(`UPDATE scheduled_emails SET status = 'failed' WHERE id = $1`, id)
			} else {
				_, _ = db.Exec(`UPDATE scheduled_emails SET status = 'sent' WHERE id = $1`, id)
			}
		}
		rows.Close()
	}
}

// Email handler
func sendEmailHandler(c *gin.Context) {
	subject := c.PostForm("subject")
	body := c.PostForm("body")
	scheduledTime := c.PostForm("scheduled_time")

	var scheduleTimeParsed time.Time
	var err error
	if scheduledTime != "" {
		scheduleTimeParsed, err = time.Parse(time.RFC3339, scheduledTime)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid scheduled_time format. Use RFC3339 format."})
			return
		}
	}

	file, _, err := c.Request.FormFile("file")
	var validEmails []string
	var invalidEmails []string

	if err == nil && file != nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			email := strings.TrimSpace(scanner.Text())
			if email != "" {
				if isValidEmail(email) {
					validEmails = append(validEmails, email)
				} else {
					invalidEmails = append(invalidEmails, email)
				}
			}
		}
		if scanner.Err() != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse uploaded file"})
			return
		}
	} else {
		manualEmails := c.PostForm("emails")
		rawEmails := strings.FieldsFunc(manualEmails, func(r rune) bool {
			return r == ',' || r == '\n'
		})
		for _, email := range rawEmails {
			email = strings.TrimSpace(email)
			if email != "" {
				if isValidEmail(email) {
					validEmails = append(validEmails, email)
				} else {
					invalidEmails = append(invalidEmails, email)
				}
			}
		}
	}

	if len(validEmails) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message":        "No valid emails to send to, but request was received.",
			"valid_emails":   validEmails,
			"invalid_emails": invalidEmails,
		})
		return
	}

	if !scheduleTimeParsed.IsZero() {
		if err := saveScheduledEmail(validEmails, subject, body, scheduleTimeParsed); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to schedule email"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Email scheduled successfully", "valid_emails": validEmails, "invalid_emails": invalidEmails})
		return
	}

	if err := sendEmail(validEmails, subject, body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Emails sent successfully",
		"valid_emails":   validEmails,
		"invalid_emails": invalidEmails,
	})
}

func main() {
	if err := godotenv.Load("configs/.env"); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	initDB()
	defer db.Close()

	go runEmailScheduler()

	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://massmailgeneratorfrontend.onrender.com"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: true,
	}))

	r.POST("/send-email", sendEmailHandler)

	r.Run(":8080")
}
