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
	// Supports digits, *, ?, lists, ranges, steps, L, nW, LW, #, and month/DOW names.
	cronExprRe = regexp.MustCompile(`(?i)^cron\(\s*([^\s]+)\s+([^\s]+)\s+([^\s]+)\s+([^\s]+)\s+([^\s]+)\s+([^\s]+)\s*\)$`)
	atExprRe   = regexp.MustCompile(`(?i)^at\(\s*(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})\s*\)$`)
)

var cronDOWNames = map[string]int{
	"SUN": 1, "MON": 2, "TUE": 3, "WED": 4, "THU": 5, "FRI": 6, "SAT": 7,
}

var cronMonthNames = map[string]int{
	"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
	"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
}

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
	if err := validateCronDOMField(dom); err != nil {
		return time.Time{}, fmt.Errorf("cron day-of-month: %w", err)
	}
	if err := validateCronNamedField(month, 1, 12, false, cronMonthNames); err != nil {
		return time.Time{}, fmt.Errorf("cron month: %w", err)
	}
	if err := validateCronDOWField(dow); err != nil {
		return time.Time{}, fmt.Errorf("cron day-of-week: %w", err)
	}
	if err := validateCronField(year, 1970, 2199, false); err != nil {
		return time.Time{}, fmt.Errorf("cron year: %w", err)
	}

	// Search forward minute-by-minute for up to ~2 years of lab fidelity.
	t := from.Truncate(time.Minute).Add(time.Minute)
	limit := from.Add(730 * 24 * time.Hour)
	for !t.After(limit) {
		if cronFieldMatches(minute, t.Minute(), 0, 59, false) &&
			cronFieldMatches(hour, t.Hour(), 0, 23, false) &&
			cronNamedFieldMatches(month, int(t.Month()), 1, 12, false, cronMonthNames) &&
			cronFieldMatches(year, t.Year(), 1970, 2199, false) &&
			cronDayMatches(dom, dow, t) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no matching cron fire time within search window")
}

func validateCronField(field string, min, max int, allowQ bool) error {
	if allowQ && min == 1 && max == 31 {
		return validateCronDOMField(field)
	}
	if allowQ && min == 1 && max == 7 {
		return validateCronDOWField(field)
	}
	if min == 1 && max == 12 {
		return validateCronNamedField(field, min, max, allowQ, cronMonthNames)
	}
	return validateCronNamedField(field, min, max, allowQ, nil)
}

func validateCronNamedField(field string, min, max int, allowQ bool, names map[string]int) error {
	field = strings.TrimSpace(field)
	if field == "" {
		return fmt.Errorf("unsupported token %q", field)
	}
	for _, part := range strings.Split(field, ",") {
		if err := validateCronSegment(strings.TrimSpace(part), min, max, allowQ, names); err != nil {
			return err
		}
	}
	return nil
}

func validateCronDOMField(field string) error {
	field = strings.TrimSpace(field)
	if field == "" {
		return fmt.Errorf("unsupported token %q", field)
	}
	parts := strings.Split(field, ",")
	for _, part := range parts {
		seg := strings.TrimSpace(part)
		if isCronNearestWeekdaySegment(seg) || isCronLastWeekdayOfMonthSegment(seg) {
			// W / LW apply to a single DOM token only (not lists or ranges).
			if len(parts) > 1 {
				return fmt.Errorf("unsupported token %q", field)
			}
			if isCronLastWeekdayOfMonthSegment(seg) {
				continue
			}
			day, ok := parseCronNearestWeekdayDay(seg)
			if !ok || day < 1 || day > 31 {
				return fmt.Errorf("unsupported token %q", seg)
			}
			continue
		}
		if strings.EqualFold(seg, "L") {
			continue
		}
		if err := validateCronSegment(seg, 1, 31, true, nil); err != nil {
			return err
		}
	}
	return nil
}

func isCronLastWeekdayOfMonthSegment(seg string) bool {
	return strings.EqualFold(strings.TrimSpace(seg), "LW")
}

func isCronNearestWeekdaySegment(seg string) bool {
	seg = strings.TrimSpace(seg)
	if len(seg) < 2 {
		return false
	}
	if !strings.EqualFold(seg[len(seg)-1:], "W") {
		return false
	}
	base := seg[:len(seg)-1]
	if base == "" || strings.ContainsAny(base, "-*/,?#") {
		return false
	}
	_, err := strconv.Atoi(base)
	return err == nil
}

func parseCronNearestWeekdayDay(seg string) (int, bool) {
	seg = strings.TrimSpace(seg)
	if !isCronNearestWeekdaySegment(seg) {
		return 0, false
	}
	n, err := strconv.Atoi(seg[:len(seg)-1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func validateCronDOWField(field string) error {
	field = strings.TrimSpace(field)
	if field == "" {
		return fmt.Errorf("unsupported token %q", field)
	}
	parts := strings.Split(field, ",")
	hashCount := 0
	for _, part := range parts {
		seg := strings.TrimSpace(part)
		if strings.Contains(seg, "#") {
			hashCount++
			if err := validateCronHashSegment(seg); err != nil {
				return err
			}
			continue
		}
		if isCronLastWeekdaySegment(seg) {
			continue
		}
		if err := validateCronSegment(seg, 1, 7, true, cronDOWNames); err != nil {
			return err
		}
	}
	if hashCount > 1 {
		return fmt.Errorf("unsupported token %q: only one # expression allowed", field)
	}
	if hashCount > 0 && len(parts) > 1 {
		return fmt.Errorf("unsupported token %q: # cannot be combined in a list", field)
	}
	return nil
}

func validateCronHashSegment(seg string) error {
	dowPart, nth, ok := splitCronHash(seg)
	if !ok {
		return fmt.Errorf("unsupported token %q", seg)
	}
	if nth < 1 || nth > 5 {
		return fmt.Errorf("unsupported token %q", seg)
	}
	n, err := parseCronNamedInt(dowPart, 1, 7, cronDOWNames)
	if err != nil {
		return fmt.Errorf("unsupported token %q", seg)
	}
	_ = n
	return nil
}

func isCronLastWeekdaySegment(seg string) bool {
	seg = strings.ToUpper(strings.TrimSpace(seg))
	if seg == "L" {
		return true // last day of week (Saturday=7) — rarely used alone
	}
	if strings.HasSuffix(seg, "L") && !strings.Contains(seg, "#") {
		base := strings.TrimSuffix(seg, "L")
		_, err := parseCronNamedInt(base, 1, 7, cronDOWNames)
		return err == nil
	}
	return false
}

func validateCronSegment(seg string, min, max int, allowQ bool, names map[string]int) error {
	if seg == "*" {
		return nil
	}
	if allowQ && seg == "?" {
		return nil
	}
	if seg == "" {
		return fmt.Errorf("unsupported token %q", seg)
	}
	rangePart, step, hasStep, err := splitCronStep(seg)
	if err != nil {
		return err
	}
	if hasStep && step < 1 {
		return fmt.Errorf("unsupported token %q", seg)
	}
	if rangePart == "*" {
		return nil
	}
	start, end, err := parseCronRangeNamed(rangePart, min, max, names)
	if err != nil {
		return err
	}
	if hasStep && !strings.ContainsRune(rangePart, '-') {
		end = max
	}
	if start > end || start > max {
		return fmt.Errorf("out of range %d", start)
	}
	_ = end
	return nil
}

func cronFieldMatches(field string, value, min, max int, allowQ bool) bool {
	if min == 1 && max == 12 {
		return cronNamedFieldMatches(field, value, min, max, allowQ, cronMonthNames)
	}
	return cronNamedFieldMatches(field, value, min, max, allowQ, nil)
}

func cronNamedFieldMatches(field string, value, min, max int, allowQ bool, names map[string]int) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return false
	}
	for _, part := range strings.Split(field, ",") {
		if cronSegmentMatchesNamed(strings.TrimSpace(part), value, min, max, allowQ, names) {
			return true
		}
	}
	return false
}

func cronSegmentMatchesNamed(seg string, value, min, max int, allowQ bool, names map[string]int) bool {
	if seg == "*" {
		return true
	}
	if allowQ && seg == "?" {
		return true
	}
	if seg == "" {
		return false
	}
	rangePart, step, hasStep, err := splitCronStep(seg)
	if err != nil {
		return false
	}
	if !hasStep {
		step = 1
	} else if step < 1 {
		return false
	}
	if rangePart == "*" {
		start := min
		if value < start || value > max {
			return false
		}
		return (value-start)%step == 0
	}
	start, end, err := parseCronRangeNamed(rangePart, min, max, names)
	if err != nil {
		return false
	}
	if hasStep && !strings.ContainsRune(rangePart, '-') {
		end = max
	}
	if value < start || value > end {
		return false
	}
	if !hasStep {
		return true
	}
	return (value-start)%step == 0
}

func splitCronStep(seg string) (rangePart string, step int, hasStep bool, err error) {
	slash := strings.IndexByte(seg, '/')
	if slash < 0 {
		return seg, 0, false, nil
	}
	rangePart = seg[:slash]
	stepStr := seg[slash+1:]
	if stepStr == "" {
		return "", 0, false, fmt.Errorf("unsupported token %q", seg)
	}
	// AWS Secrets form "/8" means */8 (every N from field min).
	if rangePart == "" {
		rangePart = "*"
	}
	step, err = strconv.Atoi(stepStr)
	if err != nil {
		return "", 0, false, fmt.Errorf("unsupported token %q", seg)
	}
	return rangePart, step, true, nil
}

func parseCronRange(rangePart string, min, max int) (start, end int, err error) {
	return parseCronRangeNamed(rangePart, min, max, nil)
}

func parseCronRangeNamed(rangePart string, min, max int, names map[string]int) (start, end int, err error) {
	dash := strings.IndexByte(rangePart, '-')
	if dash < 0 {
		n, convErr := parseCronNamedInt(rangePart, min, max, names)
		if convErr != nil {
			return 0, 0, convErr
		}
		return n, n, nil
	}
	startStr, endStr := rangePart[:dash], rangePart[dash+1:]
	start, err = parseCronNamedInt(startStr, min, max, names)
	if err != nil {
		return 0, 0, fmt.Errorf("unsupported token %q", rangePart)
	}
	end, err = parseCronNamedInt(endStr, min, max, names)
	if err != nil {
		return 0, 0, fmt.Errorf("unsupported token %q", rangePart)
	}
	if start < min || end > max || start > end {
		return 0, 0, fmt.Errorf("out of range %d-%d", start, end)
	}
	return start, end, nil
}

func parseCronNamedInt(tok string, min, max int, names map[string]int) (int, error) {
	tok = strings.TrimSpace(tok)
	if names != nil {
		if n, ok := names[strings.ToUpper(tok)]; ok {
			if n < min || n > max {
				return 0, fmt.Errorf("out of range %d", n)
			}
			return n, nil
		}
	}
	n, err := strconv.Atoi(tok)
	if err != nil {
		return 0, fmt.Errorf("unsupported token %q", tok)
	}
	if n < min || n > max {
		return 0, fmt.Errorf("out of range %d", n)
	}
	return n, nil
}

func splitCronHash(seg string) (dowPart string, nth int, ok bool) {
	hash := strings.IndexByte(seg, '#')
	if hash < 0 {
		return "", 0, false
	}
	dowPart = seg[:hash]
	nthStr := seg[hash+1:]
	if dowPart == "" || nthStr == "" {
		return "", 0, false
	}
	nth, err := strconv.Atoi(nthStr)
	if err != nil {
		return "", 0, false
	}
	return dowPart, nth, true
}

func lastDayOfMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func awsDow(t time.Time) int {
	// AWS cron: Sunday=1 ... Saturday=7. Go Weekday: Sunday=0.
	return int(t.Weekday()) + 1
}

func cronDOMMatches(dom string, t time.Time) bool {
	dom = strings.TrimSpace(dom)
	if dom == "" {
		return false
	}
	for _, part := range strings.Split(dom, ",") {
		seg := strings.TrimSpace(part)
		if isCronLastWeekdayOfMonthSegment(seg) {
			lw := lastWeekdayOfMonth(t)
			if t.Year() == lw.Year() && t.Month() == lw.Month() && t.Day() == lw.Day() {
				return true
			}
			continue
		}
		if day, ok := parseCronNearestWeekdayDay(seg); ok {
			nw := nearestWeekdayOfMonth(t.Year(), int(t.Month()), day)
			if t.Year() == nw.Year() && t.Month() == nw.Month() && t.Day() == nw.Day() {
				return true
			}
			continue
		}
		if strings.EqualFold(seg, "L") {
			if t.Day() == lastDayOfMonth(t) {
				return true
			}
			continue
		}
		if cronSegmentMatchesNamed(seg, t.Day(), 1, 31, true, nil) {
			return true
		}
	}
	return false
}

// nearestWeekdayOfMonth returns the weekday (Mon–Fri) closest to day in the
// given month. Saturday maps to Friday and Sunday to Monday, without crossing
// the month boundary (e.g. 1W when the 1st is Saturday → Monday the 3rd).
func nearestWeekdayOfMonth(year, month, day int) time.Time {
	last := lastDayOfMonth(time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC))
	if day > last {
		day = last
	}
	if day < 1 {
		day = 1
	}
	d := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	switch d.Weekday() {
	case time.Saturday:
		prev := d.AddDate(0, 0, -1)
		if prev.Month() != d.Month() {
			return d.AddDate(0, 0, 2) // e.g. 1W on Saturday → Monday the 3rd
		}
		return prev
	case time.Sunday:
		next := d.AddDate(0, 0, 1)
		if next.Month() != d.Month() {
			return d.AddDate(0, 0, -2) // Friday stays in-month
		}
		return next
	default:
		return d
	}
}

// lastWeekdayOfMonth returns the last Monday–Friday date in t's month.
func lastWeekdayOfMonth(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), lastDayOfMonth(t), 0, 0, 0, 0, time.UTC)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

func cronDOWMatches(dow string, t time.Time) bool {
	dow = strings.TrimSpace(dow)
	if dow == "" {
		return false
	}
	if strings.Contains(dow, "#") {
		dowPart, nth, ok := splitCronHash(dow)
		if !ok {
			return false
		}
		want, err := parseCronNamedInt(dowPart, 1, 7, cronDOWNames)
		if err != nil {
			return false
		}
		return isNthWeekdayOfMonth(t, want, nth)
	}
	for _, part := range strings.Split(dow, ",") {
		seg := strings.TrimSpace(part)
		upper := strings.ToUpper(seg)
		if upper == "L" {
			if awsDow(t) == 7 {
				return true
			}
			continue
		}
		if strings.HasSuffix(upper, "L") {
			base := strings.TrimSuffix(upper, "L")
			want, err := parseCronNamedInt(base, 1, 7, cronDOWNames)
			if err != nil {
				continue
			}
			if isLastWeekdayOfMonth(t, want) {
				return true
			}
			continue
		}
		if cronSegmentMatchesNamed(seg, awsDow(t), 1, 7, true, cronDOWNames) {
			return true
		}
	}
	return false
}

func isNthWeekdayOfMonth(t time.Time, awsWeekday, nth int) bool {
	if awsDow(t) != awsWeekday {
		return false
	}
	occurrence := (t.Day()-1)/7 + 1
	return occurrence == nth
}

func isLastWeekdayOfMonth(t time.Time, awsWeekday int) bool {
	if awsDow(t) != awsWeekday {
		return false
	}
	next := t.AddDate(0, 0, 7)
	return next.Month() != t.Month()
}

func cronDayMatches(dom, dow string, t time.Time) bool {
	dom = strings.TrimSpace(dom)
	dow = strings.TrimSpace(dow)
	domStarOrQ := dom == "*" || dom == "?"
	dowStarOrQ := dow == "*" || dow == "?"

	domOK := cronDOMMatches(dom, t)
	dowOK := cronDOWMatches(dow, t)

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
