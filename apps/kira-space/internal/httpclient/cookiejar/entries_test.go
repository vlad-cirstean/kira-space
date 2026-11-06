package cookiejar

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestEntriesReportsDomainAndPathPerCookie(t *testing.T) {
	j, _ := New(nil)
	u := mustURL(t, "http://example.com/a/b")
	j.SetCookies(u, []*http.Cookie{{Name: "sid", Value: "root", Path: "/"}, {Name: "sid", Value: "deep", Path: "/a"}})
	got := j.Entries(u)
	if len(got) != 2 || got[0].Path != "/a" || got[0].Value != "deep" || got[1].Path != "/" || got[0].Domain != "example.com" {
		t.Fatalf("entries = %+v", got)
	}
}

func TestDeleteRemovesOnlyTheExactEntry(t *testing.T) {
	j, _ := New(nil)
	u := mustURL(t, "http://example.com/a/b")
	j.SetCookies(u, []*http.Cookie{{Name: "sid", Value: "root", Path: "/"}, {Name: "sid", Value: "deep", Path: "/a"}})
	if !j.Delete("example.com", "/a", "sid") {
		t.Fatal("delete reported a miss")
	}
	got := j.Entries(u)
	if len(got) != 1 || got[0].Path != "/" {
		t.Fatalf("entries = %+v", got)
	}
	if j.Delete("example.com", "/a", "sid") {
		t.Fatal("second delete reported a hit")
	}
}

func TestDeleteParentDomainCookieSeenFromSubdomain(t *testing.T) {
	j, _ := New(nil)
	j.SetCookies(mustURL(t, "http://example.com/"), []*http.Cookie{{Name: "t", Value: "1", Domain: "example.com", Path: "/"}})
	sub := mustURL(t, "http://sub.example.com/")
	got := j.Entries(sub)
	if len(got) != 1 || got[0].Domain != "example.com" {
		t.Fatalf("entries = %+v", got)
	}
	if !j.Delete(got[0].Domain, got[0].Path, got[0].Name) || len(j.Entries(sub)) != 0 {
		t.Fatal("cookie survived delete")
	}
}

func TestEntriesLeavesLastAccessAndExpiredAlone(t *testing.T) {
	j, _ := New(nil)
	u := mustURL(t, "http://example.com/")
	now := time.Now()
	j.setCookies(u, []*http.Cookie{{Name: "a", Value: "1"}}, now)
	j.setCookies(u, []*http.Cookie{{Name: "gone", Value: "1", Expires: now.Add(time.Hour)}}, now)
	before := j.entries["example.com"]["example.com;/;a"]
	if got := j.entriesAt(u, now.Add(2*time.Hour)); len(got) != 1 {
		t.Fatalf("entries = %+v", got)
	}
	if after := j.entries["example.com"]["example.com;/;a"]; !after.LastAccess.Equal(before.LastAccess) {
		t.Fatal("Entries updated LastAccess")
	}
	if _, ok := j.entries["example.com"]["example.com;/;gone"]; !ok {
		t.Fatal("Entries deleted an expired entry")
	}
}
