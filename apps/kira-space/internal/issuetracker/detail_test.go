package issuetracker

import (
	"fmt"
	"testing"
)

func TestNormalize(t *testing.T) {
	t.Run("sorts, caps to newest, flags truncation", func(t *testing.T) {
		d := IssueDetail{CommentsTotal: 3}
		for i := 0; i < DetailMaxComments+5; i++ {
			d.Comments = append(d.Comments, Comment{ID: fmt.Sprint(1000 - i), CreatedAt: int64(DetailMaxComments + 5 - i)})
		}
		d.Normalize()
		if len(d.Comments) != DetailMaxComments || !d.CommentsTruncated {
			t.Fatalf("len=%d truncated=%v", len(d.Comments), d.CommentsTruncated)
		}
		if d.Comments[0].CreatedAt != 6 || d.Comments[len(d.Comments)-1].CreatedAt != DetailMaxComments+5 {
			t.Fatalf("not the newest oldest-first: %d..%d", d.Comments[0].CreatedAt, d.Comments[len(d.Comments)-1].CreatedAt)
		}
	})
	t.Run("ties break on id", func(t *testing.T) {
		d := IssueDetail{Comments: []Comment{{ID: "b", CreatedAt: 1}, {ID: "a", CreatedAt: 1}}}
		d.Normalize()
		if d.Comments[0].ID != "a" {
			t.Fatal("tie not broken by id")
		}
	})
	t.Run("total above shown flags truncation", func(t *testing.T) {
		d := IssueDetail{CommentsTotal: 5, Comments: []Comment{{ID: "1"}}}
		d.Normalize()
		if !d.CommentsTruncated {
			t.Fatal("not truncated")
		}
	})
	t.Run("url scheme and nil slices", func(t *testing.T) {
		d := IssueDetail{Comments: []Comment{{ID: "1", URL: "javascript:x"}, {ID: "2", URL: "http://x"}, {ID: "3", URL: "https://x"}}}
		d.Normalize()
		if d.Comments[0].URL != "" || d.Comments[1].URL != "" || d.Comments[2].URL != "https://x" {
			t.Fatalf("urls: %+v", d.Comments)
		}
		if d.Labels == nil || d.Components == nil {
			t.Fatal("nil slices")
		}
	})
	t.Run("labels capped", func(t *testing.T) {
		d := IssueDetail{Labels: make([]string, DetailMaxLabels+3)}
		d.Normalize()
		if len(d.Labels) != DetailMaxLabels {
			t.Fatal(len(d.Labels))
		}
	})
}
