package appflow_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/appupdate"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/releaseline"
)

const updateAsset = "test.dmg"

func updateRelease(tag string, edit func(*releaseline.Release)) releaseline.Release {
	r := releaseline.Release{Tag: tag, Assets: []releaseline.Asset{{ID: 1, Name: updateAsset, State: "uploaded", Size: 10}}}
	if edit != nil {
		edit(&r)
	}
	return r
}

// releasesServer serves rels newest first, two per page, like GitHub's paginated listing.
func releasesServer(t *testing.T, rels []releaseline.Release, failPage int, failWith func(http.ResponseWriter)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == failPage {
			failWith(w)
			return
		}
		lo, hi := (page-1)*2, page*2
		lo, hi = min(lo, len(rels)), min(hi, len(rels))
		_ = json.NewEncoder(w).Encode(rels[lo:hi])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func updateChecker(srv *httptest.Server, running string) *appupdate.Checker {
	return appupdate.NewCheckerFrom(appupdate.CheckerOptions{
		AppName: "Kira Space", Version: running, ReleasesURL: srv.URL, Asset: updateAsset, PerPage: 2,
	})
}

func TestUpdateStepsOneMinorAtATime(t *testing.T) {
	srv := releasesServer(t, []releaseline.Release{
		updateRelease("v0.6.0-rc.1", func(r *releaseline.Release) { r.Prerelease = true }),
		updateRelease("v0.5.1", nil),
		updateRelease("v0.5.0", func(r *releaseline.Release) { r.Draft = true }),
		updateRelease("v0.4.3", nil),
		updateRelease("v0.4.2", func(r *releaseline.Release) { r.Assets = nil }),
		updateRelease("v0.4.1", nil),
		updateRelease("v0.4.0", nil),
		updateRelease("v0.3.14", nil),
		updateRelease("v0.3.9", nil),
	}, 0, nil)

	// Each running version is a fresh process: a new checker, as after a relaunch.
	for _, tc := range []struct{ running, target string }{
		{"0.3.9", "0.3.14"},
		{"0.3.14", "0.4.0"},
		{"0.4.0", "0.4.3"},
		{"0.4.3", "0.5.1"},
		{"0.5.1", ""},
	} {
		app := flowharness.New(t, flowharness.WithUpdateChecker(updateChecker(srv, tc.running)))
		st, err := app.W.Update.Status(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if st.CurrentVersion != tc.running || st.TargetVersion != tc.target || st.UpdateAvailable != (tc.target != "") || st.NewestVersion != "0.5.1" {
			t.Fatalf("running %s: status %+v, want target %q, newest 0.5.1", tc.running, st, tc.target)
		}
	}
}

func TestUpdateCheckFailsClosed(t *testing.T) {
	rels := []releaseline.Release{updateRelease("v0.4.0", nil), updateRelease("v0.3.14", nil), updateRelease("v0.3.9", nil)}
	var ie *ipcerr.Error

	srv := releasesServer(t, rels, 2, func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) })
	app := flowharness.New(t, flowharness.WithUpdateChecker(updateChecker(srv, "0.3.9")))
	if _, err := app.W.Update.Check(ctx); !errors.As(err, &ie) || ie.Code != "E_UNAVAILABLE" {
		t.Fatalf("Check with a failing page = %v, want E_UNAVAILABLE", err)
	}
	if st, _ := app.W.Update.Status(ctx); st.UpdateAvailable {
		t.Fatalf("Status after a failed listing offers %+v, want no update", st)
	}

	srv = releasesServer(t, rels, 1, func(w http.ResponseWriter) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	})
	app = flowharness.New(t, flowharness.WithUpdateChecker(updateChecker(srv, "0.3.9")))
	if _, err := app.W.Update.Check(ctx); !errors.As(err, &ie) || ie.Message != "Couldn’t check for updates (rate limited)." {
		t.Fatalf("Check when rate limited = %v, want the rate limited message", err)
	}
}

func TestUpdateRefusedInDevBuild(t *testing.T) {
	app := flowharness.New(t)
	st, err := app.W.Update.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.UpdateAvailable {
		t.Fatalf("dev build reports an update: %+v", st)
	}
	if err := app.W.Update.InstallUpdate(ctx); err == nil {
		t.Fatal("InstallUpdate succeeded in a dev build")
	}
	if err := app.W.Update.CancelInstall(); err != nil {
		t.Fatalf("CancelInstall with nothing installing: %v", err)
	}
}

func TestCheckForUpdatesRefusedInDevBuild(t *testing.T) {
	app := flowharness.New(t)
	var ie *ipcerr.Error
	if _, err := app.W.Update.Check(ctx); !errors.As(err, &ie) || ie.Code != "E_CONFLICT" || ie.Message != "Updates are available only in release builds." {
		t.Fatalf("Check in a dev build = %v, want the coded notRelease error", err)
	}
}
