package promtail

import (
	"encoding/json"
	"fmt"
	"github.com/cornelk/hashmap"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type jsonLogEntry struct {
	Ts      time.Time `json:"ts"`
	Line    string    `json:"line"`
	level   LogLevel  // not used in JSON
	labels  *string
	labels2 *map[string]string
}

type clientJson struct {
	config    *ClientConfig
	quit      chan struct{}
	entries   chan *jsonLogEntry
	waitGroup sync.WaitGroup
	client    httpClient

	hashMap *hashmap.HashMap
}

//type intermediateEntry struct {
//	Labels map[string]string
//	Line string
//	Ts time.Time
//}

type lokiStreamWithLabels struct {
	Labels map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}
type lokiMsg struct {
	Streams []lokiStreamWithLabels `json:"streams"`
	//Streams []struct {
	//	Stream map[string]string `json:"stream"`
	//	Values [][]string `json:"values"`
	//} `json:"streams"`
}

func NewClientJson(conf ClientConfig) (Client, error) {
	client := clientJson{
		config:  &conf,
		quit:    make(chan struct{}),
		entries: make(chan *jsonLogEntry, LOG_ENTRIES_CHAN_SIZE),
		client: httpClient{
			parent: http.Client{Timeout: time.Second * 10},
		},
		hashMap: hashmap.New(HASHMAP_INIT_SIZE),
	}

	client.waitGroup.Add(1)

	go client.run2()

	return &client, nil
}

func (c *clientJson) Debugf(format string, labels *map[string]string, args ...interface{}) {
	c.log(format, DEBUG, "Debug: ", labels, args...)
}

func (c *clientJson) Infof(format string, labels *map[string]string, args ...interface{}) {
	c.log(format, INFO, "Info: ", labels, args...)
}

func (c *clientJson) Warnf(format string, labels *map[string]string, args ...interface{}) {
	c.log(format, WARN, "Warn: ", labels, args...)
}

func (c *clientJson) Errorf(format string, labels *map[string]string, args ...interface{}) {
	c.log(format, ERROR, "Error: ", labels, args...)
}

//func (c *clientJson) LogF(format string,labels *map[string]string, args ...interface{})  {
//	c.log(format, INFO, "", )
//}
//
//func (c *clientProto) logWithLabels(lineFormat string, level LogLevel, prefix string, labels map[string]string) {
//	var sb strings.Builder
//	sb.WriteByte('{')
//	isFirst := true
//	for key, val := range labels {
//		if isFirst {
//			isFirst = false
//		} else {
//			sb.WriteByte(',')
//		}
//		sb.WriteString(fmt.Sprintf("%v=\"%v\"", key, val))
//	}
//	sb.Write([]byte("}"))
//	s := logproto.EntryAdapter{
//		Timestamp: time.Time{},
//		Line:      "",
//	}
//	sa := logproto.StreamAdapter{
//		Labels:  sb.String(),
//		Entries: nil,
//	}
//}
func (c *clientJson) LogRaw(message string, labels *map[string]string) {

	//var strEntry = []*jsonLogEntry{{Ts: time.Now(), Line: message, level: ALL}}
	mergedKeys, _ := mergeKeys_string(*labels, c.config.Labels)
	labelString := *makeLabelString2(&mergedKeys)
	//actual, loaded := c.hashMap.GetOrInsert(labelString, &strEntry)
	//if loaded {
	//	actual = append((actual).([]*jsonLogEntry), strEntry...)
	//}

	c.entries <- &jsonLogEntry{
		Ts:      time.Now(),
		Line:    message,
		level:   DEBUG,
		labels:  &labelString,
		labels2: &mergedKeys,
	}
}
func (c *clientJson) log(format string, level LogLevel, prefix string, labels *map[string]string, args ...interface{}) {

	if (level >= c.config.SendLevel) || (level >= c.config.PrintLevel) {
		//var strEntry = []*jsonLogEntry{{Ts: time.Now(), Line: fmt.Sprintf(prefix+format, args...), level: level}}
		mergedKeys, _ := mergeKeys_string(*labels, c.config.Labels)
		labelString := *makeLabelString2(&mergedKeys)
		//labelString := *makeLabelString(labels, &c.config.Labels)
		//actual, loaded := c.hashMap.GetOrInsert(labelString, &strEntry)
		//if loaded {
		//	actual = append((actual).([]*jsonLogEntry), strEntry...)
		//}

		c.entries <- &jsonLogEntry{
			Ts:     time.Now(),
			Line:   fmt.Sprintf(prefix+format, args...),
			level:  level,
			labels: &labelString,
		}
	}
}

func (c *clientJson) Shutdown() {
	log.Println("Shutting down loki, waiting for waitGroup to complete")
	close(c.quit)
	c.waitGroup.Wait()
}
func (c *clientJson) run2() {
	maxWait := time.NewTimer(c.config.BatchWait)
	batchSize := 0
	defer func() {
		if c.hashMap.Len() > 0 {
			c.flush()
		}

		c.waitGroup.Done()
	}()
	for {
		select {

		case <-c.quit:
			return

		case entry := <-c.entries:
			if entry.level >= c.config.PrintLevel {
				log.Print(entry.Line)
			}
			if entry.level >= c.config.SendLevel {
				//var streamEntry2 []*[]string = []*[]string{&[]string{strconv.FormatInt(entry.Ts.UnixNano(), 10), entry.Line}}
				//var streamEntry = []*jsonLogEntry{entry}
				//var streamEntry2 = intermediateEntry{Labels: *entry.labels2, Line: entry.Line, Ts: entry.Ts}
				line := []string{strconv.FormatInt(entry.Ts.UnixNano(), 10), entry.Line}
				var strmWLbls = lokiStreamWithLabels{
					Labels: *entry.labels2,
					Values: [][]string{line},
				}
				actual, loaded := c.hashMap.GetOrInsert(*entry.labels, &strmWLbls)
				if loaded {
					vals := *((actual).(*lokiStreamWithLabels))
					vals.Values = append(vals.Values, line)
					//labels := (actual).(lokiStreamWithLabels).Labels
					//labels, err := mergeKeys_string(labels, *entry.labels2)

					//actual = append((actual).([]*jsonLogEntry), streamEntry2...)
				}
				batchSize++
				if batchSize >= c.config.BatchEntriesNumber {
					c.waitGroup.Add(1)
					go c.flush()
					//c.flush()
					batchSize = 0
					maxWait.Reset(c.config.BatchWait)
				}
			}
		case <-maxWait.C:
			if batchSize > 0 {
				c.waitGroup.Add(1)
				go c.flush()
				//c.flush()
				batchSize = 0
			}
			maxWait.Reset(c.config.BatchWait)
		}
	}
}

func (c *clientJson) flush() {
	defer c.waitGroup.Done()
	var streams []lokiStreamWithLabels
	for entry := range c.hashMap.Iter() {
		streams = append(streams, *(entry.Value).(*lokiStreamWithLabels))
		c.hashMap.Del(entry.Key)
	}
	jsonMsg, err := json.Marshal(lokiMsg{
		Streams: streams,
	})
	if err != nil {
		log.Printf("promtail.ClientJson: unable to marshal a JSON document: %s\n", err)
		return
	}

	resp, body, err := c.client.sendReq("POST", c.config.PushURL, "application/json", &jsonMsg)
	if err != nil {
		log.Printf("promtail.ClientJson: unable to send an HTTP request: %s\n", err)
		return
	}

	if resp.StatusCode != 204 {
		log.Printf("promtail.ClientJson: Unexpected HTTP status code: %d, message: %s\n", resp.StatusCode, body)
		return
	}
}

/// OLD IMPLEMENTATION

//func (c *clientJson) run() {
//	var batch []*jsonLogEntry
//	batchSize := 0
//	maxWait := time.NewTimer(c.config.BatchWait)
//
//	defer func() {
//		if batchSize > 0 {
//			c.send(batch)
//		}
//
//		c.waitGroup.Done()
//	}()
//
//	for {
//		select {
//		case <-c.quit:
//			return
//		case entry := <-c.entries:
//			if entry.level >= c.config.PrintLevel {
//				log.Print(entry.Line)
//			}
//
//			if entry.level >= c.config.SendLevel {
//				batch = append(batch, entry)
//				batchSize++
//				if batchSize >= c.config.BatchEntriesNumber {
//					c.send(batch)
//					batch = []*jsonLogEntry{}
//					batchSize = 0
//					maxWait.Reset(c.config.BatchWait)
//				}
//			}
//		case <-maxWait.C:
//			if batchSize > 0 {
//				c.send(batch)
//				batch = []*jsonLogEntry{}
//				batchSize = 0
//			}
//			maxWait.Reset(c.config.BatchWait)
//		}
//	}
//}
//
//func (c *clientJson) send(entries []*jsonLogEntry) {
//	var streams []promtailStream
//	streams = append(streams, promtailStream{
//		//Labels:  *makeLabelString(),
//		Entries: entries,
//	})
//
//	msg := promtailMsg{Streams: streams}
//	jsonMsg, err := json.Marshal(msg)
//	if err != nil {
//		log.Printf("promtail.ClientJson: unable to marshal a JSON document: %s\n", err)
//		return
//	}
//
//	resp, body, err := c.client.sendReq("POST", c.config.PushURL, "application/json", &jsonMsg)
//	if err != nil {
//		log.Printf("promtail.ClientJson: unable to send an HTTP request: %s\n", err)
//		return
//	}
//
//	if resp.StatusCode != 204 {
//		log.Printf("promtail.ClientJson: Unexpected HTTP status code: %d, message: %s\n", resp.StatusCode, body)
//		return
//	}
//}
