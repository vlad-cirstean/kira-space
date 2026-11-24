package appflow_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/clisetup"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeaz"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/azure"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// P417 Settings, CLI tools. Contract space-cli-tools: tests/ui/settings-cli-tools.spec.ts reads these.

func toolOf(t *testing.T, res bridge.ToolsResult, provider string) bridge.Tool {
	t.Helper()
	for _, tool := range res.Tools {
		if tool.Provider == provider {
			return tool
		}
	}
	t.Fatalf("no tool %q in %+v", provider, res.Tools)
	return bridge.Tool{}
}

func purposes(s clisetup.Setup) []string {
	var out []string
	for _, st := range s.Steps {
		out = append(out, st.Purpose)
	}
	return out
}

func TestCliTools(t *testing.T) {
	az := fakeaz.New()
	az.SetLoggedIn(false)
	app := flowharness.New(t, flowharness.WithAzure(azure.Options{Locator: az, Runner: az}))
	write := func(name, body string) string {
		p := filepath.Join(app.FakeDir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	app.Scenario(fakeagent.Scenario{Gh: []fakeagent.GhRule{
		{Match: "--version", Emit: write("gh-version.txt", "gh version 2.42.0 (2024-01-08)\n")},
		{Match: "auth status", Emit: write("gh-auth.txt", "github.com\n  Logged in to github.com account octocat (keyring)\n")},
	}})
	tools := func(refresh bool) bridge.ToolsResult {
		args := bridge.ToolsArgs{Refresh: refresh}
		res, err := app.W.GitHost.Tools(ctx, args)
		if err != nil {
			t.Fatalf("Tools: %v", err)
		}
		return res
	}

	// 1. gh is ready, az is installed with its extension, signed out and with no default organization.
	app.Contract(t, "space-cli-tools", "args:GitHostService.Tools", bridge.ToolsArgs{Refresh: true})
	first := tools(false)
	if len(first.Tools) != 2 || first.Tools[0].Kind != "host" || first.Tools[1].Kind != "host" {
		t.Fatalf("tools = %+v, want the two hosts", first.Tools)
	}
	gh := toolOf(t, first, "github")
	if gh.Status.Kind != "ok" || gh.Status.Account != "octocat" || gh.Status.Version != "2.42.0" || gh.Label != "GitHub" || gh.CLI != "GitHub CLI (gh)" {
		t.Fatalf("gh = %+v", gh)
	}
	if got := purposes(gh.Setup); !reflect.DeepEqual(got, []string{"install", "login", "check"}) {
		t.Fatalf("gh steps = %v", got)
	}

	// 2. Azure Repos steps and states.
	azure0 := toolOf(t, first, "azure")
	if azure0.Label != "Azure Repos" || azure0.CLI != "Azure CLI (az)" || azure0.Kind != "host" {
		t.Fatalf("azure = %+v", azure0)
	}
	if got := purposes(azure0.Setup); !reflect.DeepEqual(got, []string{"install", "extension", "login", "loginToken", "defaults", "check"}) {
		t.Fatalf("az steps = %v", got)
	}
	if st := azure0.Status; st.Kind != "unauthenticated" || !strings.Contains(st.Reason, "az devops configure") {
		t.Fatalf("az without defaults = %+v, want unauthenticated naming az devops configure", st)
	}
	app.Contract(t, "space-cli-tools", "GitHostService.Tools#azure-no-defaults", first, flowharness.Mask("path"))

	az.SetDefaults("https://dev.azure.com/acme", "Web App")
	app.Contract(t, "space-cli-tools", "args:GitHostService.Tools#azure-refresh", bridge.ToolsArgs{Refresh: true})
	signedOut := tools(true)
	if st := toolOf(t, signedOut, "azure").Status; st.Kind != "unauthenticated" || !strings.Contains(st.Reason, "az login") || st.Site != "https://dev.azure.com/acme" {
		t.Fatalf("az signed out = %+v, want unauthenticated naming az login", st)
	}
	app.Contract(t, "space-cli-tools", "GitHostService.Tools#signed-out", signedOut, flowharness.Mask("path"))
	az.SetLoggedIn(true)
	if st := toolOf(t, tools(false), "azure").Status; st.Kind != "unauthenticated" {
		t.Fatalf("cached az status = %+v, want the signed-out probe", st)
	}
	azReady := tools(true)
	if st := toolOf(t, azReady, "azure").Status; st.Kind != "ok" || st.Site != "https://dev.azure.com/acme" || st.Account != "me@acme.test" || st.Version != "2.91.0 (azure-devops 1.0.10)" {
		t.Fatalf("az refreshed = %+v", st)
	}
	app.Contract(t, "space-cli-tools", "GitHostService.Tools#azure-ready", azReady, flowharness.Mask("path"))

	az.SetExtension(false)
	noExt := tools(true)
	if st := toolOf(t, noExt, "azure").Status; st.Kind != "notFound" || !strings.Contains(st.Reason, "az extension add") {
		t.Fatalf("az without the extension = %+v, want notFound naming az extension add", st)
	}
	app.Contract(t, "space-cli-tools", "GitHostService.Tools#azure-no-extension", noExt, flowharness.Mask("path"))

	// 3. A missing CLI lists the paths looked at.
	az.SetInstalled(false)
	missing := tools(true)
	if st := toolOf(t, missing, "azure").Status; st.Kind != "notFound" || len(st.Probed) == 0 {
		t.Fatalf("missing status = %+v, want notFound with probed paths", st)
	}
	app.Contract(t, "space-cli-tools", "GitHostService.Tools#not-installed", missing, flowharness.Mask("path"))
}
