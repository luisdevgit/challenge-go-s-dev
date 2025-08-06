# challenge-go-s-dev

This project is a serverless Go-based application for processing financial transactions from CSV files. It allows users to upload transaction data via a simple web interface, stores the data in a PostgreSQL database, summarizes it by user and month, and sends personalized email summaries using AWS Lambda and SES.

---
## 📹 Demo

[▶️ Click here to view the demo video](assets/demo.mp4)


## 📁 Project Structure

```
challenge-go-s-dev/
├── aws/
│   ├── lambda/
│   │   ├── emailer/                # Lambda: Sends email summaries via SES
│   │   ├── summarizer/             # Lambda: Generates summary from DB
│   │   └── uploader/               # Lambda: Parses CSV and stores in DB
│   ├── sql_scripts/
│   │   └── 001_create_cuenta_table.sql  # SQL migration script
│   └── web/
│       └── csv_uploader.html       # HTML form to upload CSV file
├── .gitignore
├── go.mod / go.sum                 # Go dependencies
├── LICENSE
└── README.md
```

---

## ✅ Requirements

- [Go](https://golang.org/dl/) 1.18+
- AWS Account with permissions for:
  - Lambda
  - S3
  - SES
  - RDS (PostgreSQL)
- [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html)
- PostgreSQL instance

---

## 🛠️ Setup

### 1. Clone the repository

```bash
git clone https://github.com/yourusername/challenge-go-s-dev.git
cd challenge-go-s-dev
```

### 2. Set up the PostgreSQL table

Run the migration script to create the `cuenta` table:

```bash
psql -h your-db-host -U your-db-user -d your-db-name -f aws/sql_scripts/001_create_cuenta_table.sql
```

> ⚠️ Replace `your-db-host`, `your-db-user`, and `your-db-name` with your actual PostgreSQL credentials.

---

## ⚙️ Deploy Lambdas

Each Lambda can be deployed individually. Here’s a quick guide:

### Lambda: `uploader`

Triggered by S3 upload. Parses CSV file and stores each row in the DB.

- Input CSV format (required headers):

  ```
  id,date,transaction,email
  ```

- Deploy:

```bash
cd aws/lambda/uploader
GOOS=linux GOARCH=amd64 go build -o main main.go
zip uploader.zip main
# Deploy manually or with AWS CLI / SAM / Terraform
```

### Lambda: `summarizer`

Triggered manually or via schedule. Reads DB and summarizes transactions by user.

- Output: JSON with monthly and total summaries per email.

### Lambda: `emailer`

Takes JSON summary and sends formatted emails using AWS SES.

---

## 🌐 Web Interface

The HTML file `aws/web/csv_uploader.html` allows you to upload a CSV to your S3 bucket via an API Gateway trigger.

1. Open `aws/web/csv_uploader.html` in your browser
2. Select a CSV file with the required format
3. Click "Upload"
4. You will receive a success message if the upload and Lambda execution worked correctly

> 📝 The CSV file **must** contain the following headers: `id,date,transaction,email`

---

## 📧 Email Format Example

Emails are sent with the Stori logo and formatted summaries like:

```
Transaction Summary

Total balance: 200.88

Monthly Summary:
- January: 5 transactions, avg credit: 30.00, avg debit: -15.00
- February: 3 transactions, avg credit: 20.00, avg debit: -10.00
```

---

## 📦 Environment Variables

Each Lambda may require environment variables or secrets (e.g., DB credentials, email sender). You can configure these via AWS Console or use a `.env` loader for local testing.

---

## 🧪 Local Testing

You can test the Lambdas locally using event JSON files:

```bash
sam local invoke -e event.json
```

Or call them directly in Go (with mocks or static data).

---

## 📃 License

MIT License – see [LICENSE](./LICENSE)

---

## ✍️ Author

Luis Alberto Sandoval Hernández  
For Stori technical challenge – 2025
