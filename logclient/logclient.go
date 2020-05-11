package logclient

const (
	DebugLevel = iota + 1
	InfoLevel
	WarnLevel
	ErrorLevel
	DevPanicLevel
	PanicLevel
)

const (
	LogClientAPIEnabled       = true
	MininumCollectionLogLevel = 4
)

type LogClient struct {
	conf       *Configuration
	logReader  *LogReader
	networkOut *NetworkOut
	// LogClient input channel to which an application sends input entries.
	InputChan chan []Entry
	// Log events pass through LogClient when their log level is > minCollectLogLevel.
	minCollectLogLevel int
}

func NewLogClient(conf *Configuration) (*LogClient, error) {
	// initialize operators top down
	var inputChan chan []Entry
	ntkOut, err := NewNetworkOut(conf)
	if err != nil {
		return nil, err
	}
	inputChan = ntkOut.inputChan

	var logR *LogReader

	if !conf.LogAPIEnabled {
		logR, err = NewLogReader(conf, ntkOut.inputChan)
		if err != nil {
			return nil, err
		}
		inputChan = logR.inputChan
	}

	return &LogClient{
		minCollectLogLevel: MininumCollectionLogLevel,
		conf:               conf,
		logReader:          logR,
		networkOut:         ntkOut,
		InputChan:          inputChan,
	}, nil
}

func (lc *LogClient) Close() {
	lc.logReader.Close()
	lc.networkOut.Close()
}

func (lc *LogClient) LogReader() *LogReader {
	return lc.logReader
}

func (lc *LogClient) NetworkOut() *NetworkOut {
	return lc.networkOut
}
