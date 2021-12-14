package promtail

import (
	"fmt"
	"github.com/cornelk/hashmap"
	"github.com/golang/snappy"
	"github.com/grafana/loki/pkg/logproto"
	"log"
	"net/http"
	"sync"
	"time"
)

type protoLogEntry struct {
	entry  logproto.Entry
	level  LogLevel
	labels string
}

type clientProto struct {
	config    *ClientConfig
	quit      chan struct{}
	entries   chan protoLogEntry
	waitGroup sync.WaitGroup
	client    httpClient
	hashMap   *hashmap.HashMap
}

func (c *clientProto) LogRaw(message string, labels *map[string]string, level LogLevel) {
	mergedKeys, _ := mergeKeys_string(*labels, c.config.Labels)
	c.entries <- protoLogEntry{
		entry: logproto.Entry{
			Timestamp: time.Now().UTC(),
			Line:      message,
		},
		level:  level,
		labels: *makeLabelString(&mergedKeys, &c.config.Labels),
	}
}

func NewClientProto(conf ClientConfig) (Client, error) {
	client := clientProto{
		config:  &conf,
		quit:    make(chan struct{}),
		entries: make(chan protoLogEntry, LOG_ENTRIES_CHAN_SIZE),
		client:  httpClient{},
		hashMap: hashmap.New(HASHMAP_INIT_SIZE),
	}

	client.waitGroup.Add(1)

	go client.run2()

	return &client, nil
}

func (c *clientProto) Debugf(format string, labels *map[string]string, args ...interface{}) {
	c.log(format, DEBUG, "Debug: ", labels, args...)
}

func (c *clientProto) Infof(format string, labels *map[string]string, args ...interface{}) {
	c.log(format, INFO, "Info: ", labels, args...)
}

func (c *clientProto) Warnf(format string, labels *map[string]string, args ...interface{}) {
	c.log(format, WARN, "Warn: ", labels, args...)
}

func (c *clientProto) Errorf(format string, labels *map[string]string, args ...interface{}) {
	c.log(format, ERROR, "Error: ", labels, args...)
}

func (c *clientProto) log(format string, level LogLevel, prefix string, labels *map[string]string, args ...interface{}) {
	//hashmap implementatation

	if (level >= c.config.SendLevel) || (level >= c.config.PrintLevel) {
		labelString := *makeLabelString(labels, &c.config.Labels)
		c.entries <- protoLogEntry{
			entry: logproto.Entry{
				Timestamp: time.Now(),
				Line:      fmt.Sprintf(prefix+format, args...),
			},
			level:  level,
			labels: labelString,
		}
	}
}

func (c *clientProto) Shutdown() {
	close(c.quit)
	c.waitGroup.Wait()
}
func (c *clientProto) run2() {
	maxWait := time.NewTimer(c.config.BatchWait)
	batchSize := 0
	defer func() {
		if c.hashMap.Len() > 0 {
			err := c.flush()
			if err != nil {
				log.Printf("Error encountered during flush operation: %s", err)
				return
			}
		}

		c.waitGroup.Done()
	}()
	for {
		if batchSize > c.config.BatchEntriesNumber {
			log.Println("batch full")
		}
		select {

		case <-c.quit:
			return

		case entry := <-c.entries:
			log.Printf("hashmap has %v entries before", c.hashMap.Len())
			if entry.level >= c.config.PrintLevel {
				log.Print(entry.entry.Line)
			}
			if entry.level >= c.config.SendLevel {
				var streamEntry = []logproto.Entry{
					{Timestamp: entry.entry.Timestamp, Line: entry.entry.Line},
				}
				if actual, loaded := c.hashMap.GetOrInsert(entry.labels, streamEntry); loaded {
					c.hashMap.Set(entry.labels, append((actual).([]logproto.Entry), streamEntry...))
				}
				batchSize++
				if batchSize >= c.config.BatchEntriesNumber {
					err := c.flush()
					if err != nil {
						log.Printf("Error encountered during flush operation: %s", err)
						return
					}
					batchSize = 0
					maxWait.Reset(c.config.BatchWait)
				}
			}
		case <-maxWait.C:
			if batchSize > 0 {
				err := c.flush()
				if err != nil {
					log.Printf("Error encountered during flush operation: %s", err)
					return
				}
				batchSize = 0
			}
			maxWait.Reset(c.config.BatchWait)
		}
	}
}
func (c *clientProto) flush() error {
	log.Printf("starting flush operation on protoclient, hashmap has %v entries", c.hashMap.Len())
	var streams []logproto.Stream
	for entry := range c.hashMap.Iter() {
		streams = append(streams, logproto.Stream{Labels: (entry.Key).(string), Entries: (entry.Value).([]logproto.Entry)})
		c.hashMap.Del(entry.Key)
	}

	req := logproto.PushRequest{
		Streams: streams,
	}
	err := c.handlePushRequest(&req)
	if err != nil {
		_ = fmt.Errorf("error during shit %w", err)
		return err
	}
	return nil
}

func (c *clientProto) handlePushRequest(pRequest *logproto.PushRequest) error {
	buf, err := pRequest.Marshal()
	buf = snappy.Encode(nil, buf)
	resp, body, err := c.client.sendReq(http.MethodPost, c.config.PushURL, "application/x-protobuf", &buf)
	if err != nil {
		log.Printf("promtail.ClientProto: unable to send an HTTP request: %s\n", err)
		return err
	}

	if resp.StatusCode != 204 {
		err := fmt.Errorf("promtail.ClientProto: Unexpected HTTP status code: %d, message: %s\n", resp.StatusCode, body)
		log.Println(err)
		return err
	}
	return nil
}

/// OLD IMPLEMENTATION
//func (c *clientProto) run() {
//	var batch []logproto.Entry
//	batchSize := 0
//	maxWait := time.NewTimer(c.config.BatchWait)
//
//	defer func() {
//		if batchSize > 0 {
//			c.send(&batch)
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
//				log.Print(entry.entry.Line)
//			}
//
//			if entry.level >= c.config.SendLevel {
//				batch = append(batch, entry.entry)
//				batchSize++
//				if batchSize >= c.config.BatchEntriesNumber {
//					c.send(&batch)
//					batch = []logproto.Entry{}
//					batchSize = 0
//					maxWait.Reset(c.config.BatchWait)
//				}
//			}
//		case <-maxWait.C:
//			if batchSize > 0 {
//				c.send(&batch)
//				batch = []logproto.Entry{}
//				batchSize = 0
//			}
//			maxWait.Reset(c.config.BatchWait)
//		}
//	}
//}
//
//func (c *clientProto) send(entries *[]logproto.Entry) {
//	var streams []logproto.Stream
//	streams = append(streams, logproto.Stream{
//		//Labels:  c.config.Labels,
//		Entries: *entries,
//	})
//
//	req := logproto.PushRequest{
//		Streams: streams,
//	}
//
//	buf, err := proto.Marshal(&req)
//	if err != nil {
//		log.Printf("promtail.ClientProto: unable to marshal: %s\n", err)
//		return
//	}
//
//	buf = snappy.Encode(nil, buf)
//
//	resp, body, err := c.client.sendReq("POST", c.config.PushURL, "application/x-protobuf", &buf)
//	if err != nil {
//		log.Printf("promtail.ClientProto: unable to send an HTTP request: %s\n", err)
//		return
//	}
//
//	if resp.StatusCode != 204 {
//		log.Printf("promtail.ClientProto: Unexpected HTTP status code: %d, message: %s\n", resp.StatusCode, body)
//		return
//	}
//}
