package main

import (
	"fmt"
)

func splitStringOnChar(str string, char byte) (string, string, error) {
	for i := 0; i < len(str); i++ {
		if (str)[i] == '=' {
			return (str)[:i], (str)[i+1:], nil
		}
	}
	return "","",fmt.Errorf("no match for split char %v found for string: %v",char, str)
}

