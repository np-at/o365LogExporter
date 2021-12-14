package main

import (
	"bufio"
	"fmt"
	"github.com/cornelk/hashmap"
	"log"
	"os"
	"strconv"
	"time"
)

type Tracker struct {
	hashSet         hashmap.HashMap
	historyFilePath string
}

func (t *Tracker) load() {
	loadIntoSet(&t.hashSet, t.historyFilePath)
}
func loadIntoSet(set *hashmap.HashMap, filePath string) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		//log.Println(err)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(file)
	fileScanner := bufio.NewScanner(file)
	fileScanner.Split(bufio.ScanLines)
	for fileScanner.Scan() {
		for i, b := range fileScanner.Bytes() {
			if b == '\t' {
				if _, loaded := (*set).GetOrInsert(string(fileScanner.Bytes()[0:i]), string(fileScanner.Bytes()[(i+1):len(fileScanner.Bytes())])); loaded {
					log.Println("Duplicate entry found in histFile")
				}
				break
			}
		}
	}
}
func (t *Tracker) String() {
	log.Println(t.hashSet.String())
}

// dump re-reads the history file and finds the difference between the in-memory set and the old set
// and updates the history set accordingly with time=now
func (t *Tracker) dump() error {
	file, err := os.OpenFile(t.historyFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal(err)
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(file)
	pnt := int64(0)
	for entry := range t.hashSet.Iter() {
		s := ([]byte)(fmt.Sprintf("%v\t%v\n", entry.Key, entry.Value))
		sz, err2 := file.WriteAt(s, pnt)
		pnt += int64(sz)
		if err2 != nil {
			log.Println(err2)
			return err2
		}
	}
	return nil
}
func (t *Tracker) pruneHistory(threshold time.Duration) error {
	if t.hashSet.Len() == 0 {
		loadIntoSet(&t.hashSet, t.historyFilePath)
	}
	// subtract the threshold time to give us the unix date before which we should prune entries
	thresholdTime := time.Now().Add(-threshold).Unix()

	for entry := range t.hashSet.Iter() {
		entryTs := (entry.Value).(string)
		unixTS, err := strconv.ParseInt(entryTs, 10, 64)
		if err != nil {
			log.Println(err)
			return err
		}
		if thresholdTime >= unixTS {
			t.hashSet.Del(entry.Key)
		}
	}
	err := t.dump()
	if err != nil {
		return err
	}
	return nil
}
