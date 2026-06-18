//go:build no_logs

package auditor

// CollectorStatus holds aggregated log collector status.
type CollectorStatus struct {
	Running      bool
	TrackedFiles int
	Entries      []LogFileEntry
}

// LogFileEntry represents a single tracked log entry for metrics.
type LogFileEntry struct {
	Path        string
	SourceType  string
	TailingMode string
	Offset      string
	Source      string
	Service    string
	Tags        map[string]string
}

// GetCollectorStatus returns empty status under no_logs build.
func GetCollectorStatus() CollectorStatus {
	return CollectorStatus{}
}
