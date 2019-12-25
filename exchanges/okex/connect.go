package okex

import (
	"bytes"
	"compress/flate"
	"context"
	"io/ioutil"
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

	// WSPINGTIMER PING確認
	WSPINGTIMER = 25 * time.Second
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
	InstrumentID string    `json:"instrument_id"`
	Price        float64   `json:"price,string"`
	Side         string    `json:"side"`
	Size         float64   `json:"size,string"`
	Timestamp    time.Time `json:"timestamp"`
	TradeID      string    `json:"trade_id"`
}

// Request set options
type Request struct {
	Op   string   `json:"op"`
	Args []string `json:"args"`
}

// Connect websocket
func (p *Client) Connect() {
	conn, _, err := websocket.DefaultDialer.Dial("wss://real.okex.com:8443/ws/v3", nil)
	if err != nil {
		p.Logger.Error(err)
		return
	}
	defer conn.Close()

	channels := []string{"spot/trade:BTC-USDT"}
	for i := range channels {
		if err := conn.WriteJSON(&Request{
			Op:   "subscribe",
			Args: []string{channels[i]},
		}); err != nil {
			p.Logger.Error(err)
			return
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		defer cancel()
		for {
			conn.SetReadDeadline(time.Now().Add(WSREADDEADLINE))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return err
			}

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

	eg.Go(func() error {
		ticker := time.NewTicker(WSPINGTIMER)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				conn.WriteMessage(websocket.TextMessage, []byte(`2`))

			case <-ctx.Done():
				return ctx.Err()
			}
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
		volumes[i] = use[i].Size
		volume += use[i].Size
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
	data, err := gzipDecode(b)
	if err != nil {
		return err
	}
	data, _, _, err = jsonparser.Get(data, "data")
	if err != nil {
		return err
	}

	var ex []Execution
	json.Unmarshal(data, &ex)
	p.Executions = append(p.Executions, ex...)
	p.ServerTime = time.Now().UTC()
	return nil
}

func gzipDecode(in []byte) ([]byte, error) {
	reader := flate.NewReader(bytes.NewReader(in))
	defer reader.Close()

	return ioutil.ReadAll(reader)
}
