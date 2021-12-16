package main

import (
	"encoding/json"
	"fmt"
	"github.com/jmespath/go-jmespath"
	"log"
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

func extractJMESLabels(jsonData []byte, JMESLabels *map[string]string, labelMap *map[string]string) error {
	var data interface{}
	var result interface{}
	err := json.Unmarshal(jsonData, &data)

	for i, s := range *JMESLabels {
		result, err = jmespath.Search(s, data)
		if err != nil {
			log.Print(err)
			return err
		}
		(*labelMap)[i] = result.(string)
	}
	return nil
	//result, err := jmespath.Search("Operation", data)
	//(*labelMap)["Operation"] = result.(string)
	//result, err = jmespath.Search("Workload", data)
	//
	//(*labelMap)["Workload"] = result.(string)
	//result, err = jmespath.Search("CreationTime", data)
	//
	//(*labelMap)["_ts"] = result.(string)
	//return nil
}
