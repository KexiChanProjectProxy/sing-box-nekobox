package anytls

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/anytls/sing-anytls/session"
)

// logSelectedSession reports, at debug level, which session a new connection
// was placed on and what the pool knows about it.
func (h *Outbound) logSelectedSession(ctx context.Context, info session.SessionInfo) {
	h.logger.DebugContext(ctx, "anytls session selected: ", strings.Join(sessionFields(info, time.Now()), " "))
}

// sessionFields describes a selected session as key=value pairs. Durations are
// in milliseconds and times in UTC. Latency fields are left out until the
// session has been measured, and lifetime fields when max_connection_lifetime
// is not set; lifetime_left is negative for an expired session that age
// cleanup has not closed yet. retire_at and retire_left appear with
// close_after_lifetime.
func sessionFields(info session.SessionInfo, now time.Time) []string {
	source := "new"
	if info.Reused {
		source = "pool"
	}
	fields := make([]string, 0, 16)
	fields = append(fields,
		"session="+strconv.FormatUint(info.Seq, 10),
		"source="+source,
		"streams="+strconv.FormatUint(info.Streams, 10),
		"created_at="+formatTime(info.CreatedAt),
		"age="+formatDuration(info.Age(now)),
	)
	if info.Reused {
		fields = append(fields, "idle_for="+formatDuration(info.IdleFor))
	}
	fields = append(fields, "latency_samples="+strconv.FormatUint(uint64(info.Latency.Samples), 10))
	if info.Latency.Samples > 0 {
		fields = append(fields,
			"latency="+formatDuration(info.Latency.Smoothed),
			"latency_last="+formatDuration(info.Latency.Last),
		)
	}
	if left, limited := info.LifetimeLeft(now); limited {
		fields = append(fields,
			"max_lifetime="+formatDuration(info.MaxLifetime),
			"expires_at="+formatTime(info.ExpiresAt),
			"lifetime_left="+formatDuration(left),
		)
	}
	if !info.RetireAt.IsZero() {
		fields = append(fields,
			"retire_at="+formatTime(info.RetireAt),
			"retire_left="+formatDuration(info.RetireAt.Sub(now)),
		)
	}
	fields = append(fields,
		"idle_sessions="+strconv.Itoa(info.IdleSessions),
		"total_sessions="+strconv.Itoa(info.TotalSessions),
	)
	return fields
}

func formatDuration(d time.Duration) string {
	return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', -1, 64) + "ms"
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
