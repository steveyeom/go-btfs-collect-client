package logclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	btproto "github.com/TRON-US/go-btfs-collect-client/proto"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/jpillora/backoff"
)

var (
	ErrUnexpectedEntryType = errors.New("unexpected entry type")
)

type NetworkOut struct {
	conf       *Configuration
	stopChan   chan struct{}
	inputChan  chan []Entry
	waitGroup  sync.WaitGroup
	backoff    *backoff.Backoff
	httpClient *http.Client
}

// Use a global http client
var httpClient *http.Client

func init() {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 2,
	}
	httpClient = &http.Client{
		Transport: transport,
	}
}

func NewNetworkOut(cfg *Configuration) (*NetworkOut, error) {
	networkOut := NetworkOut{
		conf:       cfg,
		stopChan:   make(chan struct{}),
		inputChan:  make(chan []Entry),
		httpClient: httpClient,
	}
	networkOut.waitGroup.Add(1)
	networkOut.backoff = &backoff.Backoff{
		Factor: 2.0,
		Jitter: false,
		Min:    2 + time.Second,
		Max:    10 + time.Second,
	}
	go networkOut.run()
	return &networkOut, nil
}

func (netOut *NetworkOut) run() {
	// Define variables
	var entries []Entry
	conf := netOut.conf
	currBatchSize := 0

	// init
	timer := time.NewTimer(conf.BatchWaitDuration)

	defer func() {
		netOut.sendBatch(entries)
		netOut.waitGroup.Done()
	}()

	// main loop
	for true {
		select {
		case <-timer.C:
			if len(entries) > 0 {
				err := netOut.sendBatch(entries)
				if err != nil {
					log.Printf("Error: NetworkOut: sendBatch failed: %s\n", err)
				} else {
					log.Printf("Info: NetworkOut: sendBatch OK, number of entries: %d\n", len(entries))
				}
				entries = []Entry{}
				currBatchSize = 0
			}
		case inEntries := <-netOut.inputChan:
			for _, inEnt := range inEntries {
				entries = append(entries, inEnt)
				currBatchSize++
			}
			if currBatchSize >= netOut.conf.BatchCapacity {
				err := netOut.sendBatch(entries)
				if err != nil {
					log.Printf("Error: NetworkOut: sendBatch failed: %s\n", err)
				} else {
					log.Printf("Info: NetworkOut: sentBatch OK, number of entries: %d\n", len(entries))
				}
				entries = []Entry{}
				currBatchSize = 0
			}
		case <-netOut.stopChan:
			return
		}
		timer.Reset(netOut.conf.BatchWaitDuration)
	}
}

func (netOut *NetworkOut) sendBatch(entries []Entry) error {
	var streams []*btproto.Stream
	var protoEntries []*btproto.Entry
	for _, entry := range entries {
		proent, ok := entry.Value().(*btproto.Entry)
		if !ok {
			return fmt.Errorf("expected *btproto.Entry type, but got %T", entry.Value())
		}
		protoEntries = append(protoEntries, proent)
	}
	streams = append(streams, &btproto.Stream{
		Labels:  netOut.conf.Labels,
		Entries: protoEntries,
	})
	request := btproto.PushRequest{
		Streams: streams,
	}
	data, err := proto.Marshal(&request)
	if err != nil {
		return err
	}
	compressed := snappy.Encode(nil, data)

	err = netOut.send(compressed)
	if err != nil {
		return err
	}

	return nil
}

// TODO: Add retry mechanism with backoff but probably for 5xx Server status codes,
// if the consecutive errors > threshold, use recovery manager (or store the current in-memory queue)
func (netOut *NetworkOut) send(compressed []byte) error {
	httpReq, err := http.NewRequest("POST", netOut.conf.URL, bytes.NewReader(compressed))
	if err != nil {
		return err
	}
	httpReq.Header.Add("Content-Type", "application/x-protobuff")

	netOut.backoff.Reset()

	for {
		ctx, cancel := context.WithTimeout(context.Background(), netOut.conf.NetworkSendTimeout)
		defer cancel()

		httpReq = httpReq.WithContext(ctx)

		httpResp, err := netOut.httpClient.Do(httpReq)
		if err != nil {
			return errors.New(fmt.Sprintf("failed to send request: got error [%v]", err))
		}

		defer httpResp.Body.Close()
		if httpResp.StatusCode/100 == 2 {
			_, err := io.Copy(ioutil.Discard, io.LimitReader(httpResp.Body, 1<<20+1))
			if err != nil {
				log.Printf("error: failed to read response body")
			}
			return nil
		}
		ebody, err := ioutil.ReadAll(httpResp.Body)
		if err != nil {
			return fmt.Errorf("error readng error body: %s: %s", httpResp.StatusCode, err)
		}
		err = fmt.Errorf("error response %s from remote server %s: %s",
			httpResp.StatusCode, netOut.conf.URL, string(ebody))
		if netOut.backoff.Attempt() >= float64(netOut.conf.NetworkSendRetries) {
			return err
		}
		if httpResp.StatusCode/100 == 5 {
			d := netOut.backoff.Duration()
			fmt.Errorf("error: %s, retrying in %s", err, d)
			time.Sleep(d)
		} else {
			return err
		}
	}
}

func (netOut *NetworkOut) Close() {
	close(netOut.stopChan)
	netOut.waitGroup.Wait()
}
