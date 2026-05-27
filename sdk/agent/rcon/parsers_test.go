package rcon

import "testing"

func TestParseMinecraftPlayerCount(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		online   int
		capacity int
	}{
		{
			name:     "standard format",
			output:   "There are 5 of a max 20 players online: player1, player2",
			online:   5,
			capacity: 20,
		},
		{
			name:     "fallback parsing",
			output:   "There are 3 of a max 10 players online",
			online:   3,
			capacity: 10,
		},
		{
			name:     "zero players",
			output:   "There are 0 of a max 20 players online",
			online:   0,
			capacity: 20,
		},
		{
			name:     "invalid format",
			output:   "some random output",
			online:   0,
			capacity: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, c := parseMinecraftPlayerCount(tt.output)
			if o != tt.online || c != tt.capacity {
				t.Fatalf("expected online=%d capacity=%d, got online=%d capacity=%d", tt.online, tt.capacity, o, c)
			}
		})
	}
}

func TestParseSourcePlayerCount(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		online   int
		capacity int
	}{
		{
			name:     "standard format",
			output:   "hostname: Test Server\nplayers : 5 (20 max)\n# userid name",
			online:   5,
			capacity: 20,
		},
		{
			name:     "player list only",
			output:   "# 1 \"player1\"\n# 2 \"player2\"",
			online:   2,
			capacity: 0,
		},
		{
			name:     "empty",
			output:   "",
			online:   0,
			capacity: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, c := parseSourcePlayerCount(tt.output)
			if o != tt.online || c != tt.capacity {
				t.Fatalf("expected online=%d capacity=%d, got online=%d capacity=%d", tt.online, tt.capacity, o, c)
			}
		})
	}
}

func TestParseSourcePlayerList(t *testing.T) {
	output := "# userid name\n# 1 \"player1\"\n# 2 \"player2\""
	players := parseSourcePlayerList(output)
	if len(players) != 2 {
		t.Fatalf("expected 2 players, got %d", len(players))
	}
	if players[0] != "\"player1\"" || players[1] != "\"player2\"" {
		t.Fatalf("expected \"player1\", \"player2\", got %v", players)
	}
}

func TestParsePalworldPlayerCount(t *testing.T) {
	output := "name,playeruid,steamid\nPlayer1,123,456\nPlayer2,789,012"
	online, capacity := parsePalworldPlayerCount(output)
	if online != 2 {
		t.Fatalf("expected online 2, got %d", online)
	}
	if capacity != 0 {
		t.Fatalf("expected capacity 0, got %d", capacity)
	}
}

func TestParsePalworldPlayerList(t *testing.T) {
	output := "name,playeruid,steamid\nPlayer1,123,456\nPlayer2,789,012"
	players := parsePalworldPlayerList(output)
	if len(players) != 2 {
		t.Fatalf("expected 2 players, got %d", len(players))
	}
	if players[0] != "Player1" || players[1] != "Player2" {
		t.Fatalf("expected Player1, Player2, got %v", players)
	}
}

func TestParsePalworldPlayerCountFromInfo(t *testing.T) {
	output := "Online Players: 5\nMax Players: 20"
	online, capacity := parsePalworldPlayerCountFromInfo(output)
	if online != 5 || capacity != 20 {
		t.Fatalf("expected online=5 capacity=20, got online=%d capacity=%d", online, capacity)
	}
}

func TestFormatPalworldBroadcast(t *testing.T) {
	result := formatPalworldBroadcast("hello world")
	if result != "hello_world" {
		t.Fatalf("expected hello_world, got %q", result)
	}
}

func TestMinecraftDialer(t *testing.T) {
	// Test that MinecraftDialer implements Dialer interface
	var _ Dialer = (*MinecraftDialer)(nil)
}

func TestSourceDialer(t *testing.T) {
	// Test that SourceDialer implements Dialer interface
	var _ Dialer = (*SourceDialer)(nil)
}

func TestPalworldDialer(t *testing.T) {
	// Test that PalworldDialer implements Dialer interface
	var _ Dialer = (*PalworldDialer)(nil)
}
