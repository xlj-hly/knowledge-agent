package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	appstorage "go-server/internal/app/storage"
)

type s3Store struct {
	client *s3.Client
	bucket string
}

func newS3(config S3Config) *s3Store {
	client := s3.NewFromConfig(aws.Config{
		Region:                     config.Region,
		Credentials:                credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, ""),
		HTTPClient:                 &http.Client{Timeout: 30 * time.Second},
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}, func(options *s3.Options) {
		if config.Endpoint != "" {
			options.BaseEndpoint = aws.String(config.Endpoint)
		}
		options.UsePathStyle = config.UsePathStyle
		// 端口接受任意 Reader；签名请求头并使用 UNSIGNED-PAYLOAD，避免 SDK 要求 seek。
		options.APIOptions = append(options.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
	})
	return &s3Store{client: client, bucket: config.Bucket}
}

func s3Error(operation string, err error) error {
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		if apiError.ErrorCode() == "NoSuchKey" || operation == "stat" && apiError.ErrorCode() == "NotFound" {
			return appstorage.ErrNotFound
		}
	}
	return failed("s3", operation, err)
}

func (s *s3Store) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if err := checkRequest(ctx, key); err != nil {
		return err
	}
	if body == nil || size < 0 {
		return errors.New("invalid object body or size")
	}
	contentType, err := contentTypeOrDefault(contentType)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: &contextReader{ctx: ctx, reader: body}, ContentLength: aws.Int64(size), ContentType: aws.String(contentType)})
	if err != nil {
		return s3Error("put", err)
	}
	return nil
}

func (s *s3Store) Get(ctx context.Context, key string) (appstorage.Object, error) {
	if err := checkRequest(ctx, key); err != nil {
		return appstorage.Object{}, err
	}
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return appstorage.Object{}, s3Error("get", err)
	}
	return appstorage.Object{Body: &contextBody{contextReader: contextReader{ctx: ctx, reader: output.Body}, closer: output.Body}, Metadata: appstorage.Metadata{Size: aws.ToInt64(output.ContentLength), ContentType: aws.ToString(output.ContentType), ModifiedAt: aws.ToTime(output.LastModified)}}, nil
}

func (s *s3Store) Stat(ctx context.Context, key string) (appstorage.Metadata, error) {
	if err := checkRequest(ctx, key); err != nil {
		return appstorage.Metadata{}, err
	}
	output, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return appstorage.Metadata{}, s3Error("stat", err)
	}
	return appstorage.Metadata{Size: aws.ToInt64(output.ContentLength), ContentType: aws.ToString(output.ContentType), ModifiedAt: aws.ToTime(output.LastModified)}, nil
}

func (s *s3Store) Delete(ctx context.Context, key string) error {
	if err := checkRequest(ctx, key); err != nil {
		return err
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		mapped := s3Error("delete", err)
		if !errors.Is(mapped, appstorage.ErrNotFound) {
			return mapped
		}
	}
	return nil
}
