package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/smithy-go"
)

func TestS3FailureCodeAllowsOnlyDiagnosticCategories(t *testing.T) {
	privateDetail := "private-bucket/private-key https://private-endpoint test-access-key test-secret-key"
	for _, fixture := range []struct{ code, want string }{
		{code: "AccessDenied", want: "AccessDenied"},
		{code: "SignatureDoesNotMatch", want: "SignatureDoesNotMatch"},
		{code: "InvalidAccessKeyId", want: "InvalidAccessKeyId"},
		{code: "NoSuchBucket", want: "NoSuchBucket"},
		{code: "RequestTimeout", want: "RequestTimeout"},
		{code: "SlowDown", want: "SlowDown"},
		{code: "ServiceUnavailable", want: "ServiceUnavailable"},
		{code: "InternalError", want: "InternalError"},
		{code: "vendor_error_" + privateDetail, want: "upstream_error"},
		{code: "AccessDenied_" + privateDetail, want: "upstream_error"},
		{code: "", want: "upstream_error"},
	} {
		t.Run(fixture.want+"/"+fixture.code[:min(len(fixture.code), 20)], func(t *testing.T) {
			upstream := &smithy.GenericAPIError{Code: fixture.code, Message: privateDetail}
			err := s3Error("get", fmt.Errorf("%s: %w", privateDetail, upstream))
			var diagnostic interface{ FailureCode() string }
			if !errors.As(err, &diagnostic) {
				t.Fatal("storage failure lacks diagnostic category")
			}
			if actual := diagnostic.FailureCode(); actual != fixture.want {
				t.Fatalf("incorrect diagnostic category: %q", actual)
			}
			var preserved smithy.APIError
			if !errors.As(err, &preserved) || preserved != upstream {
				t.Fatal("diagnostic classification discarded original error chain")
			}
			if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "test-access-key") || strings.Contains(err.Error(), "test-secret-key") {
				t.Fatal("storage error text exposed private details")
			}
		})
	}
}

func TestFailureCodePreservesContextAndDoesNotTrustOtherProviders(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		err := failed("s3", "get", fmt.Errorf("private-endpoint: %w", cause))
		var diagnostic interface{ FailureCode() string }
		if !errors.As(err, &diagnostic) || diagnostic.FailureCode() != "upstream_error" || !errors.Is(err, cause) {
			t.Fatalf("context error chain changed: %v", err)
		}
	}
	err := failed("filesystem", "stat", &smithy.GenericAPIError{Code: "AccessDenied", Message: "private-path"})
	var diagnostic interface{ FailureCode() string }
	if !errors.As(err, &diagnostic) || diagnostic.FailureCode() != "upstream_error" {
		t.Fatalf("trusted S3 category from non-S3 provider: %v", err)
	}
}
