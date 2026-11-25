package handler

import "strings"

func passwordHasSurroundingWhitespace(password string) bool {
	return password != strings.TrimSpace(password)
}
