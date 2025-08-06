package main

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
)

// MonthlySummary represents a summary of transactions for a given month
type MonthlySummary struct {
	Month            string  `json:"month"`
	TransactionCount int     `json:"transaction_count"`
	AverageCredit    float64 `json:"average_credit"`
	AverageDebit     float64 `json:"average_debit"`
}

// AccountSummary represents the total and monthly transaction summary for a user
type AccountSummary struct {
	Email            string           `json:"email"`
	TotalBalance     float64          `json:"total_balance"`
	MonthlySummaries []MonthlySummary `json:"monthly_summaries"`
}

// Event is the structure expected as input to the Lambda
type Event struct {
	Summaries []AccountSummary `json:"summaries"`
}

var sesClient *ses.Client

// Initialize AWS SES client with region
func init() {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-east-1"))
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	sesClient = ses.NewFromConfig(cfg)
}

// Format a float with 2 decimal places
func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

// Convert integer to string
func itoa(i int) string {
	return strconv.Itoa(i)
}

// Builds the HTML body of the email
func buildHTMLBody(summary AccountSummary) string {
	body := `<html><body>`

	// Add Stori logo (public link)
	body += `<img src="https://www.storicard.com/_next/static/media/storis_savvi_color.7e286ddd.svg" alt="Stori Logo" style="width:150px;margin-bottom:20px;" />`

	// Summary info
	body += `<h1>Transaction Summary</h1>`
	body += `<p><strong>Total Balance:</strong> ` + formatFloat(summary.TotalBalance) + `</p>`

	// Monthly breakdown
	body += `<h2>Monthly Breakdown:</h2><ul>`
	for _, m := range summary.MonthlySummaries {
		body += `<li><strong>` + m.Month + `</strong>: `
		body += itoa(m.TransactionCount) + ` transactions, `
		body += `Average credit amount: ` + formatFloat(m.AverageCredit) + `, `
		body += `Average debit amount: ` + formatFloat(m.AverageDebit) + `</li>`
	}
	body += `</ul>`

	body += `</body></html>`
	return body
}

// Main handler function
func handler(ctx context.Context, event Event) (string, error) {
	from := "devsysluis@gmail.com"
	subject := "Your Monthly Transaction Summary"

	// Check if there are any summaries to process
	if len(event.Summaries) == 0 {
		log.Println("No summaries received to send.")
		return "No summaries to send", nil
	}

	// Process each summary and send email
	for _, summary := range event.Summaries {
		body := buildHTMLBody(summary)

		input := &ses.SendEmailInput{
			Source: aws.String(from),
			Destination: &types.Destination{
				ToAddresses: []string{summary.Email},
			},
			Message: &types.Message{
				Subject: &types.Content{
					Data: aws.String(subject),
				},
				Body: &types.Body{
					Html: &types.Content{
						Data: aws.String(body),
					},
				},
			},
		}

		// Attempt to send email via SES
		_, err := sesClient.SendEmail(ctx, input)
		if err != nil {
			log.Printf("Failed to send email to %s: %v", summary.Email, err)
			continue
		}
		log.Printf("Email successfully sent to %s", summary.Email)
	}

	return "Emails sent", nil
}

func main() {
	lambda.Start(handler)
}
