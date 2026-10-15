package anytls

import (
	"strings"
	"testing"
	"time"

	"github.com/anytls/sing-anytls/session"
	"github.com/stretchr/testify/require"
)

func fieldMap(fields []string) map[string]string {
	m := make(map[string]string, len(fields))
	for _, field := range fields {
		key, value, _ := strings.Cut(field, "=")
		m[key] = value
	}
	return m
}

func TestSessionFieldsReusedMeasuredLimited(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.FixedZone("test", -7*3600))
	created := now.Add(-90 * time.Second)
	fields := fieldMap(sessionFields(session.SessionInfo{
		Seq:           7,
		Reused:        true,
		IdleFor:       2 * time.Second,
		Latency:       session.Latency{Smoothed: 40 * time.Millisecond, Last: 50 * time.Millisecond, Samples: 3},
		Streams:       5,
		CreatedAt:     created,
		MaxLifetime:   2 * time.Minute,
		ExpiresAt:     created.Add(2 * time.Minute),
		RetireAt:      created.Add(100 * time.Second),
		IdleSessions:  2,
		TotalSessions: 4,
	}, now))
	require.Equal(t, "7", fields["session"])
	require.Equal(t, "pool", fields["source"])
	require.Equal(t, "5", fields["streams"])
	require.Equal(t, created.UTC().Format(time.RFC3339Nano), fields["created_at"])
	require.Equal(t, "90000ms", fields["age"])
	require.Equal(t, "2000ms", fields["idle_for"])
	require.Equal(t, "3", fields["latency_samples"])
	require.Equal(t, "40ms", fields["latency"])
	require.Equal(t, "50ms", fields["latency_last"])
	require.Equal(t, "120000ms", fields["max_lifetime"])
	require.Equal(t, created.Add(2*time.Minute).UTC().Format(time.RFC3339Nano), fields["expires_at"])
	require.Equal(t, "30000ms", fields["lifetime_left"])
	require.Equal(t, created.Add(100*time.Second).UTC().Format(time.RFC3339Nano), fields["retire_at"])
	require.Equal(t, "10000ms", fields["retire_left"])
	require.Equal(t, "2", fields["idle_sessions"])
	require.Equal(t, "4", fields["total_sessions"])
}

func TestSessionFieldsNewUnmeasuredUnlimited(t *testing.T) {
	now := time.Now()
	fields := fieldMap(sessionFields(session.SessionInfo{Seq: 1, Streams: 1, CreatedAt: now, TotalSessions: 1}, now))
	require.Equal(t, "new", fields["source"])
	require.Equal(t, "0", fields["latency_samples"])
	for _, key := range []string{"idle_for", "latency", "latency_last", "max_lifetime", "expires_at", "lifetime_left", "retire_at", "retire_left"} {
		require.NotContains(t, fields, key)
	}
}
