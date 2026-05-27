package metrics

import "testing"

func TestPlayerCount(t *testing.T) {
	value := PlayerCount("minecraft", "server-1")
	expected := `minato_player_count{game="minecraft",server="server-1"}`
	if value != expected {
		t.Fatalf("expected %q, got %q", expected, value)
	}
}

func TestPlayerCountEmptyStrings(t *testing.T) {
	value := PlayerCount("", "")
	expected := `minato_player_count{game="",server=""}`
	if value != expected {
		t.Fatalf("expected %q, got %q", expected, value)
	}
}

func TestPlayerCountSpecialCharacters(t *testing.T) {
	value := PlayerCount("game-with-dash", "server_1.test")
	expected := `minato_player_count{game="game-with-dash",server="server_1.test"}`
	if value != expected {
		t.Fatalf("expected %q, got %q", expected, value)
	}
}

func TestActionDuration(t *testing.T) {
	value := ActionDuration("save", "minecraft")
	expected := `minato_action_duration_seconds{action="save",game="minecraft"}`
	if value != expected {
		t.Fatalf("expected %q, got %q", expected, value)
	}
}

func TestActionDurationEmptyStrings(t *testing.T) {
	value := ActionDuration("", "")
	expected := `minato_action_duration_seconds{action="",game=""}`
	if value != expected {
		t.Fatalf("expected %q, got %q", expected, value)
	}
}

func TestActionDurationSpecialCharacters(t *testing.T) {
	value := ActionDuration("backup-world", "palworld-v1.0")
	expected := `minato_action_duration_seconds{action="backup-world",game="palworld-v1.0"}`
	if value != expected {
		t.Fatalf("expected %q, got %q", expected, value)
	}
}

func TestPlayerCountLongNames(t *testing.T) {
	game := "very-long-game-name-that-exceeds-normal-lengths"
	server := "server-name-with-many-characters-and-numbers-12345"
	value := PlayerCount(game, server)
	expected := `minato_player_count{game="` + game + `",server="` + server + `"}`
	if value != expected {
		t.Fatalf("expected %q, got %q", expected, value)
	}
}

func TestActionDurationLongNames(t *testing.T) {
	action := "very-long-action-name-that-exceeds-normal-lengths"
	game := "game-with-extremely-long-name-for-testing"
	value := ActionDuration(action, game)
	expected := `minato_action_duration_seconds{action="` + action + `",game="` + game + `"}`
	if value != expected {
		t.Fatalf("expected %q, got %q", expected, value)
	}
}

func TestPlayerCountUnicode(t *testing.T) {
	value := PlayerCount("マインクラフト", "サーバー1")
	expected := `minato_player_count{game="マインクラフト",server="サーバー1"}`
	if value != expected {
		t.Fatalf("expected %q, got %q", expected, value)
	}
}

func TestActionDurationUnicode(t *testing.T) {
	value := ActionDuration("保存", "マインクラフト")
	expected := `minato_action_duration_seconds{action="保存",game="マインクラフト"}`
	if value != expected {
		t.Fatalf("expected %q, got %q", expected, value)
	}
}
