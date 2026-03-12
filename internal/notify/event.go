// Package notify provides a pluggable notification system for build events.
package notify

import (
	"fmt"
	"time"
)

// FailedStep contains info about a failed build step for notification emails.
type FailedStep struct {
	Name     string
	LogLines []string // last N lines of log output
}

// BuildEvent contains the information sent in a build notification.
type BuildEvent struct {
	ProjectName   string
	BuildNumber   int
	Status        string // "success", "failure", or "cancelled"
	Branch        string
	CommitSHA     string
	CommitMessage string
	CommitAuthor  string
	Duration      time.Duration
	BuildURL      string
	ProjectURL    string       // Link to the project page in FeatherCI
	CommitURL     string       // Link to view the commit on the git provider
	FailedSteps   []FailedStep // Steps that failed, with log tails
}

// ShortSHA returns the first 8 characters of the commit SHA.
func (e BuildEvent) ShortSHA() string {
	if len(e.CommitSHA) > 8 {
		return e.CommitSHA[:8]
	}
	return e.CommitSHA
}

// DurationString returns a human-readable duration.
func (e BuildEvent) DurationString() string {
	if e.Duration < time.Second {
		return "< 1s"
	}
	if e.Duration < time.Minute {
		return e.Duration.Round(time.Second).String()
	}
	return e.Duration.Round(time.Second).String()
}

// IsSuccess returns true if the build succeeded.
func (e BuildEvent) IsSuccess() bool {
	return e.Status == "success"
}

// IsCancelled returns true if the build was cancelled.
func (e BuildEvent) IsCancelled() bool {
	return e.Status == "cancelled"
}

// StatusLabel returns a human-friendly label for the build status.
func (e BuildEvent) StatusLabel() string {
	switch e.Status {
	case "success":
		return "Passed"
	case "cancelled":
		return "Cancelled"
	default:
		return "Failed"
	}
}

// EmailSubject returns a consistent email subject line.
func (e BuildEvent) EmailSubject() string {
	return fmt.Sprintf("%s %s #%d %s in FeatherCI",
		e.StatusEmoji(), e.ProjectName, e.BuildNumber, e.StatusLabel())
}

// StatusEmoji returns an emoji for the build status.
func (e BuildEvent) StatusEmoji() string {
	switch e.Status {
	case "success":
		return "\u2705" // green check
	case "cancelled":
		return "\U0001F6AB" // prohibited sign
	default:
		return "\u274C" // red X
	}
}
