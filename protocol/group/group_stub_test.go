package group

import (
	"context"
	"errors"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
)

// groupStub is a nested group outbound that resolves to one of its members.
type groupStub struct {
	seedStub
	members []string
	current string
}

func (g *groupStub) Now() string   { return g.current }
func (g *groupStub) All() []string { return g.members }
func (g *groupStub) Type() string  { return "selector" }

// stubManager is the smallest OutboundManager that resolves tags.
type stubManager struct {
	outbounds map[string]adapter.Outbound
}

func (m *stubManager) Start(adapter.StartStage) error { return nil }
func (m *stubManager) Close() error                   { return nil }
func (m *stubManager) Outbounds() []adapter.Outbound  { return nil }
func (m *stubManager) Outbound(tag string) (adapter.Outbound, bool) {
	detour, loaded := m.outbounds[tag]
	return detour, loaded
}
func (m *stubManager) Default() adapter.Outbound { return nil }
func (m *stubManager) Remove(string) error       { return nil }
func (m *stubManager) Create(context.Context, adapter.Router, log.ContextLogger, string, string, any) error {
	return errors.New("unused")
}
