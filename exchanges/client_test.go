package exchanges

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestConnect(t *testing.T) {
	client := New(logrus.New())
	defer client.Close()

	client.Connect()
}
