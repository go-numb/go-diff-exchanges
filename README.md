# go-diff-exchanges
This program gets price by each exchanges websocket, and insert influxDB. 


# Supported exchanges
[FTX Global Volume Monitor](https://ftx.com/volume-monitor)

### in Japan
- [x] Bitflyer  
- [x] Bitbank  
- [x] Coincheck  
- [x] Gmo  

### outside
- [x] Liquid
- [x] Huobi
- [x] Okex
- [x] Ftx  

- [x] Binance
- [x] Bitmex
- [x] HitBit 

- [x] Bitfinex
- [x] Coinbase
- [x] Kraken
- [x] ZB 
- [x] Bithumb

### not include
- [ ] Deribit
- [ ] CME

# Usage
error when disconnect influxDB. 
Changes influxDB databsae and username/password.



``` golang
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-numb/go-diff-exchanges/exchanges"
	"github.com/sirupsen/logrus"
)

func main() {
	// 出力先を設定
	f, log := setter()
	defer f.Close()

	// 各取引所のクライアント生成
	client := exchanges.New(log)
	defer client.Close()

	done := make(chan struct{})

	go client.Connect()

	<-done
}

/*
    setting Logger
*/

func setter() (*os.File, *logrus.Logger) {
	log := logrus.New()
	osname := runtime.GOOS
	if !strings.HasPrefix(osname, "linux") { // developer
		log.SetLevel(logrus.DebugLevel)
		log.SetOutput(os.Stdout)
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
		return nil, log
	}

	abs, err := filepath.Abs(".")
	if err != nil {
		panic(err)
	}
	f, err := os.OpenFile(
		filepath.Join(abs, "logs", fmt.Sprintf("%s_error.log", time.Now().Format("02-Jan-2006"))),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0666)
	if err != nil {
		panic(err)
	}
	log.SetLevel(logrus.ErrorLevel)
	log.SetOutput(f)
	log.SetFormatter(&logrus.JSONFormatter{})

	return f, log
}
```

# in Exchange workers
``` go 

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
			time.Sleep(WSRECONNECTWAIT)
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
                            // Save to influxDB
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
		p.Logger.Fatal(err)
	}

}
```

# Author
[@_numbP](https://twitter.com/_numbP)