package logclient

import (
	"fmt"
	"github.com/golang/protobuf/ptypes"
	"github.com/steveyeom/go-btfs-logclient/logproto"
	"log"
	"sync"
	"time"
)

type Line struct {
	Text string
}

type LogReader struct {
	conf       *Configuration
	inputChan  chan []Entry
	outputChan chan []Entry
	stopChan   chan struct{}
	waitGroup  sync.WaitGroup
}

func NewLogReader(conf *Configuration, outChan chan []Entry) (*LogReader, error) {
	logReader := &LogReader{
		conf:       conf,
		inputChan:  make(chan []Entry),
		outputChan: outChan,
		stopChan:   make(chan struct{}),
	}
	logReader.waitGroup.Add(1)
	go logReader.run()
	return logReader, nil
}

func (logReader *LogReader) run() {
	// Define variables
	var lineEntries []Entry
	conf := logReader.conf
	retry := 1
	currBatchSize := 0

	// init
	defer func() {
		logReader.waitGroup.Done()
	}()

	// main loop
	for true {
		select {
		case inEntries := <-logReader.inputChan:
			for _, inEnt := range inEntries {
				le, ok := inEnt.(LineEntry)
				if !ok {
					log.Printf("Error: LogReader: %s", fmt.Errorf("expected *logproto.Entry type, but got %T", inEnt.Value()))
					return
				}
				lineEntries = append(lineEntries, le)
				currBatchSize++
			}
			if currBatchSize >= conf.BatchCapacity {
				err := logReader.sendBatch(lineEntries)
				if err != nil {
					log.Printf("Error: LogReader: tried %d time: %s\n", retry, err)
					// TODO: check retry times and save..
				} else {
					log.Printf("Info: LogReader: sendBatch OK, number of lines: %d\n", len(lineEntries))
				}
				lineEntries = []Entry{}
				currBatchSize = 0
			}
		case <-logReader.stopChan:
			return
		}
		retry = 1
	}
}

// Send a batch after converting lines to logproto.Entry's
func (logReader *LogReader) sendBatch(lines []Entry) error {
	ts, err := ptypes.TimestampProto(time.Now())
	if err != nil {
		return err
	}
	var protoEntries []Entry
	for _, line := range lines {
		le, _ := line.(LineEntry)
		protoEntries = append(protoEntries, ProtoEntry{&logproto.Entry{
			Timestamp: ts,
			Line:      le.Text,
		}})
	}
	logReader.outputChan <- protoEntries
	return nil
}

func (logReader *LogReader) Close() {
	close(logReader.stopChan)
}

func (logReader *LogReader) InputChan() (chan []Entry, error) {
	if logReader == nil {
		return nil, fmt.Errorf("logReader is nil")
	}
	return logReader.inputChan, nil
}
