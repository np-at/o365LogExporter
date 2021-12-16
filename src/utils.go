package main

import (
	"fmt"
	"strings"
)

func splitStringOnChar(str string, char byte) (string, string, error) {
	for i := 0; i < len(str); i++ {
		if (str)[i] == '=' {
			return (str)[:i], (str)[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("no match for split char %v found for string: %v", char, str)
}

func logStringSani(inputStr string) string {
	escapedStr := strings.Replace(inputStr, "\n", "", -1)
	escapedStr = strings.Replace(escapedStr, "\r", "", -1)
	return escapedStr
}
