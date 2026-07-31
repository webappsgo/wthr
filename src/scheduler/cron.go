package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule computes the next run time after a given reference time.
// AI.md PART 19: "Use Go's time/ticker - No external cron libraries required" -
// this file is the built-in replacement for the previously used robfig/cron/v3
// dependency, supporting the same schedule syntax the rest of the codebase
// already relies on: standard 5-field cron expressions ("0 2 * * *"),
// descriptors ("@yearly", "@monthly", "@weekly", "@daily", "@midnight",
// "@hourly"), and fixed intervals ("@every 5m").
type Schedule interface {
	// Next returns the next activation time after the given time.
	Next(t time.Time) time.Time
}

// everySchedule fires at a fixed interval after the reference time.
type everySchedule struct {
	interval time.Duration
}

func (e everySchedule) Next(t time.Time) time.Time {
	return t.Add(e.interval)
}

// fieldSet is a bitset over the valid values of a single cron field.
type fieldSet map[int]bool

// cronSchedule is a standard 5-field (minute hour dom month dow) schedule.
type cronSchedule struct {
	minute, hour, dom, month, dow fieldSet
}

// maxScanMinutes bounds how far into the future Next() will scan looking for
// a match, so a malformed field combination (e.g. Feb 30) can never spin
// forever - four years comfortably covers every real schedule in use.
const maxScanMinutes = 4 * 366 * 24 * 60

func (c cronSchedule) Next(t time.Time) time.Time {
	next := t.Truncate(time.Minute).Add(time.Minute)

	for i := 0; i < maxScanMinutes; i++ {
		if c.month[int(next.Month())] &&
			c.dom[next.Day()] &&
			c.dow[int(next.Weekday())] &&
			c.hour[next.Hour()] &&
			c.minute[next.Minute()] {
			return next
		}
		next = next.Add(time.Minute)
	}

	// Unreachable for any valid combination of standard cron fields.
	return time.Time{}
}

var descriptors = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

// parseSchedule parses a cron expression, "@every <duration>" interval, or
// one of the standard descriptors into a Schedule.
func parseSchedule(spec string) (Schedule, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty schedule")
	}

	if strings.HasPrefix(spec, "@every ") {
		durStr := strings.TrimSpace(strings.TrimPrefix(spec, "@every "))
		dur, err := time.ParseDuration(durStr)
		if err != nil {
			return nil, fmt.Errorf("invalid @every duration %q: %w", durStr, err)
		}
		if dur <= 0 {
			return nil, fmt.Errorf("invalid @every duration %q: must be positive", durStr)
		}
		return everySchedule{interval: dur}, nil
	}

	if strings.HasPrefix(spec, "@") {
		expanded, ok := descriptors[spec]
		if !ok {
			return nil, fmt.Errorf("unknown schedule descriptor %q", spec)
		}
		spec = expanded
	}

	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return nil, fmt.Errorf("invalid cron expression %q: want 5 fields (minute hour dom month dow), got %d", spec, len(fields))
	}

	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}
	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}
	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}
	dow, err := parseField(fields[4], 0, 7)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}
	// 7 is a common alias for Sunday (0) in cron implementations.
	if dow[7] {
		dow[0] = true
		delete(dow, 7)
	}

	return cronSchedule{minute: minute, hour: hour, dom: dom, month: month, dow: dow}, nil
}

// parseField parses a single cron field (e.g. "*", "*/15", "1-5", "1,3,5")
// into the set of values it matches, within [min, max].
func parseField(field string, min, max int) (fieldSet, error) {
	set := make(fieldSet)

	for _, part := range strings.Split(field, ",") {
		rangePart, step, err := splitStep(part)
		if err != nil {
			return nil, err
		}

		lo, hi := min, max
		if rangePart != "*" {
			bounds := strings.SplitN(rangePart, "-", 2)
			lo, err = strconv.Atoi(bounds[0])
			if err != nil {
				return nil, fmt.Errorf("invalid value %q", bounds[0])
			}
			hi = lo
			if len(bounds) == 2 {
				hi, err = strconv.Atoi(bounds[1])
				if err != nil {
					return nil, fmt.Errorf("invalid value %q", bounds[1])
				}
			}
		}

		if lo < min || hi > max || lo > hi {
			return nil, fmt.Errorf("value out of range [%d-%d]: %q", min, max, part)
		}

		for v := lo; v <= hi; v += step {
			set[v] = true
		}
	}

	if len(set) == 0 {
		return nil, fmt.Errorf("empty field %q", field)
	}

	return set, nil
}

// splitStep splits "range/step" into its range and step; step defaults to 1.
func splitStep(part string) (rangePart string, step int, err error) {
	pieces := strings.SplitN(part, "/", 2)
	rangePart = pieces[0]
	step = 1
	if len(pieces) == 2 {
		step, err = strconv.Atoi(pieces[1])
		if err != nil {
			return "", 0, fmt.Errorf("invalid step %q", pieces[1])
		}
		if step <= 0 {
			return "", 0, fmt.Errorf("invalid step %q: must be positive", pieces[1])
		}
	}
	return rangePart, step, nil
}
