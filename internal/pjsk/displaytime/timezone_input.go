package displaytime

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
)

var (
	timeZoneNamesOnce sync.Once
	timeZoneNames     []string
)

var timeZoneAliases = map[string]string{
	"cst": "Asia/Shanghai",
	"gmt": "UTC",
	"hkt": "Asia/Hong_Kong",
	"jst": "Asia/Tokyo",
	"kst": "Asia/Seoul",
	"sgt": "Asia/Singapore",
	"ust": "UTC",
	"utc": "UTC",
	"pst": "America/Los_Angeles",
}

var preferredTimeZoneNames = []string{
	"UTC",
	"Asia/Shanghai",
	"Asia/Hong_Kong",
	"Asia/Taipei",
	"Asia/Singapore",
	"Asia/Tokyo",
	"Asia/Seoul",
	"Asia/Bangkok",
	"Asia/Jakarta",
	"Asia/Manila",
	"Asia/Kuala_Lumpur",
	"Asia/Dubai",
	"Asia/Kolkata",
	"Asia/Kathmandu",
	"Europe/London",
	"Europe/Paris",
	"Europe/Berlin",
	"Europe/Madrid",
	"Europe/Rome",
	"Europe/Moscow",
	"Europe/Istanbul",
	"America/Los_Angeles",
	"America/Denver",
	"America/Chicago",
	"America/New_York",
	"America/Toronto",
	"America/Phoenix",
	"America/Anchorage",
	"Pacific/Honolulu",
	"America/Sao_Paulo",
	"America/Argentina/Buenos_Aires",
	"Africa/Cairo",
	"Africa/Johannesburg",
	"Africa/Nairobi",
	"Australia/Perth",
	"Australia/Adelaide",
	"Australia/Brisbane",
	"Australia/Darwin",
	"Australia/Melbourne",
	"Australia/Sydney",
	"Pacific/Auckland",
}

var preferredTimeZoneRank = buildPreferredTimeZoneRank(preferredTimeZoneNames)

func ResolveUserTimeZoneInput(raw string) (string, []string, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return "", nil, fmt.Errorf("请提供时区名或偏移量")
	}

	if resolved, ok := resolveDirectTimeZoneName(input); ok {
		return resolved, nil, nil
	}

	offsetSeconds, ok := parseTimeZoneOffsetSeconds(input)
	if !ok {
		return "", nil, fmt.Errorf("找不到符合的时区: %q", input)
	}

	candidates := findTimeZonesByOffset(offsetSeconds, time.Now().UTC())
	switch len(candidates) {
	case 0:
		return "", nil, fmt.Errorf("找不到符合的时区: %q", input)
	case 1:
		return candidates[0], nil, nil
	default:
		return "", candidates, nil
	}
}

func resolveDirectTimeZoneName(raw string) (string, bool) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return "", false
	}

	if alias, ok := timeZoneAliases[strings.ToLower(input)]; ok {
		if _, err := time.LoadLocation(alias); err == nil {
			return alias, true
		}
	}

	if _, err := time.LoadLocation(input); err == nil {
		return input, true
	}

	for _, name := range listTimeZoneNames() {
		if strings.EqualFold(name, input) {
			if _, err := time.LoadLocation(name); err == nil {
				return name, true
			}
		}
	}
	return "", false
}

func parseTimeZoneOffsetSeconds(raw string) (int, bool) {
	input := strings.TrimSpace(strings.ToUpper(raw))
	if input == "" {
		return 0, false
	}

	if strings.HasPrefix(input, "UTC") || strings.HasPrefix(input, "GMT") {
		input = strings.TrimSpace(input[3:])
		if input == "" {
			return 0, true
		}
	}

	sign := 1
	switch {
	case strings.HasPrefix(input, "+"):
		input = strings.TrimSpace(input[1:])
	case strings.HasPrefix(input, "-"):
		input = strings.TrimSpace(input[1:])
		sign = -1
	}
	if input == "" {
		return 0, false
	}

	if strings.Contains(input, ":") {
		parts := strings.Split(input, ":")
		if len(parts) != 2 {
			return 0, false
		}
		hours, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || hours < 0 {
			return 0, false
		}
		minutes, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || minutes < 0 || minutes >= 60 {
			return 0, false
		}
		return sign * (hours*3600 + minutes*60), true
	}

	value, err := strconv.Atoi(input)
	if err != nil {
		return 0, false
	}
	if value <= 14 {
		return sign * value * 3600, true
	}
	return sign * value, true
}

func findTimeZonesByOffset(offsetSeconds int, ref time.Time) []string {
	names := listTimeZoneNames()
	matches := make([]string, 0, 8)
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		loc, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		_, offset := ref.In(loc).Zone()
		if offset != offsetSeconds {
			continue
		}
		seen[name] = struct{}{}
		matches = append(matches, name)
	}
	sort.Slice(matches, func(i, j int) bool {
		return compareTimeZoneName(matches[i], matches[j]) < 0
	})
	return matches
}

func listTimeZoneNames() []string {
	timeZoneNamesOnce.Do(func() {
		names := append([]string(nil), preferredTimeZoneNames...)
		for _, path := range []string{
			"/usr/share/zoneinfo/zone1970.tab",
			"/usr/share/zoneinfo/zone.tab",
		} {
			extra, err := loadTimeZoneNamesFromTab(path)
			if err != nil || len(extra) == 0 {
				continue
			}
			names = append(names, extra...)
		}
		timeZoneNames = uniqueStrings(names)
	})
	return timeZoneNames
}

func loadTimeZoneNamesFromTab(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	names := make([]string, 0, 256)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		name := strings.TrimSpace(fields[2])
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, errors.New("timezone tab did not contain any zone name")
	}
	return names, nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return compareTimeZoneName(out[i], out[j]) < 0
	})
	return out
}

func buildPreferredTimeZoneRank(names []string) map[string]int {
	rank := make(map[string]int, len(names))
	for i, name := range names {
		rank[name] = i
	}
	return rank
}

func compareTimeZoneName(left, right string) int {
	leftRank, leftPreferred := preferredTimeZoneRank[left]
	rightRank, rightPreferred := preferredTimeZoneRank[right]
	switch {
	case leftPreferred && rightPreferred && leftRank != rightRank:
		if leftRank < rightRank {
			return -1
		}
		return 1
	case leftPreferred:
		return -1
	case rightPreferred:
		return 1
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
