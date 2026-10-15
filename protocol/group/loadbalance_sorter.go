package group

import (
	"math"
	"sort"
	"time"

	"github.com/sagernet/sing-box/common/transportstats"
	E "github.com/sagernet/sing/common/exceptions"
)

// MetricLatency is the sorter key used when no sorter is configured, which
// reproduces the plain latency ranking.
const MetricLatency = "latency"

// metricSource is everything a sorter key can be read from for one member.
type metricSource struct {
	// latency is the member's most recent health check delay in milliseconds.
	latency uint16
	// latencyWindow holds the member's health check history, and is nil when no
	// configured key needs it.
	latencyWindow *transportstats.Recorder
}

// metricDefinition describes one sorter key.
//
//nolint:govet // field order here follows the documentation, not packing.
type metricDefinition struct {
	// window is the span the key averages over, and is zero for keys that read
	// a single latest value.
	window time.Duration
	// higherIsBetter inverts the contribution, so that every key can be
	// configured with a positive weight and a lower score always ranks better.
	higherIsBetter bool
	// needsLatencyWindow marks the keys that read the health check history.
	needsLatencyWindow bool
	read               func(source metricSource) (float64, bool)
}

func readLatest(source metricSource) (float64, bool) {
	if source.latency == 0 {
		return 0, false
	}
	return float64(source.latency), true
}

func readLatencyWindow(window time.Duration) func(metricSource) (float64, bool) {
	return func(source metricSource) (float64, bool) {
		if source.latencyWindow == nil {
			return readLatest(source)
		}
		if average, ok := source.latencyWindow.Latency(window); ok {
			return average, true
		}
		// A window with no sample in it falls back to the latest probe, so that
		// a freshly started member is ranked rather than treated as missing.
		return readLatest(source)
	}
}

// metricDefinitions is the closed set of sorter keys. Keys are fixed rather
// than parsed from a metric and window pattern, so that a typo is a
// configuration error instead of a silently missing metric.
var metricDefinitions = map[string]metricDefinition{
	MetricLatency:    {read: readLatest},
	"latency_avg_1m": {needsLatencyWindow: true, window: time.Minute, read: readLatencyWindow(time.Minute)},
	"latency_avg_5m": {needsLatencyWindow: true, window: 5 * time.Minute, read: readLatencyWindow(5 * time.Minute)},
}

// MetricKeys returns every supported sorter key, sorted, for error messages and
// documentation checks.
func MetricKeys() []string {
	keys := make([]string, 0, len(metricDefinitions))
	for key := range metricDefinitions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// CheckMetricKey reports whether key is a supported sorter key.
func CheckMetricKey(key string) error {
	if _, loaded := metricDefinitions[key]; !loaded {
		return E.New("unsupported sorter key: ", key)
	}
	return nil
}

// sorterKey is one configured key with its weight, resolved once at startup.
type sorterKey struct {
	name       string
	weight     float64
	definition metricDefinition
}

// sorter turns a member's metrics into a single millisecond equivalent score,
// where a lower score ranks better.
//
// Each weight says how many milliseconds one unit of its metric is worth, so
// scores stay comparable with the raw latency they replace and the group's
// tolerance keeps its meaning. A member's score does not depend on the rest of
// the pool, except for metrics it is missing: those are substituted with the
// pool average so that a member is neither rewarded nor punished for a metric
// its protocol cannot report.
type sorter struct {
	keys []sorterKey
	// usesLatencyWindow is true when any key reads the health check history.
	usesLatencyWindow bool
	// narrowestLatencyWindow is the shortest span any latency key averages
	// over, and is zero when no key reads the history. A probe interval more
	// than half as long leaves too few samples in it for a meaningful average.
	narrowestLatencyWindow time.Duration
	// isDefault is true for the implicit latency only sorter.
	isDefault bool
}

// latencyOnlySorter ranks purely on the latest health check delay. A sorter is
// immutable once built, so the one instance is shared by every group that does
// not configure one.
var latencyOnlySorter = defaultSorter()

// defaultSorter ranks purely on the latest health check delay, which is what a
// group without a sorter does.
func defaultSorter() *sorter {
	s, err := newSorter(map[string]float64{MetricLatency: 1})
	if err != nil {
		panic(err)
	}
	s.isDefault = true
	return s
}

func newSorter(weights map[string]float64) (*sorter, error) {
	if len(weights) == 0 {
		return defaultSorter(), nil
	}
	names := make([]string, 0, len(weights))
	for name := range weights {
		names = append(names, name)
	}
	// Sort so that scoring, logging and tests are deterministic.
	sort.Strings(names)
	s := &sorter{keys: make([]sorterKey, 0, len(names))}
	for _, name := range names {
		definition, loaded := metricDefinitions[name]
		if !loaded {
			return nil, E.New("unsupported sorter key: ", name)
		}
		weight := weights[name]
		if math.IsNaN(weight) || math.IsInf(weight, 0) {
			return nil, E.New("sorter key ", name, " has a non-finite weight")
		}
		if weight < 0 {
			return nil, E.New("sorter key ", name, " has a negative weight; use a key whose direction already matches instead")
		}
		if weight == 0 {
			continue
		}
		s.keys = append(s.keys, sorterKey{name: name, weight: weight, definition: definition})
		if definition.needsLatencyWindow {
			s.usesLatencyWindow = true
			if s.narrowestLatencyWindow == 0 || definition.window < s.narrowestLatencyWindow {
				s.narrowestLatencyWindow = definition.window
			}
		}
	}
	if len(s.keys) == 0 {
		return nil, E.New("sorter has no key with a positive weight")
	}
	return s, nil
}

// contribution is one key's part of a member's score.
type contribution struct {
	Key string
	// Value is what was scored: the member's own metric, or the pool average
	// when Measured is false.
	Value    float64
	Measured bool
	// Weighted is the signed amount this key added to the score.
	Weighted float64
}

// breakdown explains one member's score.
type breakdown struct {
	Tag           string
	Score         float64
	Contributions []contribution
}

// score fills in Score on every candidate and returns the per member
// breakdowns in the same order, for debug logging.
//
// It reads every key for every member first, so that a member missing a metric
// can be scored against the pool average for that metric.
func (s *sorter) score(candidates []Candidate, source func(candidate Candidate) metricSource) []breakdown {
	if len(candidates) == 0 {
		return nil
	}
	values := make([][]float64, len(candidates))
	measured := make([][]bool, len(candidates))
	for i := range candidates {
		values[i] = make([]float64, len(s.keys))
		measured[i] = make([]bool, len(s.keys))
		memberSource := source(candidates[i])
		for j, key := range s.keys {
			value, ok := key.definition.read(memberSource)
			if ok && (math.IsNaN(value) || math.IsInf(value, 0)) {
				ok = false
			}
			values[i][j] = value
			measured[i][j] = ok
		}
	}
	// A key no member could measure contributes the same zero to every score,
	// and a member missing a key takes the pool average for it. Either way the
	// members it cannot tell apart end up with equal scores, which
	// sortCandidatesByScore then breaks on the raw delay.
	averages := make([]float64, len(s.keys))
	for j := range s.keys {
		var sum float64
		var count int
		for i := range candidates {
			if measured[i][j] {
				sum += values[i][j]
				count++
			}
		}
		if count > 0 {
			averages[j] = sum / float64(count)
		}
	}
	breakdowns := make([]breakdown, len(candidates))
	for i := range candidates {
		var score float64
		contributions := make([]contribution, 0, len(s.keys))
		for j, key := range s.keys {
			value := values[i][j]
			if !measured[i][j] {
				value = averages[j]
			}
			weighted := key.weight * value
			if key.definition.higherIsBetter {
				weighted = -weighted
			}
			score += weighted
			contributions = append(contributions, contribution{
				Key:      key.name,
				Value:    value,
				Measured: measured[i][j],
				Weighted: weighted,
			})
		}
		if math.IsNaN(score) {
			score = math.MaxFloat64
		}
		candidates[i].Score = score
		breakdowns[i] = breakdown{Tag: candidates[i].Tag, Score: score, Contributions: contributions}
	}
	return breakdowns
}
