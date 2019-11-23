package liquid

import (
	"fmt"
	"sync"
	"time"

	"github.com/buger/jsonparser"

	"github.com/go-numb/go-liquid"

	"gonum.org/v1/gonum/stat"

	"github.com/json-iterator/go"

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

// Execution is websocket tap.liquid.com
type Execution struct {
	ID        int     `json:"id"`
	Quantity  float64 `json:"quantity"`
	Price     float64 `json:"price"`
	TakerSide string  `json:"taker_side"`
	CreatedAt int64   `json:"created_at"`
}

// Connect websocket
func (p *Client) Connect() {
	conn, _, err := websocket.DefaultDialer.Dial("wss://tap.liquid.com/app/LiquidTapClient", nil)
	if err != nil {
		p.Logger.Error(err)
return
	}
	defer conn.Close()

	channels := []string{fmt.Sprintf(liquid.PUSHERchEXECUTION, liquid.BTCJPY)}
	for _, channel := range channels {
		if err := conn.WriteMessage(
			websocket.TextMessage,
			[]byte(fmt.Sprintf(
				`{"event":"pusher:subscribe","data":{"channel":"%s"}}`,
				channel)),
		); err != nil {
			p.Logger.Error(err)
return
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

			// // name, _, _, err := jsonparser.Get(msg, "channel")
			// // if err != nil {
			// // 	continue
			// // }

			if err := p.E.set(msg); err != nil {
				continue
			}
			// // fmt.Printf("%+v - %.f - %.4f\n", p.E.Executions[0].ID, p.E.Executions[0].Price, p.E.ServerTime.Sub(p.E.Executions[0].ExecDate.Time).Seconds())
			// // ltp, volume := p.LTP()
			// // fmt.Printf("%.f - %.4f - %.4fsec\n", ltp, volume, p.Delay())
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
		volumes[i] = e.Quantity
		volume += e.Quantity
	}
	return stat.Mean(prices, volumes), volume
}

// Delay is 直近価格の出来高加重平均価格・出来高を取得
func (p *Client) Delay() (sec float64) {
	if !p.E.isThere() {
		return 0
	}
	t := time.Unix(int64(p.E.Executions[len(p.E.Executions)-1].CreatedAt), 10)
	return p.E.ServerTime.Sub(t).Seconds()
}

// EventTime is send exchange stamp
func (p *Client) EventTime() time.Time {
	if !p.E.isThere() {
		return time.Now()
	}
	return time.Unix(int64(p.E.Executions[len(p.E.Executions)-1].CreatedAt), 10)
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
	// escape backslash
	str, err := jsonparser.GetString(b, "data")
	if err != nil {
		return err
	}
	var ex Execution
	if err := json.Unmarshal([]byte(str), &ex); err != nil {
		return err
	}
	p.Executions = append(p.Executions, ex)
	p.ServerTime = time.Now().UTC()
	return nil
}
