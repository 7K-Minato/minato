package testing

import "github.com/7k-group/minami/sdk/agent/rcon"

type FakeAgentEnv struct {
	RCON *rcon.MockClient
}

func NewFakeAgentEnv() *FakeAgentEnv {
	return &FakeAgentEnv{RCON: &rcon.MockClient{}}
}
