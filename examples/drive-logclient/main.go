package main

import (
	"fmt"
	"log"
	"os"
	"time"

	logclient "github.com/steveyeom/go-btfs-logclient/logclient"
)

func main() {
	// init configuration
	conf := &logclient.Configuration{
		Labels:             `{job="btnode-log"}`, // TODO: add cid.String() for the current btfs node as "instance"
		LogAPIEnabled:      false,
		URL:                "http://localhost:3100/loki/api/v1/push",
		Destination:        "loki",
		BatchWaitDuration:  10 * time.Second,
		BatchCapacity:      5,
		NetworkSendTimeout: 15 * time.Second,
		NetworkSendRetries: logclient.DEFAULT_NUM_OF_RETRIES,
	}

	// Open operators
	logc, err := logclient.NewLogClient(conf)
	if err != nil {
		log.Printf("error: failed to create LogClient: %s\n", err)
		os.Exit(1)
	}
	log.Println("demo: opened LogClient.")

	// Execute in a loop
	var lineEntries []logclient.Entry
	inputChan := logc.InputChan
	for i := 0; i < 100; i++ {
		text := fmt.Sprintf("Error: test error message [%d], possibly monitored from loki and grafana.", i+1)
		log.Printf("Sending a log event entry [%s]", text)
		lineEntries = append(lineEntries, logclient.LineEntry{Text: text})
		inputChan <- lineEntries

		time.Sleep(time.Second * 1)
		lineEntries = []logclient.Entry{}
	}

	logc.Close()
	log.Println("demo: closed LogAPI.")
}
