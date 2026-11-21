package dbmcp

import (
	"fmt"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// The refusal template is shared by every held tool. These texts are what the DB and API tools sent before
// the template was shared, so a drift here is a wording change the agent sees.
func TestHeldRefusalTexts(t *testing.T) {
	const size = 9 << 20
	win := approvalWindow()
	dbRead := dbHeldCall(queryResolution{summary: model.ConnectionSummary{Name: "shop"}, class: adapters.ClassRead})
	dbWrite := dbHeldCall(queryResolution{summary: model.ConnectionSummary{Name: "shop"}, class: adapters.ClassWrite})
	apiRead := apiHeldCall(agentapi.Planned{Method: "GET", Host: "api.shop"})
	apiWrite := apiHeldCall(agentapi.Planned{Method: "POST", Host: "api.shop", Body: "{}"})
	kube := kubeHeldCall("prod-eu", "list")

	const chg = " Its changes stay."
	const eff = " Its effects stay."
	cases := []struct {
		name string
		c    heldCall
		want map[ApprovalOutcome]string
	}{
		{"db read", dbRead, map[ApprovalOutcome]string{
			ApprovalDenied:    `the user withheld the result of this query on "shop"; it ran, but you get no rows. Do not rerun it to get the result another way.`,
			ApprovalTooLarge:  `the query on "shop" ran, but its result is 9.0 MB, over the 8 MB review limit, so it was withheld unseen. Ask for fewer rows or columns.`,
			ApprovalHeldFull:  `query against "shop" ran, but too many results are waiting for the user's review, so its result was withheld. Do not rerun it.`,
			ApprovalTimedOut:  fmt.Sprintf(`query against "shop" ran, but its result got no review within %s and was withheld.`, win),
			ApprovalAbandoned: `the Studio MCP server stopped before the result of the query against "shop" was reviewed; the query ran and its result was withheld.`,
			ApprovalQueueFull: `query against "shop" ran, but too many queries are waiting for approval, so its result was withheld. Do not rerun it.`,
			ApprovalApproved:  `query against "shop" ran, but its result was not approved.`,
		}},
		{"db write", dbWrite, map[ApprovalOutcome]string{
			ApprovalDenied:    `the user withheld the result of this query on "shop"; it ran, but you get no rows. Do not rerun it to get the result another way.` + chg,
			ApprovalTooLarge:  `the query on "shop" ran, but its result is 9.0 MB, over the 8 MB review limit, so it was withheld unseen.` + chg + " Do not rerun it.",
			ApprovalHeldFull:  `query against "shop" ran, but too many results are waiting for the user's review, so its result was withheld. Do not rerun it.` + chg,
			ApprovalTimedOut:  fmt.Sprintf(`query against "shop" ran, but its result got no review within %s and was withheld.`, win) + chg + " Do not rerun it.",
			ApprovalAbandoned: `the Studio MCP server stopped before the result of the query against "shop" was reviewed; the query ran and its result was withheld.` + chg + " Do not rerun it.",
			ApprovalQueueFull: `query against "shop" ran, but too many queries are waiting for approval, so its result was withheld. Do not rerun it.` + chg,
			ApprovalApproved:  `query against "shop" ran, but its result was not approved.` + chg + " Do not rerun it.",
		}},
		{"api read", apiRead, map[ApprovalOutcome]string{
			ApprovalDenied:    `the user withheld the response to GET api.shop; the request was sent, but you get no response. Do not resend it to get the response another way.`,
			ApprovalTooLarge:  `the GET api.shop request was sent, but its response is 9.0 MB, over the 8 MB review limit, so it was withheld unseen. Ask for less: narrow the request or reduce the response with a transform script.`,
			ApprovalHeldFull:  `the GET api.shop request was sent, but too many results are waiting for the user's review, so its response was withheld. Do not resend it.`,
			ApprovalTimedOut:  fmt.Sprintf(`the GET api.shop request was sent, but its response got no review within %s and was withheld.`, win),
			ApprovalAbandoned: `the Studio MCP server stopped before the response to GET api.shop was reviewed; the request was sent and its response was withheld.`,
			ApprovalQueueFull: `the GET api.shop request was sent, but too many requests are waiting for approval, so its response was withheld. Do not resend it.`,
			ApprovalApproved:  `the GET api.shop request was sent, but its response was not approved.`,
		}},
		{"api write", apiWrite, map[ApprovalOutcome]string{
			ApprovalDenied:    `the user withheld the response to POST api.shop; the request was sent, but you get no response. Do not resend it to get the response another way.` + eff,
			ApprovalTooLarge:  `the POST api.shop request was sent, but its response is 9.0 MB, over the 8 MB review limit, so it was withheld unseen.` + eff + " Do not resend it.",
			ApprovalHeldFull:  `the POST api.shop request was sent, but too many results are waiting for the user's review, so its response was withheld. Do not resend it.` + eff,
			ApprovalAbandoned: `the Studio MCP server stopped before the response to POST api.shop was reviewed; the request was sent and its response was withheld.` + eff + " Do not resend it.",
			ApprovalApproved:  `the POST api.shop request was sent, but its response was not approved.` + eff + " Do not resend it.",
		}},
		{"kube list", kube, map[ApprovalOutcome]string{
			ApprovalDenied:    `the user withheld the result of this Kubernetes list on "prod-eu"; it ran, but you get no data. Do not rerun it to get the result another way.`,
			ApprovalTooLarge:  `the Kubernetes list on "prod-eu" ran, but its result is 9.0 MB, over the 8 MB review limit, so it was withheld unseen. Ask for fewer rows, a narrower selector or a shorter tail.`,
			ApprovalHeldFull:  `the Kubernetes list on "prod-eu" ran, but too many results are waiting for the user's review, so its result was withheld. Do not rerun it.`,
			ApprovalAbandoned: `the Studio MCP server stopped before the result of the Kubernetes list on "prod-eu" was reviewed; the read ran and its result was withheld.`,
			ApprovalQueueFull: `the Kubernetes list on "prod-eu" ran, but too many reads are waiting for approval, so its result was withheld. Do not rerun it.`,
			ApprovalApproved:  `the Kubernetes list on "prod-eu" ran, but its result was not approved.`,
		}},
	}
	for _, tc := range cases {
		for outcome, want := range tc.want {
			if got := heldRefusalText(outcome, tc.c, size); got != want {
				t.Errorf("%s %s:\n got %q\nwant %q", tc.name, outcome, got, want)
			}
		}
	}
}
