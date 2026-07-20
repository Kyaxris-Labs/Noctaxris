package store

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	rateExprRe = regexp.MustCompile(`(?i)^rate\(\s*(\d+)\s+(minute|minutes|hour|hours|day|days)\s*\)$`)
	// Lab cron subset: cron(minutes hours day-of-month month day-of-week year)
	// Supports digits, *, and ? in day-of-month / day-of-week. No lists, ranges, or steps.
	cronExprRe = regexp.MustCompile(`(?i)^cron\(\s*([^\s]+)\s+([^\s]+)\s+([^\s]+)\s+([^\s]+)\s+([^\s]+)\s+([^\s]+)\s*\)$`)
	atExprRe   = regexp.MustCompile(`(?i)^at\(\s*(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})\s*\)$`)
)

// NextScheduleRun computes the next fire time after `from` for a lab schedule expression.
// Supported: rate(n minutes|hours|days), cron(6-field AWS subset), at(yyyy-mm-ddThh:mm:ss).
func NextScheduleRun(expression string, from time.Time) (time.Time, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return time.Time{}, fmt.Errorf("schedule expression is required")
	}
	from = from.UTC()
	if m := rateExprRe.FindStringSubmatch(expression); len(m) == 3 {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 {
			return time.Time{}, fmt.Errorf("invalid rate value")
		}
		unit := strings.ToLower(m[2])
		var d time.Duration
		switch unit {
		case "minute", "minutes":
			d = time.Duration(n) * time.Minute
		case "hour", "hours":
			d = time.Duration(n) * time.Hour
		case "day", "days":
			d = time.Duration(n) * 24 * time.Hour
		default:
			return time.Time{}, fmt.Errorf("unsupported rate unit")
		}
		return from.Add(d), nil
	}
	if m := atExprRe.FindStringSubmatch(expression); len(m) == 2 {
		t, err := time.ParseInLocation("2006-01-02T15:04:05", m[1], time.UTC)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid at expression: %w", err)
		}
		if !t.After(from) {
			return time.Time{}, fmt.Errorf("at expression is not in the future")
		}
		return t, nil
	}
	if m := cronExprRe.FindStringSubmatch(expression); len(m) == 7 {
		return nextCronRun(from, m[1], m[2], m[3], m[4], m[5], m[6])
	}
	return time.Time{}, fmt.Errorf("unsupported schedule expression %q", expression)
}

func nextCronRun(from time.Time, minute, hour, dom, month, dow, year string) (time.Time, error) {
	if err := validateCronField(minute, 0, 59, false); err != nil {
		return time.Time{}, fmt.Errorf("cron minutes: %w", err)
	}
	if err := validateCronField(hour, 0, 23, false); err != nil {
		return time.Time{}, fmt.Errorf("cron hours: %w", err)
	}
	if err := validateCronField(dom, 1, 31, true); err != nil {
		return time.Time{}, fmt.Errorf("cron day-of-month: %w", err)
	}
	if err := validateCronField(month, 1, 12, false); err != nil {
		return time.Time{}, fmt.Errorf("cron month: %w", err)
	}
	if err := validateCronField(dow, 1, 7, true); err != nil {
		return time.Time{}, fmt.Errorf("cron day-of-week: %w", err)
	}
	if err := validateCronField(year, 1970, 2199, false); err != nil {
		return time.Time{}, fmt.Errorf("cron year: %w", err)
	}

	// Search forward minute-by-minute for up to ~2 years of lab fidelity.
	t := from.Truncate(time.Minute).Add(time.Minute)
	limit := from.Add(730 * 24 * time.Hour)
	for !t.After(limit) {
		if cronFieldMatches(minute, t.Minute(), false) &&
			cronFieldMatches(hour, t.Hour(), false) &&
			cronFieldMatches(month, int(t.Month()), false) &&
			cronFieldMatches(year, t.Year(), false) &&
			cronDayMatches(dom, dow, t) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no matching cron fire time within search window")
}

func validateCronField(field string, min, max int, allowQ bool) error {
	field = strings.TrimSpace(field)
	if field == "*" {
		return nil
	}
	if allowQ && field == "?" {
		return nil
	}
	n, err := strconv.Atoi(field)
	if err != nil {
		return fmt.Errorf("unsupported token %q", field)
	}
	if n < min || n > max {
		return fmt.Errorf("out of range %d", n)
	}
	return nil
}

func cronFieldMatches(field string, value int, allowQ bool) bool {
	field = strings.TrimSpace(field)
	if field == "*" {
		return true
	}
	if allowQ && field == "?" {
		return true
	}
	n, err := strconv.Atoi(field)
	if err != nil {
		return false
	}
	return n == value
}

func cronDayMatches(dom, dow string, t time.Time) bool {
	dom = strings.TrimSpace(dom)
	dow = strings.TrimSpace(dow)
	domStarOrQ := dom == "*" || dom == "?"
	dowStarOrQ := dow == "*" || dow == "?"

	domOK := cronFieldMatches(dom, t.Day(), true)
	// AWS cron: Sunday=1 ... Saturday=7. Go Weekday: Sunday=0.
	awsDow := int(t.Weekday()) + 1
	dowOK := cronFieldMatches(dow, awsDow, true)

	switch {
	case dom == "?" && dow != "?":
		return dowOK
	case dow == "?" && dom != "?":
		return domOK
	case domStarOrQ && dowStarOrQ:
		return true
	case !domStarOrQ && dowStarOrQ:
		return domOK
	case domStarOrQ && !dowStarOrQ:
		return dowOK
	default:
		// Both constrained: AWS requires one to be ?. Lab accepts either match.
		return domOK || dowOK
	}
}
