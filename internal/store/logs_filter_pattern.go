package store

import (
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
}

// MatchLogFilterPattern reports whether message matches the lab filter subset.
// Empty pattern matches all. Unsupported syntax returns ErrLogFilterPatternInvalid.
//
// Lab subset (case-sensitive substring honesty, not AWS token/word boundaries):
//   - whitespace-separated terms are AND
//   - "quoted phrase" exact substring
//   - unquoted term substring; ? = one char, * = any run within that term
//   - -term / -"phrase" exclude
//
// Rejected: JSON {$.…}, space-delimited […], %regex%, && / ||, | (Insights/OR).
func MatchLogFilterPattern(pattern, message string) (bool, error) {
	terms, err := parseLogFilterPattern(pattern)
	if err != nil {
		return false, err
	}
	if len(terms) == 0 {
		return true, nil
	}
	for _, term := range terms {
		matched, err := matchLogFilterTerm(term, message)
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
	if strings.HasPrefix(trimmed, "{") || strings.Contains(trimmed, "$.") {
		return fmt.Errorf("%w: JSON filter syntax is not supported", ErrLogFilterPatternInvalid)
	}
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
	glob := strings.ContainsAny(tok, "?*")
	return logFilterTerm{exclude: exclude, literal: tok, glob: glob}, nil
}

func matchLogFilterTerm(term logFilterTerm, message string) (bool, error) {
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
