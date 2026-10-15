package rule

import (
	"testing"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"

	"github.com/stretchr/testify/require"
)

// The rule-set a connection matched is kept for the matched_ruleset hash keys
// of loadbalance, and only when the rule-set actually matched.
func TestRuleSetMatchRecordsTag(t *testing.T) {
	t.Parallel()
	metadata := &adapter.InboundContext{}
	require.True(t, newRuleSetOnlyRule(true, false).Match(metadata))
	require.Equal(t, "fake-rule-set", metadata.MatchedRuleSetTag)

	metadata = &adapter.InboundContext{}
	require.False(t, newRuleSetOnlyRule(false, false).Match(metadata))
	require.Empty(t, metadata.MatchedRuleSetTag)
}

func TestLogicalRuleRecordsNestedRuleSetTag(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{C.LogicalTypeAnd, C.LogicalTypeOr} {
		logicalRule := &abstractLogicalRule{
			mode: mode,
			rules: []adapter.HeadlessRule{
				newSingleItemRule(true),
				newRuleSetOnlyRule(true, false),
			},
		}
		metadata := &adapter.InboundContext{}
		require.True(t, logicalRule.Match(metadata), mode)
		if mode == C.LogicalTypeAnd {
			require.Equal(t, "fake-rule-set", metadata.MatchedRuleSetTag, mode)
		}
	}
}

func TestResetRuleCacheClearsMatchedRuleSetTag(t *testing.T) {
	t.Parallel()
	metadata := &adapter.InboundContext{MatchedRuleSetTag: "stale"}
	metadata.ResetRuleCache()
	require.Empty(t, metadata.MatchedRuleSetTag)
}
