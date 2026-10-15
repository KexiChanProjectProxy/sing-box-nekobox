package group

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/sagernet/sing-box/common/transportstats"
	"github.com/sagernet/sing-box/option"

	"github.com/stretchr/testify/require"
)

func scoreWith(t *testing.T, weights map[string]float64, sources map[string]metricSource, tags ...string) []Candidate {
	t.Helper()
	memberSorter, err := newSorter(weights)
	require.NoError(t, err)
	candidates := make([]Candidate, 0, len(tags))
	for _, tag := range tags {
		candidates = append(candidates, Candidate{Tag: tag, Latency: sources[tag].latency})
	}
	memberSorter.score(candidates, func(candidate Candidate) metricSource {
		return sources[candidate.Tag]
	})
	return candidates
}

func TestDefaultSorterScoresRawLatency(t *testing.T) {
	t.Parallel()
	candidates := []Candidate{{Tag: "a", Latency: 120}, {Tag: "b", Latency: 30}}
	defaultSorter().score(candidates, func(candidate Candidate) metricSource {
		return metricSource{latency: candidate.Latency}
	})
	require.InDelta(t, 120, candidates[0].Score, 1e-9)
	require.InDelta(t, 30, candidates[1].Score, 1e-9)
}

// The implicit sorter must order members exactly as ranking on delay alone did.
func TestDefaultSorterOrdersLikeLatency(t *testing.T) {
	t.Parallel()
	delays := []uint16{700, 1, 65535, 42, 42, 300}
	candidates := make([]Candidate, 0, len(delays))
	for i, delay := range delays {
		candidates = append(candidates, Candidate{Tag: string(rune('a' + i)), Latency: delay})
	}
	defaultSorter().score(candidates, func(candidate Candidate) metricSource {
		return metricSource{latency: candidate.Latency}
	})
	byScore := append([]Candidate(nil), candidates...)
	sortCandidatesByScore(byScore)
	byLatency := append([]Candidate(nil), candidates...)
	sortCandidatesByLatency(byLatency)
	require.Equal(t, byLatency, byScore)
}

func TestSorterWeightsAreMillisecondEquivalent(t *testing.T) {
	t.Parallel()
	window := transportstats.NewRecorder()
	window.AddLatency(40)
	sources := map[string]metricSource{
		// The latest 50ms plus half of a 40ms one minute average.
		"a": {latency: 50, latencyWindow: window},
		// No history, so the average reads the latest 60ms.
		"b": {latency: 60},
	}
	got := scoreWith(t, map[string]float64{MetricLatency: 1, "latency_avg_1m": 0.5}, sources, "a", "b")
	require.InDelta(t, 70, got[0].Score, 1e-9)
	require.InDelta(t, 90, got[1].Score, 1e-9)
}

func TestSorterUnmeasuredLatencyIsMissing(t *testing.T) {
	t.Parallel()
	// A candidate with no delay at all should not drag the score to zero and
	// win the pool; it takes the pool average instead.
	sources := map[string]metricSource{
		"measured":   {latency: 200},
		"unmeasured": {latency: 0},
	}
	got := scoreWith(t, map[string]float64{MetricLatency: 1}, sources, "measured", "unmeasured")
	require.InDelta(t, 200, got[0].Score, 1e-9)
	require.InDelta(t, 200, got[1].Score, 1e-9)
}

func TestSorterLatencyWindowFallsBackToLatest(t *testing.T) {
	t.Parallel()
	recorder := transportstats.NewRecorder()
	sources := map[string]metricSource{
		// A window with history.
		"a": {latency: 999, latencyWindow: recorder},
		// A window that exists but holds nothing yet.
		"b": {latency: 40, latencyWindow: transportstats.NewRecorder()},
		// No window at all.
		"c": {latency: 70},
	}
	recorder.AddLatency(10)
	recorder.AddLatency(30)
	got := scoreWith(t, map[string]float64{"latency_avg_1m": 1}, sources, "a", "b", "c")
	require.InDelta(t, 20, got[0].Score, 1e-9, "window average wins over the latest sample")
	require.InDelta(t, 40, got[1].Score, 1e-9, "an empty window falls back to the latest sample")
	require.InDelta(t, 70, got[2].Score, 1e-9, "no window falls back to the latest sample")
}

func TestNewSorterRejectsBadInput(t *testing.T) {
	t.Parallel()
	for name, weights := range map[string]map[string]float64{
		"unknown key": {"latency_avg_2m": 1},
		"negative":    {MetricLatency: -1},
		"nan":         {MetricLatency: math.NaN()},
		"inf":         {MetricLatency: math.Inf(1)},
		"all zero":    {MetricLatency: 0, "latency_avg_1m": 0},
		"single zero": {MetricLatency: 0},
	} {
		_, err := newSorter(weights)
		require.Error(t, err, name)
	}
}

func TestNewSorterDropsZeroWeights(t *testing.T) {
	t.Parallel()
	memberSorter, err := newSorter(map[string]float64{MetricLatency: 1, "latency_avg_1m": 0})
	require.NoError(t, err)
	require.Len(t, memberSorter.keys, 1)
	require.False(t, memberSorter.usesLatencyWindow, "a zero weighted key must not keep history")
}

func TestNewSorterEmptyIsDefault(t *testing.T) {
	t.Parallel()
	memberSorter, err := newSorter(nil)
	require.NoError(t, err)
	require.True(t, memberSorter.isDefault)
	require.False(t, memberSorter.usesLatencyWindow)
}

func TestNewSorterDetectsNeeds(t *testing.T) {
	t.Parallel()
	memberSorter, err := newSorter(map[string]float64{
		MetricLatency:    1,
		"latency_avg_1m": 1,
		"latency_avg_5m": 1,
	})
	require.NoError(t, err)
	require.True(t, memberSorter.usesLatencyWindow)
	require.Equal(t, time.Minute, memberSorter.narrowestLatencyWindow)
}

func TestSorterKeyOrderIsDeterministic(t *testing.T) {
	t.Parallel()
	// Go randomises map iteration, so a sorter built twice from the same
	// weights must still read its keys in the same order.
	weights := map[string]float64{"latency_avg_5m": 1, MetricLatency: 2, "latency_avg_1m": 3}
	want := []string{MetricLatency, "latency_avg_1m", "latency_avg_5m"}
	for i := 0; i < 50; i++ {
		memberSorter, err := newSorter(weights)
		require.NoError(t, err)
		names := make([]string, 0, len(memberSorter.keys))
		for _, key := range memberSorter.keys {
			names = append(names, key.name)
			require.Equal(t, weights[key.name], key.weight)
		}
		require.Equal(t, want, names)
	}
}

func TestSorterBreakdownExplainsScore(t *testing.T) {
	t.Parallel()
	memberSorter, err := newSorter(map[string]float64{MetricLatency: 1, "latency_avg_1m": 1})
	require.NoError(t, err)
	window := transportstats.NewRecorder()
	window.AddLatency(40)
	candidates := []Candidate{{Tag: "a", Latency: 50}, {Tag: "b"}}
	sources := map[string]metricSource{
		"a": {latency: 50, latencyWindow: window},
		"b": {},
	}
	breakdowns := memberSorter.score(candidates, func(candidate Candidate) metricSource {
		return sources[candidate.Tag]
	})
	require.Len(t, breakdowns, 2)
	require.Equal(t, "a", breakdowns[0].Tag)
	require.InDelta(t, candidates[0].Score, breakdowns[0].Score, 1e-9)

	// Independently computed: 50ms of delay, plus a 40ms one minute average.
	contributions := map[string]float64{}
	for _, item := range breakdowns[0].Contributions {
		contributions[item.Key] = item.Weighted
	}
	require.InDelta(t, 50, contributions[MetricLatency], 1e-9)
	require.InDelta(t, 40, contributions["latency_avg_1m"], 1e-9)
	require.InDelta(t, 90, breakdowns[0].Score, 1e-9)

	byKey := map[string]contribution{}
	for _, item := range breakdowns[1].Contributions {
		byKey[item.Key] = item
	}
	require.False(t, byKey[MetricLatency].Measured, "b has no delay")
	require.False(t, byKey["latency_avg_1m"].Measured, "b's average is the pool estimate")
	require.InDelta(t, 40, byKey["latency_avg_1m"].Value, 1e-9)
}

func TestSorterScoreEmptyPool(t *testing.T) {
	t.Parallel()
	require.Nil(t, defaultSorter().score(nil, func(Candidate) metricSource { return metricSource{} }))
}

// option validates sorter keys without importing the ranking table, so the two
// lists have to be kept in step.
func TestSorterKeysMatchOptionKeys(t *testing.T) {
	t.Parallel()
	fromOptions := append([]string(nil), option.LoadBalanceSorterKeys...)
	sort.Strings(fromOptions)
	require.Equal(t, fromOptions, MetricKeys())
}

func TestCheckMetricKey(t *testing.T) {
	t.Parallel()
	for _, key := range option.LoadBalanceSorterKeys {
		require.NoError(t, CheckMetricKey(key), key)
	}
	require.Error(t, CheckMetricKey("latency_avg_2m"))
	require.Error(t, CheckMetricKey(""))
}
