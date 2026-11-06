package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/awscfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// connect is client.ts's connectS3. UsePathStyle is turned on automatically whenever an endpoint
// override is present (P58d D5) — a non-AWS S3-compatible endpoint almost always needs path-style
// addressing, and it has no effect against real AWS S3 when no override is set.
func connect(ctx context.Context, cfg model.ResolvedConnectionConfig, log func(level, message string)) (*s3.Client, error) {
	resolved, err := awscfg.Resolve(ctx, cfg, "s3", log)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(resolved.AWS, func(o *s3.Options) {
		if resolved.BaseEndpoint != "" {
			o.BaseEndpoint = aws.String(resolved.BaseEndpoint)
			o.UsePathStyle = true
		}
	}), nil
}
