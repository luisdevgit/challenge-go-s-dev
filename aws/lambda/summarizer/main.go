package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	awslambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/lib/pq"
)

var (
	s3Client     *s3.Client
	lambdaClient *awslambda.Client

	db     *sql.DB
	dbOnce sync.Once
)

// initAWSClients initializes AWS SDK clients for S3 and Lambda.
func initAWSClients() {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("Error loading AWS config: %v", err)
	}
	s3Client = s3.NewFromConfig(cfg)
	lambdaClient = awslambda.NewFromConfig(cfg)
}

// getDBConnection initializes and returns a DB connection pool singleton.
func getDBConnection() (*sql.DB, error) {
	var err error
	dbOnce.Do(func() {
		host := os.Getenv("DB_HOST")
		port := os.Getenv("DB_PORT")
		user := os.Getenv("DB_USER")
		password := os.Getenv("DB_PASSWORD")
		dbname := os.Getenv("DB_NAME")

		connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
			host, port, user, password, dbname)

		db, err = sql.Open("postgres", connStr)
		if err != nil {
			return
		}
		err = db.Ping()
	})
	return db, err
}

// insertTransactions inserts multiple transaction records inside a transaction block.
// Returns a set of unique emails found in the transactions.
func insertTransactions(tx *sql.Tx, transactions [][]string) (map[string]struct{}, error) {
	const expectedColumns = 4
	stmt, err := tx.Prepare(`INSERT INTO transacciones (external_id, date, transaction, email) VALUES ($1, $2, $3, $4)`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	emailSet := make(map[string]struct{})

	for i, row := range transactions {
		if len(row) != expectedColumns {
			return nil, fmt.Errorf("invalid column count in row %d: expected %d, got %d", i+1, expectedColumns, len(row))
		}

		externalID, err := strconv.Atoi(row[0])
		if err != nil {
			return nil, fmt.Errorf("invalid externalID in row %d: %w", i+1, err)
		}
		date := row[1]
		transaction := row[2]
		email := row[3]

		if _, err := stmt.Exec(externalID, date, transaction, email); err != nil {
			return nil, fmt.Errorf("insert failed at row %d: %w", i+1, err)
		}

		emailSet[email] = struct{}{}
	}

	return emailSet, nil
}

// processCSVFile downloads the CSV from S3, reads and validates it, returns rows as [][]string.
func processCSVFile(ctx context.Context, bucket, key string) ([][]string, error) {
	log.Printf("Starting to process file s3://%s/%s", bucket, key)

	obj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("error getting S3 object: %w", err)
	}
	defer obj.Body.Close()

	reader := csv.NewReader(obj.Body)
	reader.Comma = ','
	reader.TrimLeadingSpace = true

	// Read and discard header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("error reading CSV header: %w", err)
	}
	if len(header) != 4 {
		return nil, fmt.Errorf("invalid CSV header column count: expected 4, got %d", len(header))
	}

	var rows [][]string
	lineNum := 1
	for {
		lineNum++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Warning: error reading CSV line %d: %v", lineNum, err)
			continue
		}
		if len(record) != 4 {
			log.Printf("Warning: invalid column count in line %d: expected 4, got %d", lineNum, len(record))
			continue
		}
		rows = append(rows, record)
	}

	log.Printf("CSV file processing complete: %d valid rows found", len(rows))
	return rows, nil
}

// MonthlySummary represents a summary of transactions for a specific month.
type MonthlySummary struct {
	Month            string  `json:"month"`
	TransactionCount int     `json:"transaction_count"`
	AverageCredit    float64 `json:"average_credit"`
	AverageDebit     float64 `json:"average_debit"`
}

// AccountSummary represents a summary of transactions for an account.
type AccountSummary struct {
	Email            string           `json:"email"`
	TotalBalance     float64          `json:"total_balance"`
	MonthlySummaries []MonthlySummary `json:"monthly_summaries"`
}

// Event represents the input event structure for the Lambda function.
func getTransactionSummaryByEmail(db *sql.DB, email string) (*AccountSummary, error) {
	query := `
		SELECT 
			TO_CHAR(date, 'FMMonth') AS month,
			COUNT(*) AS num_transactions,
			AVG(CASE 
					WHEN TRIM(transaction) LIKE '+%' 
					THEN CAST(REPLACE(TRIM(transaction), '+', '') AS NUMERIC) 
					ELSE NULL 
				END) AS avg_credit,
			AVG(CASE 
					WHEN TRIM(transaction) LIKE '-%' 
					THEN CAST(REPLACE(TRIM(transaction), '-', '') AS NUMERIC) 
					ELSE NULL 
				END) AS avg_debit,
			SUM(CAST(TRIM(transaction) AS NUMERIC)) AS balance
		FROM transacciones
		WHERE email = $1
		GROUP BY DATE_TRUNC('month', date), TO_CHAR(date, 'FMMonth')
		ORDER BY DATE_TRUNC('month', date);
	`

	rows, err := db.Query(query, email)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var summary AccountSummary
	summary.Email = email
	var totalBalance float64

	for rows.Next() {
		var m MonthlySummary
		var month string
		var avgCredit, avgDebit, balance sql.NullFloat64

		err := rows.Scan(&month, &m.TransactionCount, &avgCredit, &avgDebit, &balance)
		if err != nil {
			return nil, fmt.Errorf("failed scanning row: %w", err)
		}

		m.Month = month
		if avgCredit.Valid {
			m.AverageCredit = avgCredit.Float64
		}
		if avgDebit.Valid {
			m.AverageDebit = -avgDebit.Float64 // debit is negative
		}
		if balance.Valid {
			totalBalance += balance.Float64
		}

		summary.MonthlySummaries = append(summary.MonthlySummaries, m)
	}
	summary.TotalBalance = totalBalance

	return &summary, nil
}

// invokeNotificationLambda asynchronously invokes the notification Lambda function.
func invokeNotificationLambda(ctx context.Context, summaries []*AccountSummary) error {
	payload := map[string]interface{}{
		"summaries": summaries,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error serializing payload: %w", err)
	}

	output, err := lambdaClient.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName:   aws.String("pongo_mail"),
		Payload:        jsonPayload,
		InvocationType: awslambdaTypes.InvocationTypeEvent, // async
	})
	if err != nil {
		return fmt.Errorf("error invoking pongo_mail Lambda: %w", err)
	}

	log.Printf("Lambda pongo_mail invoked, status: %d", output.StatusCode)
	return nil
}

// handler is the main Lambda handler triggered by S3 events.
func handler(ctx context.Context, s3Event events.S3Event) error {
	log.Println("Lambda started processing S3 event")

	db, err := getDBConnection()
	if err != nil {
		log.Printf("Error getting DB connection: %v", err)
		return err
	}

	var summaries []*AccountSummary
	for _, record := range s3Event.Records {
		bucket := record.S3.Bucket.Name
		key := record.S3.Object.Key

		// Process CSV and get valid rows
		rows, err := processCSVFile(ctx, bucket, key)
		if err != nil {
			log.Printf("Error processing CSV file: %v", err)
			return err
		}

		// Begin transaction
		tx, err := db.Begin()
		if err != nil {
			log.Printf("Failed to begin DB transaction: %v", err)
			return err
		}

		// Insert all rows atomically
		emailSet, err := insertTransactions(tx, rows)
		if err != nil {
			tx.Rollback()
			log.Printf("Transaction rollback due to error: %v", err)
			return err
		}

		if err := tx.Commit(); err != nil {
			log.Printf("Failed to commit DB transaction: %v", err)
			return err
		}

		log.Printf("Successfully inserted %d rows from file s3://%s/%s", len(rows), bucket, key)

		for email := range emailSet {
			summary, err := getTransactionSummaryByEmail(db, email)
			if err != nil {
				log.Printf("Error generating summary for %s: %v", email, err)
				continue
			}

			summaries = append(summaries, summary)
		}
	}

	if err := invokeNotificationLambda(ctx, summaries); err != nil {
		log.Printf("Error invoking notification Lambda: %v", err)
		return err
	}

	log.Println("Lambda finished processing S3 event successfully")
	return nil
}

func main() {
	initAWSClients()
	lambda.Start(handler)
}
