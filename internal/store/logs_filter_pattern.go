package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ErrLogFilterPatternInvalid is returned for filter patterns outside the lab subset.
var ErrLogFilterPatternInvalid = errors.New("ValidationException: unsupported filter pattern")

type logFilterTerm struct {
	exclude bool
	literal string // exact substring when quoted or when no wildcards
	quoted  bool
	glob    bool // unquoted term with ? / *

	jsonPath  string // dotted path without $. prefix
	jsonValue string // string equality for JSON field filter
}

var logFilterJSONTermRE = regexp.MustCompile(`^\{\s*\$\.([a-zA-Z][a-zA-Z0-9_.]*)\s*=\s*"([^"]*)"\s*\}$`)

// MatchLogFilterPattern reports whether message matches the lab filter subset.
// Empty pattern matches all. Unsupported syntax returns ErrLogFilterPatternInvalid.
//
// Lab subset (case-sensitive substring honesty, not AWS token/word boundaries):
//   - whitespace-separated terms are AND
//   - "quoted phrase" exact substring
//   - unquoted term substring; ? = one char, * = any run within that term
//   - -term / -"phrase" exclude
//   - { $.path = "value" } JSON field string equality on CT-shaped JSON messages
//
// Rejected: space-delimited […], %regex%, && / ||, | (Insights/OR), JSON outside subset.
func MatchLogFilterPattern(pattern, message string) (bool, error) {
	terms, err := parseLogFilterPattern(pattern)
	if err != nil {
		return false, err
	}
	if len(terms) == 0 {
		return true, nil
	}
	var jsonMsg map[string]any
	needJSON := false
	for _, term := range terms {
		if term.jsonPath != "" {
			needJSON = true
			break
		}
	}
	if needJSON {
		jsonMsg = parseLogFilterJSONMessage(message)
	}
	for _, term := range terms {
		matched, err := matchLogFilterTerm(term, message, jsonMsg)
		if err != nil {
			return false, err
		}
		if term.exclude {
			if matched {
				return false, nil
			}
			continue
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func parseLogFilterPattern(pattern string) ([]logFilterTerm, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, nil
	}
	if err := rejectUnsupportedLogFilterPattern(pattern); err != nil {
		return nil, err
	}
	tokens, err := tokenizeLogFilterPattern(pattern)
	if err != nil {
		return nil, err
	}
	terms := make([]logFilterTerm, 0, len(tokens))
	for _, tok := range tokens {
		term, err := parseLogFilterToken(tok)
		if err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}
	return terms, nil
}

func rejectUnsupportedLogFilterPattern(pattern string) error {
	trimmed := strings.TrimSpace(pattern)
	if strings.HasPrefix(trimmed, "[") {
		return fmt.Errorf("%w: space-delimited field syntax is not supported", ErrLogFilterPatternInvalid)
	}
	if strings.Contains(trimmed, "%") {
		return fmt.Errorf("%w: regex %%...%% syntax is not supported", ErrLogFilterPatternInvalid)
	}
	if strings.Contains(trimmed, "&&") || strings.Contains(trimmed, "||") {
		return fmt.Errorf("%w: boolean operators are not supported", ErrLogFilterPatternInvalid)
	}
	if strings.Contains(trimmed, "|") {
		return fmt.Errorf("%w: Insights query syntax is not supported", ErrLogFilterPatternInvalid)
	}
	if strings.Contains(trimmed, "$.") {
		tokens, err := tokenizeLogFilterPattern(trimmed)
		if err != nil {
			return err
		}
		for _, tok := range tokens {
			if strings.Contains(tok, "$.") {
				if _, _, err := parseLogFilterJSONTerm(tok); err != nil {
					return fmt.Errorf("%w: JSON filter syntax is not supported", ErrLogFilterPatternInvalid)
				}
			}
		}
	}
	if strings.Contains(trimmed, "{") {
		tokens, err := tokenizeLogFilterPattern(trimmed)
		if err != nil {
			return err
		}
		for _, tok := range tokens {
			if strings.Contains(tok, "{") || strings.Contains(tok, "}") {
				if _, _, err := parseLogFilterJSONTerm(tok); err != nil {
					return fmt.Errorf("%w: JSON filter syntax is not supported", ErrLogFilterPatternInvalid)
				}
			}
		}
	}
	return nil
}

func tokenizeLogFilterPattern(pattern string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inQuote := false

	flush := func() {
		if cur.Len() == 0 {
			return
		}
		tokens = append(tokens, cur.String())
		cur.Reset()
	}

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch {
		case ch == '"':
			cur.WriteByte(ch)
			inQuote = !inQuote
		case ch == '{' && !inQuote:
			flush()
			end, err := readLogFilterBraceToken(pattern, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, pattern[i:end+1])
			i = end
		case unicode.IsSpace(rune(ch)) && !inQuote:
			flush()
		default:
			cur.WriteByte(ch)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("%w: unclosed quoted phrase", ErrLogFilterPatternInvalid)
	}
	flush()
	if len(tokens) == 0 {
		return nil, fmt.Errorf("%w: empty filter pattern", ErrLogFilterPatternInvalid)
	}
	return tokens, nil
}

func readLogFilterBraceToken(pattern string, start int) (end int, err error) {
	if pattern[start] != '{' {
		return 0, fmt.Errorf("%w: malformed JSON filter term", ErrLogFilterPatternInvalid)
	}
	inQuote := false
	depth := 0
	for i := start; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '"':
			inQuote = !inQuote
		case '{':
			if !inQuote {
				depth++
			}
		case '}':
			if !inQuote {
				depth--
				if depth == 0 {
					return i, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("%w: malformed JSON filter term", ErrLogFilterPatternInvalid)
}

func parseLogFilterToken(tok string) (logFilterTerm, error) {
	if tok == "" || tok == "-" {
		return logFilterTerm{}, fmt.Errorf("%w: empty filter term", ErrLogFilterPatternInvalid)
	}
	exclude := false
	if strings.HasPrefix(tok, "-") {
		exclude = true
		tok = tok[1:]
		if tok == "" {
			return logFilterTerm{}, fmt.Errorf("%w: empty exclude term", ErrLogFilterPatternInvalid)
		}
	}

	if strings.HasPrefix(tok, "{") {
		path, value, err := parseLogFilterJSONTerm(tok)
		if err != nil {
			return logFilterTerm{}, err
		}
		return logFilterTerm{exclude: exclude, jsonPath: path, jsonValue: value}, nil
	}

	if strings.HasPrefix(tok, `"`) {
		if len(tok) < 2 || !strings.HasSuffix(tok, `"`) {
			return logFilterTerm{}, fmt.Errorf("%w: unclosed quoted phrase", ErrLogFilterPatternInvalid)
		}
		inner := tok[1 : len(tok)-1]
		if strings.Contains(inner, `"`) {
			return logFilterTerm{}, fmt.Errorf("%w: nested quotes are not supported", ErrLogFilterPatternInvalid)
		}
		return logFilterTerm{exclude: exclude, literal: inner, quoted: true}, nil
	}

	if strings.Contains(tok, `"`) {
		return logFilterTerm{}, fmt.Errorf("%w: malformed quoted term", ErrLogFilterPatternInvalid)
	}
	if strings.Contains(tok, "$.") {
		return logFilterTerm{}, fmt.Errorf("%w: JSON filter syntax is not supported", ErrLogFilterPatternInvalid)
	}
	glob := strings.ContainsAny(tok, "?*")
	return logFilterTerm{exclude: exclude, literal: tok, glob: glob}, nil
}

func parseLogFilterJSONTerm(tok string) (path, value string, err error) {
	m := logFilterJSONTermRE.FindStringSubmatch(strings.TrimSpace(tok))
	if m == nil {
		return "", "", fmt.Errorf("%w: JSON filter syntax is not supported", ErrLogFilterPatternInvalid)
	}
	return m[1], m[2], nil
}

func parseLogFilterJSONMessage(message string) map[string]any {
	msg := strings.TrimSpace(message)
	if msg == "" || !strings.HasPrefix(msg, "{") {
		return nil
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(msg), &root); err != nil {
		return nil
	}
	return root
}

func logFilterJSONFieldValue(root map[string]any, dottedPath string) (string, bool) {
	if root == nil || dottedPath == "" {
		return "", false
	}
	var cur any = root
	for _, seg := range strings.Split(dottedPath, ".") {
		if seg == "" {
			return "", false
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		next, ok := obj[seg]
		if !ok {
			return "", false
		}
		cur = next
	}
	switch v := cur.(type) {
	case string:
		return v, true
	default:
		return "", false
	}
}

func matchLogFilterTerm(term logFilterTerm, message string, jsonMsg map[string]any) (bool, error) {
	if term.jsonPath != "" {
		got, ok := logFilterJSONFieldValue(jsonMsg, term.jsonPath)
		if !ok {
			return false, nil
		}
		return got == term.jsonValue, nil
	}
	if term.quoted || !term.glob {
		return strings.Contains(message, term.literal), nil
	}
	re, err := logFilterGlobRegexp(term.literal)
	if err != nil {
		return false, err
	}
	return re.MatchString(message), nil
}

func logFilterGlobRegexp(term string) (*regexp.Regexp, error) {
	var b strings.Builder
	for i := 0; i < len(term); i++ {
		switch term[i] {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(term[i : i+1]))
		}
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("%w: invalid wildcard term", ErrLogFilterPatternInvalid)
	}
	return re, nil
}
