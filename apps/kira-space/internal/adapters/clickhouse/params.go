package clickhouse

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// maxBoundParamBytes bounds every bound value's text together: values ride the URL as param_<name>
// (http_max_uri_size defaults to 1 MiB and proxies allow less).
const maxBoundParamBytes = 256 << 10

// paramNull is the value of a NULL parameter. ClickHouse reads `\N` as NULL before any unescaping, so
// buildURL sends it raw where it escapes every other value.
const paramNull = "\x00null"

// paramRaw marks a value buildURL must not escape: ClickHouse parses an array parameter as quoted text
// directly, where it unescapes a scalar one first, so escapeParamValue would double its backslashes.
const paramRaw = "\x00raw:"

// boundParams encodes bound values into ClickHouse parameter text, keyed by placeholder name. The
// server parses each as the type the placeholder declares.
func boundParams(params []model.BindParam) (map[string]string, error) {
	if len(params) == 0 {
		return nil, nil
	}
	vals, err := adapters.DecodeBinds(params)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(params))
	total := 0
	for i, p := range params {
		if p.Name == "" {
			return nil, adapters.New(adapters.CodeQuery, "a ClickHouse bound value needs its placeholder name", nil)
		}
		text := paramText(vals[i])
		total += len(p.Name) + len(text)
		if total > maxBoundParamBytes {
			return nil, adapters.New(adapters.CodeQuery, fmt.Sprintf("bound values are over the %d KiB ClickHouse takes in a request; bind fewer or smaller values", maxBoundParamBytes>>10), nil)
		}
		out[p.Name] = text
	}
	return out, nil
}

func paramText(v adapters.ParamValue) string {
	switch v.Kind {
	case adapters.ParamNull:
		return paramNull
	case adapters.ParamBool:
		return strconv.FormatBool(v.Bool)
	case adapters.ParamList:
		var b strings.Builder
		b.WriteByte('[')
		for i, it := range v.Items {
			if i > 0 {
				b.WriteByte(',')
			}
			switch it.Kind {
			case adapters.ParamString:
				b.WriteString(quotedString(it.Text))
			case adapters.ParamBool:
				b.WriteString(strconv.FormatBool(it.Bool))
			default:
				b.WriteString(it.Text)
			}
		}
		b.WriteByte(']')
		return paramRaw + b.String()
	}
	return v.Text
}

var quoteEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

// quotedString is a list item in ClickHouse's quoted text form.
func quotedString(s string) string { return "'" + quoteEscaper.Replace(s) + "'" }
