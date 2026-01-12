package utils

import "strings"

func GetTopicN(topic string, n int) string {
	tokens := strings.Split(topic, "/")
	if n >= len(tokens) {
		return ""
	}
	return tokens[n]
}
