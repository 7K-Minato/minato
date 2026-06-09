package testing

import "github.com/7k-minato/minato/sdk/agent/rcon"

type FakeAgentEnv struct {
	RCON *rcon.MockClient
}

func NewFakeAgentEnv() *FakeAgentEnv {
	return &FakeAgentEnv{RCON: &rcon.MockClient{}}
}
