package testsupport

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// TestLocalStackProxyCounts is M8.1's own acceptance check: start a container, proxy one
// GetQueueUrl through the counting proxy, and confirm the counter reads 1.
func TestLocalStackProxyCounts(t *testing.T) {
	if !IsDockerAvailable() {
		t.Skip(DockerUnavailableMessage)
	}
	ctx := context.Background()
	c, endpoint, err := startLocalStack(ctx, "sqs")
	if err != nil {
		t.Fatalf("startLocalStack: %v", err)
	}
	defer c.Terminate(ctx)

	proxy, err := NewOperationCountingProxy(endpoint, "AmazonSQS.GetQueueUrl")
	if err != nil {
		t.Fatalf("NewOperationCountingProxy: %v", err)
	}
	defer proxy.Close()

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(LocalStackRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(LocalStackStaticAccessKey, LocalStackStaticSecret, "")),
	)
	if err != nil {
		t.Fatalf("LoadDefaultConfig: %v", err)
	}
	client := sqs.NewFromConfig(cfg, func(o *sqs.Options) { o.BaseEndpoint = aws.String(proxy.Endpoint) })

	qOut, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("proxy-test-queue")})
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	if proxy.Count() != 0 {
		t.Fatalf("proxy.Count() = %d before any GetQueueUrl, want 0", proxy.Count())
	}

	if _, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{QueueName: aws.String("proxy-test-queue")}); err != nil {
		t.Fatalf("GetQueueUrl: %v", err)
	}
	if got := proxy.Count(); got != 1 {
		t.Fatalf("proxy.Count() = %d after one GetQueueUrl, want 1", got)
	}

	// CreateQueue again — a different operation must not move the counter.
	if _, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("proxy-test-queue-2")}); err != nil {
		t.Fatalf("CreateQueue 2: %v", err)
	}
	if got := proxy.Count(); got != 1 {
		t.Fatalf("proxy.Count() after an unrelated op = %d, want still 1", got)
	}

	_ = qOut
}
