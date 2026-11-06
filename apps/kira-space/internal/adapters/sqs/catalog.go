package sqs

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

const pageLimit = 1000 // ListQueues's own max MaxResults per call

// queueListing is catalog.ts's QueueListing.
type queueListing struct {
	nodes     []model.TreeNode
	urlByName map[string]string
}

// listQueues is catalog.ts's listQueues. The tree is a flat queue list, no deeper level; name is
// the URL's last path segment. ListQueues already has every queue's full URL in hand while paging,
// so it hands the name->URL map back instead of discarding it — adapter.go caches it, avoiding a
// GetQueueUrl round trip on every read()/count() call.
func listQueues(ctx context.Context, client *sqs.Client) (queueListing, error) {
	nodes := []model.TreeNode{}
	urlByName := map[string]string{}
	var nextToken *string
	for {
		result, err := client.ListQueues(ctx, &sqs.ListQueuesInput{MaxResults: aws.Int32(pageLimit), NextToken: nextToken})
		if err != nil {
			return queueListing{}, mapError(err)
		}
		for _, url := range result.QueueUrls {
			name := url
			if i := strings.LastIndexByte(url, '/'); i >= 0 {
				name = url[i+1:]
			}
			urlByName[name] = url
			nodes = append(nodes, model.TreeNode{
				Kind: "queue", Name: name, Path: model.EncodePath([]model.PathSegment{{Kind: "queue", Name: name}}),
				HasChildren: false,
			})
		}
		nextToken = result.NextToken
		if nextToken == nil {
			break
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return queueListing{nodes: nodes, urlByName: urlByName}, nil
}

// resolveQueueURL is catalog.ts's resolveQueueUrl — a queue's leaf name resolved back to its full
// URL via a single cheap GetQueueUrl call.
func resolveQueueURL(ctx context.Context, client *sqs.Client, name string) (string, error) {
	result, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{QueueName: aws.String(name)})
	if err != nil {
		return "", mapError(err)
	}
	if result.QueueUrl == nil {
		return "", mapError(fmt.Errorf("queue not found: %s", name))
	}
	return *result.QueueUrl, nil
}
