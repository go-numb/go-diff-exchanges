package bitfinex

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/buger/jsonparser"

	"github.com/bitfinexcom/bitfinex-api-go/v2"

	"gonum.org/v1/gonum/stat"

	"github.com/json-iterator/go"

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
			Executions: make([]bitfinex.Trade, 0),
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
	Executions []bitfinex.Trade
	ServerTime time.Time
}

// Request set options
type Request struct {
	Event   string `json:"event"`
	Channel string `json:"channel"`
	Symbol  string `json:"symbol"`
}

// Connect websocket
func (p *Client) Connect() {
	conn, _, err := websocket.DefaultDialer.Dial("wss://api-pub.bitfinex.com/ws/2", nil)
	if err != nil {
		p.Logger.Fatal(err)
	}
	defer conn.Close()

	channels := []string{"trades"}
	symbols := []string{"tBTCUSD"}
	for _, channel := range channels {
		for _, symbol := range symbols {
			if err := conn.WriteJSON(&Request{
				Event:   "subscribe",
				Channel: channel,
				Symbol:  symbol,
			}); err != nil {
				p.Logger.Fatal(err)
			}
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
	p.E.Executions = []bitfinex.Trade{}
}

// LTP is 直近価格の出来高加重平均価格・出来高を取得
<<<<<<< HEAD
func (p Client) LTP() (ltp, volume float64) {
	prices := make([]float64, len(p.E.Executions))
	volumes := make([]float64, len(p.E.Executions))
	for i, e := range p.E.Executions {
=======
func (p *Client) LTP() (ltp, volume float64) {
	p.E.mux.Lock()
	defer p.E.mux.Unlock()
	use := p.E.Executions
	prices := make([]float64, len(use))
	volumes := make([]float64, len(use))
	for i, e := range use {
>>>>>>> Refactoring because an abnormal price was noticeable when price changed suddenly. Add IsZeroCheck
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
	t := time.Unix(p.E.Executions[len(p.E.Executions)-1].MTS/1000, 10)
	return p.E.ServerTime.Sub(t).Seconds()
}

// EventTime is send exchange stamp
func (p *Client) EventTime() time.Time {
	if !p.E.isThere() {
		return time.Now()
	}
	return time.Unix(p.E.Executions[len(p.E.Executions)-1].MTS/1000, 10)
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
	isUpdate, err := jsonparser.GetString(b, "[1]")
	if err != nil {
		return err
	}

	var trade bitfinex.Trade
	switch isUpdate {
	case "te": // Execution（処理済み、精査未満
		b, _, _, err := jsonparser.Get(b, "[2]")
		if err != nil {
			return err
		}
		var f []float64
		json.Unmarshal(b, &f)
		trade.ID = int64(f[0])
		trade.MTS = int64(f[1])
		trade.Amount = f[2]
		trade.Price = f[3]

	case "tu": // Update（確定情報
		b, _, _, err := jsonparser.Get(b, "[2]")
		if err != nil {
			return err
		}
		var f []float64
		json.Unmarshal(b, &f)
		trade.ID = int64(f[0])
		trade.MTS = int64(f[1])
		trade.Amount = f[2]
		trade.Price = f[3]

	default: // case "hb":
		// heartbeat/5sec
		return errors.New("is heartbeat")
	}

	p.Executions = append(p.Executions, trade)
	p.ServerTime = time.Now()
	return nil
}
