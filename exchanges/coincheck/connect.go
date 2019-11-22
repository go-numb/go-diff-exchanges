package coincheck

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

	// WSPINGTIMER PING確認
	WSPINGTIMER = 10 * time.Second
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

// Execution is struct
type Execution struct {
	ID     int
	Symbol string
	Price  float64
	Size   float64
	Side   string
}

// JSONRPC2 is request param
type JSONRPC2 struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
}

// Connect websocket
func (p *Client) Connect() {
	conn, _, err := websocket.DefaultDialer.Dial("wss://ws-api.coincheck.com/", nil)
	if err != nil {
		p.Logger.Fatal(err)
	}
	defer conn.Close()

	channels := []string{"btc_jpy-trades"} //, "eth_jpy-trades", "xrp_jpy-trades"} //, "btc_jpy-orderbook"}
	for _, channel := range channels {
		if err := conn.WriteJSON(
			&JSONRPC2{
				Type:    "subscribe",
				Channel: channel,
			},
		); err != nil {
			p.Logger.Fatal(err)
		}
	}

	// 伝搬キャンセル
	ctx, cancel := context.WithCancel(context.Background())
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		go func() {
			defer cancel()
			for {
				if err := conn.SetReadDeadline(time.Now().Add(WSREADDEADLINE)); err != nil {
					break
				}
				_, msg, err := conn.ReadMessage()
				if err != nil {
					break
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
		}()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})

	eg.Go(func() error {
		defer cancel()
		for {
			ticker := time.NewTicker(WSPINGTIMER)
			defer ticker.Stop()

			select {
			case <-ticker.C:
				if err := conn.WriteMessage(
					websocket.TextMessage,
					[]byte(fmt.Sprintf("%d", time.Now().UTC().Unix()))); err != nil {
					return err
				}

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
	for i, e := range use {
		prices[i] = e.Price
		volumes[i] = e.Size
		volume += e.Size
	}
	return stat.Mean(prices, volumes), volume
}

// Delay is 直近価格の出来高加重平均価格・出来高を取得
// Coincheckはタイムスタンプがない
func (p *Client) Delay() (sec float64) {
	if !p.E.isThere() {
		return 0
	}
	return 0
}

// EventTime is send exchange stamp
func (p *Client) EventTime() time.Time {
	if !p.E.isThere() {
		return time.Now()
	}
	return time.Now()
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
	var (
		wg sync.WaitGroup
		e  Execution
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		id, err := jsonparser.GetInt(b, "[0]")
		if err != nil {
			return
		}
		e.ID = int(id)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		symbol, err := jsonparser.GetString(b, "[1]")
		if err != nil {
			return
		}
		e.Symbol = symbol
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		price, err := jsonparser.GetString(b, "[2]")
		if err != nil {
			return
		}
		e.Price, _ = strconv.ParseFloat(price, 64)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		size, err := jsonparser.GetString(b, "[3]")
		if err != nil {
			return
		}
		e.Size, _ = strconv.ParseFloat(size, 64)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		side, err := jsonparser.GetString(b, "[4]")
		if err != nil {
			return
		}
		e.Side = side
	}()
	wg.Wait()

	if e.ID == 0 || e.Symbol != "btc_jpy" {
		return errors.New("undefined type")
	}

	p.Executions = append(p.Executions, e)
	p.ServerTime = time.Now().UTC()
	return nil
}
