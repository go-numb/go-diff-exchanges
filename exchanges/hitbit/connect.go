package hitbit

import (
	"sync"
	"time"

	"gonum.org/v1/gonum/stat"

	jsoniter "github.com/json-iterator/go"

	"github.com/buger/jsonparser"

	"golang.org/x/sync/errgroup"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

const (
	// WSREADDEADLINE 受信待機時間
	WSREADDEADLINE = 5 * time.Minute
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// Client is structure
type Client struct {
	E      *E
	Logger *logrus.Entry
}

// New is new client
func New(log *logrus.Entry) *Client {
	return &Client{
		E: &E{
			Executions: make([]Execution, 0),
			ServerTime: time.Now().UTC(),
		},
		Logger: log,
	}
}

// Close all work
func (p *Client) Close() error {

	return nil
}

// E get executions
type E struct {
	mux        sync.Mutex
	Executions []Execution
	ServerTime time.Time
}

// Execution is trades
type Execution struct {
	ID        int       `json:"id"`
	Price     float64   `json:"price,string"`
	Quantity  float64   `json:"quantity,string"`
	Side      string    `json:"side"`
	Timestamp time.Time `json:"timestamp"`
}

// Request set options
type Request struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
	ID     int                    `json:"id"`
}

// Connect websocket
func (p *Client) Connect() {
	conn, _, err := websocket.DefaultDialer.Dial("wss://api.hitbtc.com/api/2/ws", nil)
	if err != nil {
		p.Logger.Error(err)
		return
	}
	defer conn.Close()

	channels := []string{"subscribeTrades"}
	symbols := []string{"BTCUSD"}
	for i := range channels {
		for j := range symbols {
			if err := conn.WriteJSON(&Request{
				Method: channels[i],
				Params: map[string]interface{}{
					"symbol": symbols[j],
					// "limit":  3,
					// "sort":   "DESC",
					// "by":     "id",
				},
				ID: int(time.Now().UTC().Unix()),
			}); err != nil {
				p.Logger.Error(err)
				return
			}
		}
	}

	var eg errgroup.Group
	eg.Go(func() error {
		for {
			conn.SetReadDeadline(time.Now().Add(WSREADDEADLINE))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return err
			}

			// p.Logger.Infof("%s", string(msg))

			// // Ping&Pong
			// conn.WriteMessage(websocket.TextMessage, []byte(`2`))

			// name, _, _, err := jsonparser.Get(msg, "params", "channel")
			// if err != nil {
			// 	continue
			// }
			if err := p.E.set(msg); err != nil {
				continue
			}
			// fmt.Printf("%+v - %.f - %.4f\n", p.E.Executions[0].ID, p.E.Executions[0].Price, p.E.ServerTime.Sub(p.E.Executions[0].ExecDate.Time).Seconds())
			// ltp, volume := p.LTP()
			// fmt.Printf("%.f - %.4f - %.4fsec\n", ltp, volume, p.Delay())
		}
	})

	if err := eg.Wait(); err != nil {
		// scheduled maintenance
		p.Logger.Error(err)
	}
}

// Reset is 約定配列のリセット
// 各取引所同時にn秒足などをつくる際に活用
func (p *Client) Reset() {
	p.E.mux.Lock()
	defer p.E.mux.Unlock()
	if !p.E.isThere() {
		return
	}
	p.E.Executions = []Execution{}
}

// LTP is 直近価格の出来高加重平均価格・出来高を取得
func (p *Client) LTP() (ltp, volume float64) {
	p.E.mux.Lock()
	defer p.E.mux.Unlock()
	use := p.E.Executions
	prices := make([]float64, len(use))
	volumes := make([]float64, len(use))
	for i := range use {
		prices[i] = use[i].Price
		volumes[i] = use[i].Quantity
		volume += use[i].Quantity
	}
	return stat.Mean(prices, volumes), volume
}

// Delay is 直近価格の出来高加重平均価格・出来高を取得
func (p *Client) Delay() (sec float64) {
	if !p.E.isThere() {
		return 0
	}
	return p.E.ServerTime.Sub(p.E.Executions[len(p.E.Executions)-1].Timestamp).Seconds()
}

// EventTime is send exchange stamp
func (p *Client) EventTime() time.Time {
	if !p.E.isThere() {
		return time.Now()
	}
	return p.E.Executions[len(p.E.Executions)-1].Timestamp
}

// ServerTime is revice server time
func (p *Client) ServerTime() time.Time {
	if !p.E.isThere() {
		return time.Now()
	}
	return p.E.ServerTime
}

func (p *E) isThere() bool {
	if len(p.Executions) < 1 {
		return false
	}
	return true
}

func (p *E) set(b []byte) error {
	data, _, _, err := jsonparser.Get(b, "params", "data")
	if err != nil {
		return err
	}

	var ex []Execution
	json.Unmarshal(data, &ex)
	p.Executions = append(p.Executions, ex...)
	p.ServerTime = time.Now().UTC()
	return nil
}
