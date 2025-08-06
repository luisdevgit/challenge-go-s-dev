package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var (
	s3Client *s3.Client
	bucket   string
)

func init() {
	initS3Client()
}

// initS3Client initializes the S3 client and loads the target bucket name from environment variables.
// It terminates execution if configuration is missing or AWS setup fails.
func initS3Client() {
	bucket = os.Getenv("S3_BUCKET")
	if bucket == "" {
		log.Fatal("S3_BUCKET is not defined in the environment")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(os.Getenv("AWS_REGION")))
	if err != nil {
		log.Fatalf("Error loading AWS configuration: %v", err)
	}

	s3Client = s3.NewFromConfig(cfg)
}

// handler is the main Lambda handler.
// It accepts only POST requests, decodes the CSV file from the request,
// uploads it to S3, and returns an appropriate HTTP response.
func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	if req.RequestContext.HTTP.Method != http.MethodPost {
		return methodNotAllowedResponse(), nil
	}

	body, err := decodeRequestBody(req)
	if err != nil {
		return badRequestResponse("Failed to decode request body"), nil
	}

	filename := generateFilename()
	if err := uploadToS3(ctx, filename, body); err != nil {
		return internalServerErrorResponse(fmt.Sprintf("Failed to upload to S3: %v", err)), nil
	}

	log.Printf("File %s uploaded successfully to bucket %s", filename, bucket)
	return successResponse(fmt.Sprintf("File successfully uploaded as %s", filename)), nil
}

// decodeRequestBody decodes the HTTP request body.
// If it's base64 encoded, it decodes it. Otherwise, it returns the raw body.
func decodeRequestBody(req events.APIGatewayV2HTTPRequest) ([]byte, error) {
	if req.IsBase64Encoded {
		return base64.StdEncoding.DecodeString(req.Body)
	}
	return []byte(req.Body), nil
}

// generateFilename returns a unique filename using the current Unix timestamp.
func generateFilename() string {
	return fmt.Sprintf("upload-%d.csv", time.Now().Unix())
}

// uploadToS3 uploads the provided byte content to S3 with the specified key.
func uploadToS3(ctx context.Context, key string, body []byte) error {
	_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	})
	return err
}

// methodNotAllowedResponse returns a 405 HTTP response when the method is not POST.
func methodNotAllowedResponse() events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusMethodNotAllowed,
		Body:       "Only POST method is allowed",
	}
}

// badRequestResponse returns a 400 HTTP response with a custom error message.
func badRequestResponse(msg string) events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusBadRequest,
		Body:       msg,
	}
}

// internalServerErrorResponse returns a 500 HTTP response with a custom error message.
func internalServerErrorResponse(msg string) events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusInternalServerError,
		Body:       msg,
	}
}

// successResponse returns a 200 HTTP response with a success message.
func successResponse(msg string) events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Body:       msg,
	}
}

// main starts the Lambda function.
func main() {
	lambda.Start(handler)
}
