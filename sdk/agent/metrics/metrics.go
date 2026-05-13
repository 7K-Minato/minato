package metrics

import "fmt"

func PlayerCount(game string, server string) string {
	return fmt.Sprintf("minami_player_count{game=\"%s\",server=\"%s\"}", game, server)
}

func ActionDuration(action string, game string) string {
	return fmt.Sprintf("minami_action_duration_seconds{action=\"%s\",game=\"%s\"}", action, game)
}
