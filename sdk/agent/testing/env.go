package testing

import "github.com/7k-group/minato/sdk/agent/rcon"

type FakeAgentEnv struct {
	RCON *rcon.MockClient
}

func NewFakeAgentEnv() *FakeAgentEnv {
	return &FakeAgentEnv{RCON: &rcon.MockClient{}}
}
