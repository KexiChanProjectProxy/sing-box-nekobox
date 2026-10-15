package option

import (
	"github.com/sagernet/sing/common/json/badoption"

	E "github.com/sagernet/sing/common/exceptions"
)

type SelectorOutboundOptions struct {
	Outbounds                 []string `json:"outbounds" reference:"outbound"`
	Default                   string   `json:"default,omitempty" reference:"outbound"`
	InterruptExistConnections bool     `json:"interrupt_exist_connections,omitempty"`
}

type URLTestOutboundOptions struct {
	Outbounds                 []string           `json:"outbounds" reference:"outbound"`
	URL                       string             `json:"url,omitempty"`
	Interval                  badoption.Duration `json:"interval,omitempty"`
	Tolerance                 uint16             `json:"tolerance,omitempty"`
	IdleTimeout               badoption.Duration `json:"idle_timeout,omitempty"`
	InterruptExistConnections bool               `json:"interrupt_exist_connections,omitempty"`
	LatencyMode               string             `json:"latency_mode,omitempty"`
}

// Values of LatencyMode in the urltest and loadbalance groups. URL test
// latency, the default, comes from periodic HTTP probes. Session latency is
// the live latency of the connection an outbound would use for a new dial,
// read at every dial, so the member is chosen per connection.
const (
	LatencyModeURLTest = "url_test"
	LatencyModeSession = "session"
)

// CheckLatencyMode rejects an unknown latency_mode.
func CheckLatencyMode(mode string) error {
	switch mode {
	case "", LatencyModeURLTest, LatencyModeSession:
		return nil
	default:
		return E.New("unsupported latency_mode: ", mode)
	}
}
