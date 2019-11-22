package huobi

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"math/big"
	"sync"
	"time"

	"gonum.org/v1/gonum/stat"

	"github.com/json-iterator/go"

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
	ID        big.Int `json:"id"`
	Ts        int64   `json:"ts"`
	TradeID   int64   `json:"tradeId"`
	Amount    float64 `json:"amount"`
	Price     float64 `json:"price"`
	Direction string  `json:"direction"`
}

// Request set options
type Request struct {
	Sub string `json:"sub"`
	ID  string `json:"id"`
}

// Connect websocket
func (p *Client) Connect() {
	conn, _, err := websocket.DefaultDialer.Dial("wss://api.huobi.pro/ws", nil)
	if err != nil {
		p.Logger.Fatal(err)
	}
	defer conn.Close()

	channels := []string{"market.%s.trade.detail"}
	symbols := []string{"btcusdt"}
	for _, channel := range channels {
		for _, symbol := range symbols {
			if err := conn.WriteJSON(&Request{
				Sub: fmt.Sprintf(channel, symbol),
				ID:  fmt.Sprintf("id:%d", time.Now().Unix()),
			}); err != nil {
				p.Logger.Fatal(err)
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
			data, err := gzipDecode(msg)
			if err != nil {
				return err
			}

			// p.Logger.Infof("%s", string(msg))

			ts, _ := jsonparser.GetInt(data, "ping")
			if ts != 0 { // ping&pong/5s
				conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"pong":%d}`, ts)))
				continue
			}

			if err := p.E.set(data); err != nil {
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
	for i, e := range use {
		prices[i] = e.Price
		volumes[i] = e.Amount
		volume += e.Amount
	}
	return stat.Mean(prices, volumes), volume
}

// Delay is 直近価格の出来高加重平均価格・出来高を取得
func (p *Client) Delay() (sec float64) {
	if !p.E.isThere() {
		return 0
	}
	t := time.Unix(p.E.Executions[len(p.E.Executions)-1].Ts/1000, 10)
	return p.E.ServerTime.Sub(t).Seconds()
}

// EventTime is send exchange stamp
func (p *Client) EventTime() time.Time {
	if !p.E.isThere() {
		return time.Now()
	}
	return time.Unix(p.E.Executions[len(p.E.Executions)-1].Ts/1000, 10)
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
	data, _, _, err := jsonparser.Get(b, "tick", "data")
	if err != nil {
		return err
	}

	var ex []Execution
	if err := json.Unmarshal(data, &ex); err != nil {
		return err
	}
	p.Executions = append(p.Executions, ex...)
	p.ServerTime = time.Now().UTC()
	return nil
}

func gzipDecode(in []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return ioutil.ReadAll(reader)
}
