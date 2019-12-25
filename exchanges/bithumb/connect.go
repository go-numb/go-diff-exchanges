package bithumb

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/buger/jsonparser"

	"gonum.org/v1/gonum/stat"

	jsoniter "github.com/json-iterator/go"

	"golang.org/x/sync/errgroup"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

const (
	// WSREADDEADLINE 受信待機時間
	WSREADDEADLINE = 5 * time.Minute

	// WSPINGTIMER PING確認
	WSPINGTIMER = 30 * time.Second
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
	P         string `json:"p"`
	Price     float64
	Side      string `json:"s"`
	S         string `json:"v"`
	Size      float64
	Timestamp time.Time `json:"-"`
	Symbol    string    `json:"symbol"`
	Ver       string    `json:"ver"`
}

// Request set options
type Request struct {
	Command string   `json:"cmd"`
	Args    []string `json:"args"`
}

// Connect websocket
func (p *Client) Connect() {
	conn, _, err := websocket.DefaultDialer.Dial("wss://global-api.bithumb.pro/message/realtime", nil)
	if err != nil {
		p.Logger.Error(err)
		return
	}
	defer conn.Close()

	channels := []string{"TRADE"}
	symbols := []string{"BTC-USDT"}
	var args []string
	for i := range channels {
		for j := range symbols {
			args = append(args, fmt.Sprintf("%s:%s", channels[i], symbols[j]))
		}
	}

	if err := conn.WriteJSON(&Request{
		Command: "subscribe",
		Args:    args,
	}); err != nil {
		p.Logger.Error(err)
		return
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

	eg.Go(func() error {
		ticker := time.NewTicker(WSPINGTIMER)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				conn.WriteMessage(websocket.TextMessage, []byte(`{"cmd":"ping"}`))

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
	data, _, _, err := jsonparser.Get(b, "data")
	if err != nil {
		return err
	}
	timestamp, err := jsonparser.GetInt(b, "timestamp")
	if err != nil {
		return err
	}
	var ex Execution
	json.Unmarshal(data, &ex)
	price, err := strconv.ParseFloat(ex.P, 64)
	if err != nil {
		return err
	}
	ex.Price = price
	size, err := strconv.ParseFloat(ex.S, 64)
	if err != nil {
		return err
	}
	ex.Size = size
	t := time.Unix(timestamp/1000, 0)
	ex.Timestamp = t

	p.Executions = append(p.Executions, ex)
	p.ServerTime = time.Now().UTC()
	return nil
}
