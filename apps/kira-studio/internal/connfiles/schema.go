package connfiles

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/palette"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

//go:embed connection.schema.json
var connectionSchema []byte

const (
	ruleName   = "name is not blank."
	ruleMode   = "Fields mode needs host and port, except sqlite (an absolute database file path) and sqs and s3 (no host or port). password is not for sqlite, sqs and s3. options holds no secret such as password or sslpassword: use uri mode for those. Uri mode needs uri and no host, port, database, username or password."
	ruleLimits = "throttle_per_sec is 0 or 0.01 to 1000."
	ruleMasks  = "A mask rule is unique per table and column, ignoring case. A number mask does not correlate."
	ruleReview = "mask_review is for postgres, mysql, mariadb, clickhouse and sqlite; each column entry has one part more than a table entry of the kind."
	ruleTree   = "tree lists each hidden kind and path once."
	ruleStem   = "The file name without .yaml is the connection id: lowercase letters, digits and -, starting with a letter or digit, up to 60 characters. A file with another name is listed but not used."
)

// assembleSchema writes the enums and bounds that live in Go, so the schema and the code cannot drift.
func assembleSchema() ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(connectionSchema, &root); err != nil {
		return nil, fmt.Errorf("connfiles: schema: %w", err)
	}
	enum := func(names []string) []any {
		out := make([]any, len(names))
		for i, n := range names {
			out[i] = n
		}
		return out
	}
	props := root["properties"].(map[string]any)
	props["kind"].(map[string]any)["enum"] = enum(model.ConnectionKinds())
	props["color"].(map[string]any)["enum"] = enum(palette.Names())
	props["throttle_per_sec"].(map[string]any)["maximum"] = ThrottleMax
	props["throttle_per_sec"].(map[string]any)["minimum"] = 0
	props["masks"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["kind"].(map[string]any)["enum"] = enum(model.MaskKinds())
	return json.Marshal(root)
}

// Decoder holds the file decoder and its AGENTS.md model.
type decoder struct {
	decode configfs.DecodeFunc[File]
	doc    configfs.DocModel
}

func newDecoder() (decoder, error) {
	schema, err := assembleSchema()
	if err != nil {
		return decoder{}, err
	}
	rule := func(doc string, check func(File) *ValidationError) configfs.Rule[File] {
		return configfs.Rule[File]{Doc: doc, Check: func(f File, _ configfs.Locator) *configfs.Problem {
			if err := check(f); err != nil {
				return err.Problem()
			}
			return nil
		}}
	}
	decode, doc, err := configfs.YAMLSchema[File](schema,
		rule(ruleName, func(f File) *ValidationError {
			if err := f.checkIdentity(); err != nil {
				return err
			}
			return f.checkMCP()
		}),
		rule(ruleLimits, File.checkLimits), rule(ruleMode, File.checkMode), rule(ruleMasks, File.checkMasks), rule(ruleReview, File.checkMaskReview), rule(ruleTree, File.checkTree))
	if err != nil {
		return decoder{}, err
	}
	doc.Rules = append(doc.Rules, ruleStem)
	return decoder{decode: withStem(decode), doc: doc}, nil
}

// withStem refuses a file whose name is not a valid id, before the content is read.
func withStem(decode configfs.DecodeFunc[File]) configfs.DecodeFunc[File] {
	return func(file string, src []byte) (File, *configfs.Problem) {
		if !fileNameRe.MatchString(file) {
			return File{}, &configfs.Problem{Message: i18n.T("backend.studio.connectionFiles.fileName.body", nil)}
		}
		return decode(file, src)
	}
}
