package kraken

import (
	"context"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/buger/jsonparser"

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
	WSPINGTIMER = 30 * time.Second
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// Client is structure
type Client struct {
	mux sync.Mutex

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
	Executions []Execution
	ServerTime time.Time
}

// Execution is trades
type Execution struct {
	TradeID   int
	Symbol    string
	OrderType string
	Price     float64
	Side      string
	Size      float64
	Time      time.Time
}

// Request set options
type Request struct {
	Event        string            `json:"event"`
	Pair         []string          `json:"pair"`
	Subscription map[string]string `json:"subscription"`
}

// Connect websocket
func (p *Client) Connect() {
	conn, _, err := websocket.DefaultDialer.Dial("wss://ws.kraken.com", nil)
	if err != nil {
		p.Logger.Fatal(err)
	}
	defer conn.Close()

	channels := []string{"trade"}
	symbols := []string{"BTC/USD"}
	for _, channel := range channels {
		if err := conn.WriteJSON(&Request{
			Event: "subscribe",
			Pair:  symbols,
			Subscription: map[string]string{
				"name": channel,
			},
		}); err != nil {
			p.Logger.Fatal(err)
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
				conn.WriteMessage(websocket.TextMessage, []byte(`{"event":"ping","reqid":42}`))

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
	p.mux.Lock()
	defer p.mux.Unlock()
	if !p.E.isThere() {
		return
	}
	p.E.Executions = []Execution{}
}

// LTP is 直近価格の出来高加重平均価格・出来高を取得
func (p *Client) LTP() (ltp, volume float64) {
	p.mux.Lock()
	defer p.mux.Unlock()
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
func (p *Client) Delay() (sec float64) {
	if !p.E.isThere() {
		return 0
	}
	return p.E.ServerTime.Sub(p.E.Executions[len(p.E.Executions)-1].Time).Seconds()
}

// EventTime is send exchange stamp
func (p *Client) EventTime() time.Time {
	if !p.E.isThere() {
		return time.Now()
	}
	return p.E.Executions[len(p.E.Executions)-1].Time
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
		ex []Execution
	)

	// 必要データ以外をreturn
	// 主にHeartBeat
	symbol, err := jsonparser.GetString(b, "[3]")
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		data, _, _, err := jsonparser.Get(b, "[1]")
		if err != nil {
			return
		}
		var str [][]string
		if err := json.Unmarshal(data, &str); err != nil {
			return
		}

		for _, v := range str {
			var (
				price float64
				size  float64
			)
			temp, err := strconv.ParseFloat(v[0], 64)
			if err != nil {
				continue
			}
			price = temp
			temp, err = strconv.ParseFloat(v[1], 64)
			if err != nil {
				continue
			}
			size = temp

			temp, err = strconv.ParseFloat(v[2], 64)
			if err != nil {
				continue
			}
			sec, dec := math.Modf(temp)
			fint := int64(dec * float64(1e9))
			t := time.Unix(int64(sec), fint)

			ex = append(ex, Execution{
				OrderType: v[4],
				Price:     price,
				Size:      size,
				Side:      v[3],
				Time:      t,
			})
		}

	}()
	wg.Wait()

	for i := range ex {
		ex[i].Symbol = symbol
	}

	p.Executions = append(p.Executions, ex...)
	p.ServerTime = time.Now().UTC()
	return nil
}
