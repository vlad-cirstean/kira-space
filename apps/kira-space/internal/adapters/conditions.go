package adapters

import (
	"encoding/json"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ConditionOperators maps each structured-condition operator to its SQL spelling.
var ConditionOperators = map[string]string{
	model.CondEq: "=", model.CondNe: "<>", model.CondLt: "<", model.CondLte: "<=",
	model.CondGt: ">", model.CondGte: ">=", model.CondLike: "LIKE",
}

// BuildConditionsSQL renders the structured conditions as `(c1) AND (c2) …`, every value bound as a
// parameter and never spelled into the text (P535). placeholder gets the column so ClickHouse can type
// it; a "?"-style or "$N"-style dialect ignores it. A binary column's value arrives as "0x<hex>" display
// text and binds as bytes. Numbers bind as their exact text, a bool as a bool.
func BuildConditionsSQL(conds []model.Condition, columns map[string]model.ColumnMeta, quote func(string) string,
	placeholder func(idx int, col model.ColumnMeta) string, addParam func(any) int, isBinary func(string) bool) (string, error) {
	parts := make([]string, 0, len(conds))
	for _, c := range conds {
		col, ok := columns[c.Column]
		if !ok {
			return "", New(CodeNotFound, "unknown column in condition: "+c.Column, nil)
		}
		name := quote(c.Column)
		switch c.Op {
		case model.CondIsNull:
			parts = append(parts, "("+name+" IS NULL)")
		case model.CondNotNull:
			parts = append(parts, "("+name+" IS NOT NULL)")
		case model.CondIn:
			if len(c.Values) == 0 {
				return "", Contract("in condition on "+c.Column+" has no values", nil)
			}
			phs := make([]string, len(c.Values))
			for i, v := range c.Values {
				bound, err := conditionValue(v, c.Column, isBinary(c.Column))
				if err != nil {
					return "", err
				}
				phs[i] = placeholder(addParam(bound), col)
			}
			parts = append(parts, "("+name+" IN ("+strings.Join(phs, ", ")+"))")
		default:
			op, ok := ConditionOperators[c.Op]
			if !ok {
				return "", Contract("unknown condition operator: "+c.Op, nil)
			}
			if len(c.Values) != 1 {
				return "", Contract(c.Op+" condition on "+c.Column+" needs one value", nil)
			}
			bound, err := conditionValue(c.Values[0], c.Column, isBinary(c.Column))
			if err != nil {
				return "", err
			}
			parts = append(parts, "("+name+" "+op+" "+placeholder(addParam(bound), col)+")")
		}
	}
	return strings.Join(parts, " AND "), nil
}

func conditionValue(v any, column string, binary bool) (any, error) {
	var text string
	switch x := v.(type) {
	case bool:
		if binary {
			return nil, Contract("a boolean cannot filter binary column "+column, nil)
		}
		return x, nil
	case json.Number:
		text = x.String()
	case string:
		text = x
	default:
		return nil, Contract("condition value on "+column+" must be a string, number or boolean", nil)
	}
	if !binary {
		return text, nil
	}
	decoded, err := DecodeBinaryCellText(text)
	if err != nil {
		return nil, New(CodeQuery, "condition value on binary column "+column+": "+err.Error(), nil)
	}
	return decoded, nil
}

// AppendWhere ANDs extra (an already-parenthesised predicate list) onto a WHERE clause, or starts one.
func AppendWhere(whereSQL, extra string) string {
	switch {
	case extra == "":
		return whereSQL
	case whereSQL == "":
		return "WHERE " + extra
	default:
		return whereSQL + " AND " + extra
	}
}

// ApplyConditions ANDs the structured conditions onto whereSQL for a relational page. Binary detection
// covers every catalog column, not just the projected ones: a condition may filter a column the page omits.
func ApplyConditions(whereSQL string, conds []model.Condition, columns []model.ColumnMeta, quote func(string) string,
	placeholder func(int) string, addParam func(any) int, typeClassFor func(string) page.TypeClass) (string, error) {
	if len(conds) == 0 {
		return whereSQL, nil
	}
	byName := make(map[string]model.ColumnMeta, len(columns))
	for _, c := range columns {
		byName[c.Name] = c
	}
	extra, err := BuildConditionsSQL(conds, byName, quote, func(idx int, _ model.ColumnMeta) string { return placeholder(idx) },
		addParam, BinaryColumnsOf(columns, typeClassFor))
	if err != nil {
		return "", err
	}
	return AppendWhere(whereSQL, extra), nil
}

// RejectConditions refuses structured conditions on an engine without them; dbmcp never sends them there.
func RejectConditions(kind string, conds []model.Condition) error {
	if len(conds) > 0 {
		return Unsupported(kind, "structured conditions")
	}
	return nil
}
