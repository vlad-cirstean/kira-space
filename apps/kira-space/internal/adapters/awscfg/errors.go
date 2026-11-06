package awscfg

import (
	"context"
	"errors"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	smithy "github.com/aws/smithy-go"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// authErrorCodes is the union of sqs/errors.ts's and s3/errors.ts's own E_AUTH lists, plus
// S3's InvalidAccessKeyId — the widening is deliberate and recorded (SigV4 rejects an unknown
// access key identically for both services; the TypeScript split was an oversight, not a design
// choice).
var authErrorCodes = map[string]bool{
	"UnrecognizedClientException": true,
	"InvalidClientTokenId":        true,
	"AccessDenied":                true,
	"SignatureDoesNotMatch":       true,
	"InvalidAccessKeyId":          true,
}

// MapError is the shared table both sqs/errors.go and s3/errors.go wrap in a three-line function.
// Dispatch order ports errors.ts's own verbatim: cancellation first, then auth, then
// timeout, then connect, then E_QUERY as the default. A missing queue/bucket/object falls through
// to E_QUERY deliberately, not E_NOT_FOUND — an ordinary query-time condition against a connection
// that is still perfectly live (sqs/errors.ts's and s3/errors.ts's own comment).
func MapError(err error) *adapters.Error {
	if err == nil {
		return nil
	}
	var ae *adapters.Error
	if errors.As(err, &ae) {
		return ae
	}
	message := err.Error()

	// AWS-1(e)/AWS-3(e) confirmed: the SDK wraps a cancelled context through
	// *smithy.OperationError with %w, so errors.Is still finds context.Canceled underneath.
	if errors.Is(err, context.Canceled) {
		return adapters.New(adapters.CodeCancelled, message, err)
	}

	var profileErr awsconfig.SharedConfigProfileNotExistError
	if errors.As(err, &profileErr) {
		return adapters.New(adapters.CodeAuth, message, err)
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if authErrorCodes[apiErr.ErrorCode()] {
			return adapters.New(adapters.CodeAuth, message, err)
		}
		if apiErr.ErrorCode() == "RequestTimeout" {
			return adapters.New(adapters.CodeTimeout, message, err)
		}
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return adapters.New(adapters.CodeTimeout, message, err)
	}

	if ne := adapters.ClassifyNetError(err); ne.DNS || ne.OpError != nil {
		return adapters.New(adapters.CodeConnect, message, err)
	}

	return adapters.New(adapters.CodeQuery, message, err)
}
