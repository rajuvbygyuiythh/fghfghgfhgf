package browser

import (
	"context"
	"fmt"
	"strings"
)

// BanReport is the site's own suspension signal (/banned).
type BanReport struct {
	Banned    bool
	URL       string
	UserID    string
	BanID     string
	Reason    string
	Remaining string
}

func (r BanReport) Summary() string {
	parts := make([]string, 0, 3)
	if r.BanID != "" {
		parts = append(parts, "Ban ID: "+r.BanID)
	}
	if r.Reason != "" {
		parts = append(parts, "Reason: "+r.Reason)
	}
	if r.Remaining != "" {
		parts = append(parts, "Remaining: "+r.Remaining)
	}
	if len(parts) == 0 {
		return "account suspended"
	}
	return strings.Join(parts, " · ")
}

// BanChecker is implemented by backends that can report the site ban page.
type BanChecker interface {
	CheckBan(context.Context) (BanReport, error)
}

// BannedError stops a login/chat run and lets the caller delete the session.
type BannedError struct {
	Report BanReport
}

func (e *BannedError) Error() string {
	return "account banned: " + e.Report.Summary()
}

// ParseBanText extracts the readable fields from the /banned page text.
func ParseBanText(text, pageURL string) BanReport {
	report := BanReport{Banned: true, URL: pageURL}
	lines := strings.Split(text, "\n")
	valueAfter := func(label string) string {
		for i, line := range lines {
			if strings.TrimSpace(line) != label || i+1 >= len(lines) {
				continue
			}
			value := strings.TrimSpace(lines[i+1])
			if value != "" {
				return value
			}
		}
		return ""
	}
	report.BanID = valueAfter("Ban ID")
	report.Reason = valueAfter("Reason")
	report.Remaining = valueAfter("Remaining")
	if report.UserID == "" {
		// The current page prints "User ID: <id>" on one line.
		lower := strings.ToLower(text)
		if idx := strings.Index(lower, "user id"); idx >= 0 {
			rest := strings.TrimSpace(text[idx+len("user id"):])
			rest = strings.TrimLeft(rest, ": \t")
			if end := strings.IndexAny(rest, " \t\r\n"); end >= 0 {
				rest = rest[:end]
			}
			report.UserID = strings.TrimSpace(rest)
		}
	}
	if report.Reason == "" && strings.Contains(strings.ToLower(text), "account suspended") {
		report.Reason = "account suspended"
	}
	return report
}

func isBannedURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	path := trimmed
	if idx := strings.Index(trimmed, "://"); idx >= 0 {
		rest := trimmed[idx+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			path = rest[slash:]
		} else {
			path = "/"
		}
	} else if slash := strings.Index(trimmed, "/"); slash >= 0 {
		path = trimmed[slash:]
	} else if trimmed != "/banned" {
		path = "/"
	}
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		path = path[:idx]
	}
	return strings.EqualFold(strings.TrimRight(path, "/"), "/banned")
}

func (r BanReport) String() string {
	return fmt.Sprintf("banned=%t url=%s %s", r.Banned, r.URL, r.Summary())
}
