package gitflow_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitwire"
	"github.com/kirathecat/kira-studio/internal/flowtest"
)

type fixtureEnvelope struct {
	RepoID    string `json:"repoId"`
	Seq       int    `json:"seq"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	Source    string `json:"source"`
	Remaining int    `json:"remaining"`
	Exhausted bool   `json:"exhausted"`
}

type fixtureDecorationRef struct {
	Kind   string `json:"kind"`
	Name   string `json:"name,omitempty"`
	IsHead bool   `json:"isHead,omitempty"`
}

type fixtureRowDecorations struct {
	Row  int                    `json:"row"`
	Refs []fixtureDecorationRef `json:"refs"`
}

type fixturePackedChunk struct {
	From           int                     `json:"from"`
	To             int                     `json:"to"`
	ShaWidthBytes  int                     `json:"shaWidthBytes"`
	Shas           []string                `json:"shas"`
	ParentOffsets  []uint32                `json:"parentOffsets"`
	ParentShas     []string                `json:"parentShas"`
	IdentityIds    []uint32                `json:"identityIds"`
	Times          []uint32                `json:"times"`
	Subjects       []string                `json:"subjects"`
	SubjectOffsets []uint32                `json:"subjectOffsets"`
	DictionaryBase uint32                  `json:"dictionaryBase"`
	Dictionary     []string                `json:"dictionary"`
	Decorations    []fixtureRowDecorations `json:"decorations"`
}

type fixtureFile struct {
	Envelope fixtureEnvelope    `json:"envelope"`
	Commits  fixturePackedChunk `json:"commits"`
}

func u32Column(raw []byte) []uint32 {
	out := make([]uint32, len(raw)/4)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(raw[i*4 : i*4+4])
	}
	return out
}

func hexColumn(raw []byte, width int) []string {
	if width == 0 {
		return nil
	}
	out := make([]string, len(raw)/width)
	for i := range out {
		out[i] = hex.EncodeToString(raw[i*width : (i+1)*width])
	}
	return out
}

func packedChunkToFixture(t *testing.T, blob []byte) fixturePackedChunk {
	t.Helper()
	if !gitwire.FrameBufferHasIdentifier(blob) {
		t.Fatal("blob is missing the 'KIG1' file identifier")
	}
	frame := gitwire.GetRootAsFrame(blob, 0)
	if frame.PayloadType() != gitwire.PayloadPackedCommitChunk {
		t.Fatalf("frame payload type = %v, want PackedCommitChunk", frame.PayloadType())
	}
	var tab flatbuffers.Table
	if !frame.Payload(&tab) {
		t.Fatal("frame has no payload")
	}
	table := new(gitwire.PackedCommitChunk)
	table.Init(tab.Bytes, tab.Pos)

	width := int(table.ShaWidthBytes())
	shas := hexColumn(table.ShasBytes(), width)
	subjectOffsets := u32Column(table.SubjectOffsetsBytes())
	subjectBytes := table.SubjectBytesBytes()
	subjects := make([]string, len(shas))
	for i := range subjects {
		subjects[i] = string(subjectBytes[subjectOffsets[i]:subjectOffsets[i+1]])
	}
	dictionary := make([]string, table.DictionaryLength())
	for i := range dictionary {
		dictionary[i] = string(table.Dictionary(i))
	}
	decorations := make([]fixtureRowDecorations, table.DecorationsLength())
	for i := range decorations {
		var rd gitwire.RowDecorations
		table.Decorations(&rd, i)
		refs := make([]fixtureDecorationRef, rd.RefsLength())
		for j := range refs {
			var ref gitwire.DecorationRef
			rd.Refs(&ref, j)
			refs[j] = fixtureDecorationRef{Kind: string(ref.Kind()), Name: string(ref.Name()), IsHead: ref.IsHead()}
		}
		decorations[i] = fixtureRowDecorations{Row: int(rd.Row()), Refs: refs}
	}
	return fixturePackedChunk{
		From: int(table.From()), To: int(table.To()), ShaWidthBytes: width,
		Shas: shas, ParentOffsets: u32Column(table.ParentOffsetsBytes()),
		ParentShas:  hexColumn(table.ParentShasBytes(), width),
		IdentityIds: u32Column(table.IdentityIdsBytes()), Times: u32Column(table.TimesBytes()),
		Subjects: subjects, SubjectOffsets: subjectOffsets,
		DictionaryBase: table.DictionaryBase(), Dictionary: dictionary, Decorations: decorations,
	}
}

// The golden corpus packages/git-ipc/src/streamChannel.spec.ts (D16) decodes: the Go encoder and the
// TypeScript decoder must agree byte for byte. The test compares the encoder's frame with the
// committed corpus; KIRA_CONTRACT=write regenerates it.
func TestFixtures_CaptureGraphChunkFrame(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("fixture")
	for _, n := range []string{"0", "1", "2"} {
		repo.Commit("commit "+n, map[string]string{"f.txt": n + "\n"})
	}
	id := r.open(repo.Dir).RepoID

	call := r.gs.Stream("graph.stream", m{"repoId": id}, 2)
	raw, ok, err := call.NextRaw()
	if err != nil || !ok {
		t.Fatalf("first chunk frame: ok=%v err=%v", ok, err)
	}
	header, blob := splitBlobFrame(t, raw)
	var env struct {
		T     string          `json:"t"`
		ID    int             `json:"id"`
		Chunk json.RawMessage `json:"chunk"`
	}
	if err := json.Unmarshal(header, &env); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	var meta fixtureEnvelope
	if err := json.Unmarshal(env.Chunk, &meta); err != nil {
		t.Fatalf("unmarshal chunk payload: %v", err)
	}
	if meta.RepoID != id {
		t.Fatalf("chunk.repoId = %q, want %q", meta.RepoID, id)
	}
	fixture := fixtureFile{Envelope: meta, Commits: packedChunkToFixture(t, blob)}

	outDir := filepath.Join("..", "..", "..", "..", "..", "packages", "git-ipc", "testdata")
	binPath := filepath.Join(outDir, "graphChunkFrame.bin")
	if os.Getenv(flowtest.EnvContract) == "write" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := json.MarshalIndent(fixture, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outDir, "graphChunkFrame.json"), append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("captured graphChunkFrame.bin (%d bytes)", len(raw))
		return
	}
	golden, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read golden: %v (regenerate with %s=write)", err, flowtest.EnvContract)
	}
	_, goldenBlob := splitBlobFrame(t, golden)
	if !bytes.Equal(blob, goldenBlob) {
		t.Fatalf("graph chunk frame differs from %s; rerun with %s=write if the encoder change is intended", binPath, flowtest.EnvContract)
	}
}

// splitBlobFrame returns the JSON header and the FlatBuffers blob of one blob frame.
func splitBlobFrame(t *testing.T, raw []byte) (header, blob []byte) {
	t.Helper()
	if len(raw) < 5 || raw[0] != 0x00 {
		t.Fatalf("frame is not a blob frame: %v", raw)
	}
	n := int(binary.BigEndian.Uint32(raw[1:5]))
	return raw[5 : 5+n], raw[5+n:]
}
