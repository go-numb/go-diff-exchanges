package exchanges

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/go-numb/go-diff-exchanges/exchanges/bithumb"
	"github.com/go-numb/go-diff-exchanges/exchanges/kraken"

	"github.com/go-numb/go-diff-exchanges/exchanges/coinbase"

	"github.com/go-numb/go-diff-exchanges/exchanges/hitbit"
	"github.com/go-numb/go-diff-exchanges/exchanges/zb"

	"github.com/go-numb/go-diff-exchanges/exchanges/huobi"

	"github.com/go-numb/go-diff-exchanges/exchanges/ftx"

	"github.com/go-numb/go-diff-exchanges/exchanges/bitbank"

	"github.com/go-numb/go-diff-exchanges/exchanges/coincheck"

	"golang.org/x/sync/errgroup"

	"github.com/go-numb/go-diff-exchanges/exchanges/binance"
	"github.com/go-numb/go-diff-exchanges/exchanges/bitfinex"
	"github.com/go-numb/go-diff-exchanges/exchanges/bitflyer"
	"github.com/go-numb/go-diff-exchanges/exchanges/bitmex"
	"github.com/go-numb/go-diff-exchanges/exchanges/gmo"
	"github.com/go-numb/go-diff-exchanges/exchanges/liquid"
	"github.com/go-numb/go-diff-exchanges/exchanges/okex"
	"github.com/sirupsen/logrus"

	infv2 "github.com/influxdata/influxdb1-client/v2"

	"gopkg.in/mgo.v2"
	"gopkg.in/redis.v3"
)

const (
	// TERM n秒足設定
	TERM = 500 * time.Millisecond

	// WSRECONNECTWAIT ウェブソケット再接続待機
	WSRECONNECTWAIT = 3 * time.Second
)

// Client is client structure
type Client struct {
	mongo  *mgo.Session
	redis  *redis.Client
	influx infv2.Client

	// Exchanges
	// 国内
	Bitflyer  *bitflyer.Client
	Bitbank   *bitbank.Client // リストラ: ws つながらず
	Coincheck *coincheck.Client
	Gmo       *gmo.Client
	// 一部海外（アジア？）
	Liquid *liquid.Client
	Okex   *okex.Client
	// 海外
	Huobi    *huobi.Client
	Binance  *binance.Client
	Bitmex   *bitmex.Client
	Bitfinex *bitfinex.Client
	Ftx      *ftx.Client
	HitBit   *hitbit.Client
	Coinbase *coinbase.Client
	Kraken   *kraken.Client
	ZB       *zb.Client
	Bithumb  *bithumb.Client

	Logger *logrus.Logger
}

// New is new client
func New(log *logrus.Logger) *Client {
	redigo := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	pong, err := redigo.Ping().Result()
	if err != nil {
		log.Fatal(err)
	}
	if pong == "" {
		log.Fatal("redis pong is nil")
	}

	inf, err := infv2.NewHTTPClient(infv2.HTTPConfig{
		Addr:     "http://localhost:8086",
		Username: "admin",
		Password: "admin",
	})
	if err != nil {
		log.Fatal(err)
	}

	return &Client{
		redis:  redigo,
		mongo:  nil,
		influx: inf,

		Bitflyer:  bitflyer.New(log.WithField("exchange", "bitflyer")),
		Bitbank:   bitbank.New(log.WithField("exchange", "bitbank")),
		Coincheck: coincheck.New(log.WithField("exchange", "coincheck")),
		Gmo:       gmo.New(log.WithField("exchange", "gmo")),
		Liquid:    liquid.New(log.WithField("exchange", "liquid")),

		Huobi: huobi.New(log.WithField("exchange", "huobi")),
		Okex:  okex.New(log.WithField("exchange", "okex")),

		Binance:  binance.New(log.WithField("exchange", "binance")),
		Bitmex:   bitmex.New(log.WithField("exchange", "bitmex")),
		Bitfinex: bitfinex.New(log.WithField("exchange", "bitfinex")),
		Ftx:      ftx.New(log.WithField("exchange", "ftx")),
		HitBit:   hitbit.New(log.WithField("exchange", "hitbit")),
		Coinbase: coinbase.New(log.WithField("exchange", "coinbase")),
		Kraken:   kraken.New(log.WithField("exchange", "kraken")),
		ZB:       zb.New(log.WithField("exchange", "zb")),
		Bithumb:  bithumb.New(log.WithField("exchange", "hithumb")),

		Logger: log,
	}
}

// Close close all client
func (p *Client) Close() error {
	if err := p.redis.Close(); err != nil {
		return err
	}
	if err := p.influx.Close(); err != nil {
		return err
	}
	return nil
}

// Execution is
// - 各取引所の合計・平均価格
// - 各取引所の合計・平均出来高
// -
type Execution struct {
	Prices  []float64
	Volumes []float64

	RecivedAt time.Time
}

// Connect is connect to each exchange websocket
func (p *Client) Connect() {
	exchanges := map[string]Exchange{
		"bitflyer":  p.Bitflyer,
		"bitbank":   p.Bitbank,
		"coincheck": p.Coincheck,
		"gmo":       p.Gmo,
		"liquid":    p.Liquid,

		"huobi":    p.Huobi,
		"okex":     p.Okex,
		"binance":  p.Binance,
		"bitmex":   p.Bitmex,
		"bitfinex": p.Bitfinex,
		"ftx":      p.Ftx,
		"hitBIT":   p.HitBit,
		"coinbase": p.Coinbase,
		"kraken":   p.Kraken,
		"zb":       p.ZB,
		"bithumb":  p.Bithumb,
	}

	var eg errgroup.Group

	for name, exchange := range exchanges {
		name := name
		exchange := exchange
		eg.Go(func() error {
		Reconnect:
			p.Logger.Infof("start connect websocket to %s", name)
			exchange.Connect()
			if name != "bitflyer" {
				time.Sleep(WSRECONNECTWAIT)
			} else {
				if time.Now().UTC().Hour() != 19 {
					time.Sleep(WSRECONNECTWAIT)
				} else { // JTC4時定期メンテ
					time.Sleep(12 * time.Minute)
				}
			}
			p.Logger.Errorf("stop connect websocket at %s", name)
			goto Reconnect
		})
	}

	eg.Go(func() error {
		ticker := time.NewTicker(TERM)
		defer ticker.Stop()

		var wg sync.WaitGroup

		for {
			select {
			case <-ticker.C:
				for name, exchange := range exchanges {
					wg.Add(1)
					go func(name string, exchange Exchange) {
						ltp, vol := exchange.LTP()
						if !math.IsNaN(ltp) {
							p.Logger.Infof(
								"%.4fs	%s	%.3f	%s",
								exchange.Delay(),
								toPrice(ltp),
								vol,
								name)
							p.save(name, exchange)
						}

						exchange.Reset()
						wg.Done()
					}(name, exchange)
				}
				wg.Wait()

			}
		}
	})

	if err := eg.Wait(); err != nil {
		p.Logger.Error(err)
		return
	}

}

func toPrice(x float64) string {
	return fmt.Sprintf("%.2f", x)
}

func (p *Client) save(name string, e Exchange) {
	bp, err := infv2.NewBatchPoints(infv2.BatchPointsConfig{
		Database:  "testdata",
		Precision: "s",
	})
	if err != nil {
		p.Logger.Error(err)
		return
	}

	ltp, vol := e.LTP()
	if math.IsNaN(ltp) {
		return
	}
	point, err := infv2.NewPoint(
		name,
		map[string]string{
			"tag1": "tag2",
		},
		map[string]interface{}{
			"price":       ltp,
			"volume":      vol,
			"delay":       e.Delay(),
			"event_time":  e.EventTime(),
			"server_time": e.ServerTime(),
		},
		e.EventTime())
	if err != nil {
		p.Logger.Error(err)
		return
	}
	bp.AddPoint(point)

	if err := p.influx.Write(bp); err != nil {
		p.Logger.Error(err)
		return
	}
}
