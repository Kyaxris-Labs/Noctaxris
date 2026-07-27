package dynamodb

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	rePartiQLUnsupported = regexp.MustCompile(`(?i)\b(JOIN|UNION|INTERSECT|EXCEPT|EXISTS|IN\s*\(|GROUP\s+BY|ORDER\s+BY|HAVING|LIMIT|OFFSET)\b`)
	reNestedSelect       = regexp.MustCompile(`(?i)\(\s*SELECT\b`)
	reInsert             = regexp.MustCompile(`(?is)^\s*INSERT\s+INTO\s+("?[A-Za-z0-9_.-]+"?)\s+VALUE\s+(.+)\s*$`)
	reDelete             = regexp.MustCompile(`(?is)^\s*DELETE\s+FROM\s+("?[A-Za-z0-9_.-]+"?)\s+WHERE\s+(.+)\s*$`)
	reSelect             = regexp.MustCompile(`(?is)^\s*SELECT\s+(\*|[A-Za-z0-9_,".\s]+)\s+FROM\s+("?[A-Za-z0-9_.-]+"?)\s+WHERE\s+(.+)\s*$`)
	reUpdate             = regexp.MustCompile(`(?is)^\s*UPDATE\s+("?[A-Za-z0-9_.-]+"?)\s+SET\s+(.+?)\s+WHERE\s+(.+)\s*$`)
	reWhereEq            = regexp.MustCompile(`(?i)^\s*"?([A-Za-z0-9_.-]+)"?\s*=\s*(.+)$`)
	reSetAssign          = regexp.MustCompile(`(?i)^\s*"?([A-Za-z0-9_.-]+)"?\s*=\s*(.+)$`)
)

// PartiQLOp is a parsed lab PartiQL statement.
type PartiQLOp struct {
	Kind      string // INSERT, SELECT, UPDATE, DELETE
	TableName string
	Item      ItemMap            // INSERT
	Key       ItemMap            // SELECT/UPDATE/DELETE WHERE keys
	SetAttrs  map[string]any     // UPDATE SET attr -> AttributeValue map (or placeholder)
	SelectAll bool
}

// ParsePartiQLStatement parses a lab subset of DynamoDB PartiQL.
// Parameters are AttributeValue maps (DynamoDB JSON) substituted for `?` left-to-right.
func ParsePartiQLStatement(statement string, parameters []any) (PartiQLOp, error) {
	stmt := strings.TrimSpace(statement)
	if stmt == "" {
		return PartiQLOp{}, fmt.Errorf("Statement is required")
	}
	if rePartiQLUnsupported.MatchString(stmt) || reNestedSelect.MatchString(stmt) {
		return PartiQLOp{}, fmt.Errorf("unsupported PartiQL clause")
	}
	params := parameters
	nextParam := func() (map[string]any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("not enough Parameters for Statement")
		}
		av, err := paramToAV(params[0])
		params = params[1:]
		return av, err
	}

	if m := reInsert.FindStringSubmatch(stmt); m != nil {
		table := unquoteIdent(m[1])
		valueExpr := strings.TrimSpace(m[2])
		item, err := parsePartiQLValueMap(valueExpr, nextParam)
		if err != nil {
			return PartiQLOp{}, err
		}
		return PartiQLOp{Kind: "INSERT", TableName: table, Item: item}, nil
	}
	if m := reDelete.FindStringSubmatch(stmt); m != nil {
		table := unquoteIdent(m[1])
		key, err := parsePartiQLWhere(m[2], nextParam)
		if err != nil {
			return PartiQLOp{}, err
		}
		return PartiQLOp{Kind: "DELETE", TableName: table, Key: key}, nil
	}
	if m := reSelect.FindStringSubmatch(stmt); m != nil {
		table := unquoteIdent(m[2])
		key, err := parsePartiQLWhere(m[3], nextParam)
		if err != nil {
			return PartiQLOp{}, err
		}
		sel := strings.TrimSpace(m[1])
		return PartiQLOp{Kind: "SELECT", TableName: table, Key: key, SelectAll: sel == "*" || sel == ""}, nil
	}
	if m := reUpdate.FindStringSubmatch(stmt); m != nil {
		table := unquoteIdent(m[1])
		setMap, err := parsePartiQLSet(m[2], nextParam)
		if err != nil {
			return PartiQLOp{}, err
		}
		key, err := parsePartiQLWhere(m[3], nextParam)
		if err != nil {
			return PartiQLOp{}, err
		}
		return PartiQLOp{Kind: "UPDATE", TableName: table, Key: key, SetAttrs: setMap}, nil
	}
	return PartiQLOp{}, fmt.Errorf("unsupported PartiQL statement")
}

func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func paramToAV(v any) (map[string]any, error) {
	switch t := v.(type) {
	case map[string]any:
		return t, nil
	case string:
		return map[string]any{"S": t}, nil
	case float64:
		// JSON numbers decode as float64
		if t == float64(int64(t)) {
			return map[string]any{"N": fmt.Sprintf("%d", int64(t))}, nil
		}
		return map[string]any{"N": fmt.Sprintf("%v", t)}, nil
	case bool:
		return map[string]any{"BOOL": t}, nil
	case nil:
		return map[string]any{"NULL": true}, nil
	default:
		return nil, fmt.Errorf("unsupported Parameter type")
	}
}

func parsePartiQLWhere(expr string, nextParam func() (map[string]any, error)) (ItemMap, error) {
	parts := splitAND(expr)
	out := ItemMap{}
	for _, part := range parts {
		m := reWhereEq.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return nil, fmt.Errorf("unsupported WHERE clause (equality on primary keys only)")
		}
		name := unquoteIdent(m[1])
		rhs := strings.TrimSpace(m[2])
		av, err := parsePartiQLScalar(rhs, nextParam)
		if err != nil {
			return nil, err
		}
		out[name] = av
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("WHERE must include primary key equality")
	}
	return out, nil
}

func parsePartiQLSet(expr string, nextParam func() (map[string]any, error)) (map[string]any, error) {
	parts := splitCommaTop(expr)
	out := map[string]any{}
	for _, part := range parts {
		m := reSetAssign.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return nil, fmt.Errorf("unsupported SET clause")
		}
		name := unquoteIdent(m[1])
		av, err := parsePartiQLScalar(strings.TrimSpace(m[2]), nextParam)
		if err != nil {
			return nil, err
		}
		out[name] = av
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("SET requires at least one assignment")
	}
	return out, nil
}

func parsePartiQLScalar(rhs string, nextParam func() (map[string]any, error)) (map[string]any, error) {
	if rhs == "?" {
		return nextParam()
	}
	if strings.HasPrefix(rhs, "'") && strings.HasSuffix(rhs, "'") && len(rhs) >= 2 {
		return map[string]any{"S": strings.ReplaceAll(rhs[1:len(rhs)-1], "''", "'")}, nil
	}
	if strings.EqualFold(rhs, "true") || strings.EqualFold(rhs, "false") {
		return map[string]any{"BOOL": strings.EqualFold(rhs, "true")}, nil
	}
	// bare number
	if matched, _ := regexp.MatchString(`^-?[0-9]+(\.[0-9]+)?$`, rhs); matched {
		return map[string]any{"N": rhs}, nil
	}
	return nil, fmt.Errorf("unsupported literal in PartiQL (use ? Parameters)")
}

func parsePartiQLValueMap(expr string, nextParam func() (map[string]any, error)) (ItemMap, error) {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "{") || !strings.HasSuffix(expr, "}") {
		return nil, fmt.Errorf("INSERT VALUE must be a map literal")
	}
	inner := strings.TrimSpace(expr[1 : len(expr)-1])
	if inner == "" {
		return ItemMap{}, nil
	}
	parts := splitCommaTop(inner)
	out := ItemMap{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid INSERT VALUE entry")
		}
		name := unquoteIdent(strings.TrimSpace(strings.Trim(kv[0], "'")))
		av, err := parsePartiQLScalar(strings.TrimSpace(kv[1]), nextParam)
		if err != nil {
			return nil, err
		}
		out[name] = av
	}
	return out, nil
}

func splitAND(expr string) []string {
	upper := strings.ToUpper(expr)
	var parts []string
	start := 0
	depth := 0
	for i := 0; i < len(expr); {
		if expr[i] == '(' {
			depth++
			i++
			continue
		}
		if expr[i] == ')' {
			depth--
			i++
			continue
		}
		if depth == 0 && i+4 <= len(expr) && upper[i:i+4] == " AND" && (i+4 == len(expr) || expr[i+4] == ' ') {
			// also handle "AND" with leading space already counted
		}
		if depth == 0 && i+5 <= len(expr) && strings.HasPrefix(upper[i:], " AND ") {
			parts = append(parts, strings.TrimSpace(expr[start:i]))
			start = i + 5
			i = start
			continue
		}
		i++
	}
	parts = append(parts, strings.TrimSpace(expr[start:]))
	return parts
}

func splitCommaTop(expr string) []string {
	var parts []string
	start := 0
	depth := 0
	inQuote := false
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if c == '\'' && !inQuote {
			inQuote = true
			continue
		}
		if c == '\'' && inQuote {
			inQuote = false
			continue
		}
		if inQuote {
			continue
		}
		if c == '{' || c == '[' || c == '(' {
			depth++
			continue
		}
		if c == '}' || c == ']' || c == ')' {
			depth--
			continue
		}
		if c == ',' && depth == 0 {
			parts = append(parts, strings.TrimSpace(expr[start:i]))
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(expr[start:]))
	return parts
}

// ExecuteStatementJSON builds an ExecuteStatement success body.
func ExecuteStatementJSON(items []ItemMap) ([]byte, error) {
	if items == nil {
		items = []ItemMap{}
	}
	out := make([]any, 0, len(items))
	for _, it := range items {
		out = append(out, it)
	}
	return json.Marshal(map[string]any{"Items": out})
}

// BatchExecuteStatementJSON builds a BatchExecuteStatement success body.
func BatchExecuteStatementJSON(responses []map[string]any) ([]byte, error) {
	if responses == nil {
		responses = []map[string]any{}
	}
	return json.Marshal(map[string]any{"Responses": responses})
}
