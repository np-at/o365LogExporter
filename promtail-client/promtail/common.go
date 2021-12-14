package promtail

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

const LOG_ENTRIES_CHAN_SIZE = 5000
const HASHMAP_INIT_SIZE uintptr = 100

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO  LogLevel = iota
	WARN  LogLevel = iota
	ERROR LogLevel = iota
	// Maximum level, disables sending or printing
	DISABLE LogLevel = iota
)

type ClientConfig struct {
	// E.g. http://localhost:3100/api/prom/push
	PushURL string
	// E.g. "{job=\"somejob\"}"
	Labels             map[string]string
	BatchWait          time.Duration
	BatchEntriesNumber int
	// Logs are sent to Promtail if the entry level is >= SendLevel
	SendLevel LogLevel
	// Logs are printed to stdout if the entry level is >= PrintLevel
	PrintLevel LogLevel
}

type Client interface {
	Debugf(format string, labels *map[string]string, args ...interface{})
	Infof(format string, labels *map[string]string, args ...interface{})
	Warnf(format string, labels *map[string]string, args ...interface{})
	Errorf(format string, labels *map[string]string, args ...interface{})
	//Logf(format string, labels *map[string]string, args ...interface{})

	// LogRaw Writes log entry with pre-formatted line and arbitrary labels
	LogRaw(message string, labels *map[string]string, level LogLevel)
	Shutdown()
}

// http.Client wrapper for adding new methods, particularly sendReq
type httpClient struct {
	parent http.Client
}

// A bit more convenient method for sending requests to the HTTP server
func (client *httpClient) sendReq(method, url string, ctype string, reqBody *[]byte) (resp *http.Response, resBody []byte, err error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(*reqBody))
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", ctype)
	resp, err = client.parent.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatalln(err)
		}
	}(resp.Body)

	resBody, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	log.Printf("got response with status: %v", resp.StatusCode)

	return resp, resBody, nil
}

func makeLabelString2(labels *map[string]string) *string {
	var sb strings.Builder
	sb.WriteByte('{')
	isFirst := true
	keys := make([]string, 0, len(*labels))
	for k := range *labels {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, key := range keys {
		if isFirst {
			isFirst = false
		} else {
			sb.WriteByte(',')
		}
		val := (*labels)[key]

		sb.WriteString(fmt.Sprintf("%v=\"%v\"", key, val))
	}
	sb.Write([]byte("}"))
	var labelString = sb.String()
	return &labelString
}
func makeLabelString(labels, staticLabels *map[string]string) *string {

	var sb strings.Builder
	sb.WriteByte('{')
	isFirst := true
	keys := make([]string, 0, len(*labels)+len(*staticLabels))
	for k := range *labels {
		keys = append(keys, k)
	}
	for k := range *staticLabels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if isFirst {
			isFirst = false
		} else {
			sb.WriteByte(',')
		}
		val, ok := (*labels)[key]
		if !ok {
			val = (*staticLabels)[key]
		}
		sb.WriteString(fmt.Sprintf("%v=\"%v\"", key, val))
	}
	sb.Write([]byte("}"))
	var labelString = sb.String()
	return &labelString

}
