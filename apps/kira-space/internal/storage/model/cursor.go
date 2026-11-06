package model

import (
	"encoding/json"
	"fmt"
)

// PageCursor is protocol/dataOps.ts's PageCursor discriminated union, following the same
// Marshal/Unmarshal pattern as SortSpec (queries.go): the struct carries all three arms' fields,
// but marshalling only ever emits the one matching Mode, and unmarshalling rejects a value
// carrying the wrong field for its Mode.
type PageCursor struct {
	Mode   string // "offset" | "after" | "before"
	Offset int    // offset only
	Token  string // after | before only
}

func (c PageCursor) MarshalJSON() ([]byte, error) {
	switch c.Mode {
	case "offset":
		return json.Marshal(struct {
			Mode   string `json:"mode"`
			Offset int    `json:"offset"`
		}{Mode: "offset", Offset: c.Offset})
	case "after", "before":
		return json.Marshal(struct {
			Mode  string `json:"mode"`
			Token string `json:"token"`
		}{Mode: c.Mode, Token: c.Token})
	default:
		return nil, fmt.Errorf("model: PageCursor: invalid mode %q", c.Mode)
	}
}

func (c *PageCursor) UnmarshalJSON(data []byte) error {
	var probe struct {
		Mode   string          `json:"mode"`
		Offset json.RawMessage `json:"offset"`
		Token  json.RawMessage `json:"token"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	switch probe.Mode {
	case "offset":
		if probe.Offset == nil {
			return fmt.Errorf("model: PageCursor: offset mode missing offset")
		}
		var offset int
		if err := json.Unmarshal(probe.Offset, &offset); err != nil {
			return fmt.Errorf("model: PageCursor: invalid offset: %w", err)
		}
		if offset < 0 {
			return fmt.Errorf("model: PageCursor: offset must be >= 0")
		}
		*c = PageCursor{Mode: "offset", Offset: offset}
		return nil
	case "after", "before":
		if probe.Token == nil {
			return fmt.Errorf("model: PageCursor: %s mode missing token", probe.Mode)
		}
		var token string
		if err := json.Unmarshal(probe.Token, &token); err != nil {
			return fmt.Errorf("model: PageCursor: invalid token: %w", err)
		}
		*c = PageCursor{Mode: probe.Mode, Token: token}
		return nil
	default:
		return fmt.Errorf("model: PageCursor: unknown mode %q", probe.Mode)
	}
}
