package bridge_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
)

// recordingDialogs implements bridge.Dialogs, recording every request and answering with a
// pre-set result.
type recordingDialogs struct {
	saveReq bridge.SaveFileRequest
	saveRes string
	saveErr error

	openReq bridge.OpenFileRequest
	openRes string
	openErr error

	openDirReq bridge.OpenDirectoryRequest
	openDirRes string
	openDirErr error
}

func (d *recordingDialogs) SaveFile(req bridge.SaveFileRequest) (string, error) {
	d.saveReq = req
	return d.saveRes, d.saveErr
}

func (d *recordingDialogs) OpenFile(req bridge.OpenFileRequest) (string, error) {
	d.openReq = req
	return d.openRes, d.openErr
}

func (d *recordingDialogs) OpenFiles(bridge.OpenFilesRequest) ([]string, error) { return nil, nil }

func (d *recordingDialogs) OpenDirectory(req bridge.OpenDirectoryRequest) (string, error) {
	d.openDirReq = req
	return d.openDirRes, d.openDirErr
}

// TestWailsFilterTranslation covers the Electron->Wails filter collapse, whose rules do not
// compose obviously: Wails' open panel applies ONE flattened extension set, and it has no
// wildcard at all — it matches a literal ".<ext>" suffix — so any group containing "*" must
// produce NO filter rather than a literal ".*". Getting that wrong hides every file the user
// came to pick, and the dialog itself is the only place it would ever show up.
func TestWailsFilterTranslation(t *testing.T) {
	// wailsFilter is unexported; drive it through ChooseOpen and inspect what reached the dialog.
	tests := []struct {
		name        string
		filters     []bridge.FileFilter
		wantName    string
		wantPattern string
	}{
		{
			name: "ConnectionDialog.vue's real two-filter list collapses to no filter (D8)",
			filters: []bridge.FileFilter{
				{Name: "SQLite database", Extensions: []string{"sqlite", "sqlite3", "db", "db3"}},
				{Name: "All files", Extensions: []string{"*"}},
			},
			wantName: "", wantPattern: "",
		},
		{
			name: "two groups with no wildcard flatten into one pattern",
			filters: []bridge.FileFilter{
				{Name: "Images", Extensions: []string{"png", "jpg"}},
				{Name: "Text", Extensions: []string{"txt"}},
			},
			wantName:    "Images",
			wantPattern: "*.png;*.jpg;*.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &recordingDialogs{openRes: ""}
			svc := &bridge.FilesService{Dialogs: d}
			if _, err := svc.ChooseOpen(bridge.FilesChooseOpenArgs{Filters: tt.filters}); err != nil {
				t.Fatalf("ChooseOpen: %v", err)
			}
			if d.openReq.FilterName != tt.wantName || d.openReq.FilterPattern != tt.wantPattern {
				t.Errorf("got (name=%q, pattern=%q), want (name=%q, pattern=%q)",
					d.openReq.FilterName, d.openReq.FilterPattern, tt.wantName, tt.wantPattern)
			}
		})
	}
}
