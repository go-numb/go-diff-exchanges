package gmo

import (
	"context"
	"sync"
	"time"

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
	WSPINGTIMER = 150 * time.Second
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

// Execution is gmoExecutionを内包したwebsocket用struct
type Execution struct {
	Price     float64   `json:"price,string"`
	Side      string    `json:"side"`
	Size      float64   `json:"size,string"`
	Timestamp time.Time `json:"timestamp"`
	Channel   string    `json:"channel"`
	Symbol    string    `json:"symbol"`
}

// Request is json writer
type Request struct {
	Command string `json:"command"`
	Channel string `json:"channel"`
	Symbol  string `json:"symbol"`
}

// Connect websocket
func (p *Client) Connect() {
	conn, _, err := websocket.DefaultDialer.Dial("wss://api.coin.z.com/ws/public/v1", nil)
	if err != nil {
		p.Logger.Fatal(err)
	}
	defer conn.Close()

	channels := []string{"trades"}
	symbols := []string{"BTC_JPY"}

	for _, channel := range channels {
		for _, symbol := range symbols {
			if err := conn.WriteJSON(
				&Request{
					Command: "subscribe",
					Channel: channel,
					Symbol:  symbol,
				},
			); err != nil {
				p.Logger.Fatal(err)
			}
			time.Sleep(time.Second)
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
				if err := conn.WriteMessage(websocket.TextMessage, []byte(`2`)); err != nil {
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
		volumes[i] = e.Size
		volume += e.Size
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

func (p *E) set(data []byte) error {
	var ex Execution
	json.Unmarshal(data, &ex)
	p.Executions = append(p.Executions, ex)
	p.ServerTime = time.Now().UTC()
	return nil
}
