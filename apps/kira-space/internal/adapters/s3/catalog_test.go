package s3

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// fakePrefixLister ports catalog-listing.spec.ts's fakeS3 (P58f D14): a scripted page per call,
// driving the truncation conjunction without a live server.
type fakePrefixLister struct {
	page   func(call int) *s3.ListObjectsV2Output
	inputs []*s3.ListObjectsV2Input
}

func (f *fakePrefixLister) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	f.inputs = append(f.inputs, in)
	return f.page(len(f.inputs) - 1), nil
}

func TestListPrefixChildren_SplitsCommonPrefixesAndContentsSendsExpectedInput(t *testing.T) {
	fake := &fakePrefixLister{page: func(int) *s3.ListObjectsV2Output {
		return &s3.ListObjectsV2Output{
			CommonPrefixes: []types.CommonPrefix{{Prefix: aws.String("reports/")}, {Prefix: aws.String("assets/")}},
			Contents:       []types.Object{{Key: aws.String("root.txt")}},
		}
	}}
	result, err := listPrefixChildren(context.Background(), fake, "my-bucket", nil, adapters.NewOpCtx("op1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	in := fake.inputs[0]
	if aws.ToString(in.Bucket) != "my-bucket" || aws.ToString(in.Prefix) != "" || aws.ToString(in.Delimiter) != "/" {
		t.Fatalf("unexpected input: %+v", in)
	}
	got := make([][2]string, len(result.Nodes))
	for i, n := range result.Nodes {
		got[i] = [2]string{n.Kind, n.Name}
	}
	want := [][2]string{{"prefix", "assets"}, {"prefix", "reports"}, {"object", "root.txt"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestListPrefixChildren_SkipsExactPrefixDirectoryMarker(t *testing.T) {
	fake := &fakePrefixLister{page: func(int) *s3.ListObjectsV2Output {
		return &s3.ListObjectsV2Output{
			Contents: []types.Object{{Key: aws.String("reports/")}, {Key: aws.String("reports/file.csv")}},
		}
	}}
	result, err := listPrefixChildren(context.Background(), fake, "my-bucket", []string{"reports"}, adapters.NewOpCtx("op1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].Name != "reports/file.csv" {
		t.Fatalf("got %v", result.Nodes)
	}
}

func TestListPrefixChildren_UnclearedTokenRunsMaxRoundsAndTruncates(t *testing.T) {
	fake := &fakePrefixLister{page: func(int) *s3.ListObjectsV2Output {
		return &s3.ListObjectsV2Output{NextContinuationToken: aws.String("more")}
	}}
	result, err := listPrefixChildren(context.Background(), fake, "my-bucket", nil, adapters.NewOpCtx("op1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.inputs) != maxListRounds {
		t.Fatalf("got %d calls, want %d", len(fake.inputs), maxListRounds)
	}
	if result.Truncated == nil || !*result.Truncated {
		t.Fatalf("expected truncated=true, got %v", result.Truncated)
	}
}

func TestListPrefixChildren_CompletesWithinCapReportsNoTruncation(t *testing.T) {
	fake := &fakePrefixLister{page: func(int) *s3.ListObjectsV2Output {
		return &s3.ListObjectsV2Output{Contents: []types.Object{{Key: aws.String("a.txt")}}}
	}}
	result, err := listPrefixChildren(context.Background(), fake, "my-bucket", nil, adapters.NewOpCtx("op1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.inputs) != 1 {
		t.Fatalf("got %d calls, want 1", len(fake.inputs))
	}
	if result.Truncated != nil {
		t.Fatalf("expected no truncation flag, got %v", *result.Truncated)
	}
}
