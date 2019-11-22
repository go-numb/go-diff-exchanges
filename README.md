# go-diff-exchanges
This program gets price by each exchanges websocket, and insert influxDB. 


# Supported exchanges
[FTX Global Volume Monitor](https://ftx.com/volume-monitor)

### in Japan
Bitflyer 
Bitbank  
Coincheck 
Gmo

### outside
Liquid  
Huobi
Okex 
Ftx  

Binance 
Bitmex 
HitBit 

Bitfinex
Coinbase
Kraken 
ZB     
Bithumb 

# Usage
error when disconnect influxDB. 

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

# Auther
[@_numbP](https://twitter.com/_numbP)