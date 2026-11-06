package sqs

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/awscfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// connect is client.ts's connectSqs. No path-style analogue — that is S3's alone.
func connect(ctx context.Context, cfg model.ResolvedConnectionConfig, log func(level, message string)) (*sqs.Client, error) {
	resolved, err := awscfg.Resolve(ctx, cfg, "sqs", log)
	if err != nil {
		return nil, err
	}
	return sqs.NewFromConfig(resolved.AWS, func(o *sqs.Options) {
		if resolved.BaseEndpoint != "" {
			o.BaseEndpoint = aws.String(resolved.BaseEndpoint)
		}
	}), nil
}
