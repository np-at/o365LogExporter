package main

import (
	"fmt"
	"os"
	"sync"
)

type fileOutputWrapper struct {
	filePath       string
	writeLock      sync.Mutex
	openFileHandle *os.File
	isOpen         bool
}

func (f *fileOutputWrapper) open() error {
	if f.openFileHandle != nil || f.isOpen {
		return fmt.Errorf("attempted to open file, but file handle already present in object: %v", f.openFileHandle)
	}
	var err error
	f.openFileHandle, err = os.OpenFile(f.filePath, os.O_CREATE|os.O_APPEND, 0755)
	if err != nil {
		return err
	}
	f.isOpen = true
	return nil
}
func (f *fileOutputWrapper) writeBytes(content []byte) (int, error) {
	f.writeLock.Lock()
	write, err := f.openFileHandle.Write(content)
	if err != nil {
		return 0, nil
	}
	f.writeLock.Unlock()
	return write, err
}
func (f *fileOutputWrapper) writeString(stringContent string) (int, error) {
	f.writeLock.Lock()
	write, err := f.openFileHandle.WriteString(stringContent)
	if err != nil {
		return 0, nil
	}
	f.writeLock.Unlock()
	return write, err
}
func (f *fileOutputWrapper) close() error {
	f.writeLock.Lock()
	err := f.openFileHandle.Close()
	if err != nil {
		return err
	}
	return nil
}
