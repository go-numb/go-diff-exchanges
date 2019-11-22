package exchanges

import "time"

// Exchange is each exchange interface
type Exchange interface {
	Close() error
	// Connect is websocket connetion
	Connect()

	Reset()
	LTP() (ltp, volume float64)
	Delay() (sec float64)
	EventTime() time.Time
	ServerTime() time.Time
}
