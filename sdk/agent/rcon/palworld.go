package rcon

import (
	"context"
	"strconv"
	"strings"
)

// PalworldClient is a wrapper around a generic Client for Palworld-specific operations.
type PalworldClient struct {
	client Client
}

// NewPalworldClient creates a new PalworldClient wrapping the given Client.
func NewPalworldClient(client Client) *PalworldClient {
	return &PalworldClient{client: client}
}

// Command delegates to the underlying client.
func (p *PalworldClient) Command(ctx context.Context, command string) (string, error) {
	return p.client.Command(ctx, command)
}

// Close delegates to the underlying client.
func (p *PalworldClient) Close() error {
	return p.client.Close()
}

// parsePalworldPlayerCount parses the output of Palworld's "ShowPlayers" command.
func parsePalworldPlayerCount(output string) (online, capacity int) {
	// Palworld ShowPlayers output typically has a header line and then one line per player
	// Header might be something like "name,playeruid,steamid"
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "name") {
			continue
		}
		online++
	}
	return online, capacity
}

// parsePalworldPlayerList parses player names from Palworld ShowPlayers output.
func parsePalworldPlayerList(output string) []string {
	var players []string
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "name") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) > 0 && fields[0] != "" {
			players = append(players, fields[0])
		}
	}
	return players
}

// parsePalworldPlayerCountFromInfo parses player count from Palworld "Info" command output.
func parsePalworldPlayerCountFromInfo(output string) (online, capacity int) {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "Online Players:"); ok {
			if v, err := strconv.Atoi(strings.TrimSpace(after)); err == nil {
				online = v
			}
		}
		if after, ok := strings.CutPrefix(line, "Max Players:"); ok {
			if v, err := strconv.Atoi(strings.TrimSpace(after)); err == nil {
				capacity = v
			}
		}
	}
	return online, capacity
}

// formatPalworldBroadcast formats a message for Palworld's Broadcast command.
// Palworld replaces spaces with underscores in broadcast messages.
func formatPalworldBroadcast(message string) string {
	return strings.ReplaceAll(message, " ", "_")
}
