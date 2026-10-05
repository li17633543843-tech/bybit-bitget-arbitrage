package bitget

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/orderbook"
)

const publicFuturesWS = "wss://ws.bitget.com/v2/ws/public"

type BookFeed struct {
	URL string
}

func NewBookFeed() *BookFeed { return &BookFeed{URL: publicFuturesWS} }

func (f *BookFeed) Run(ctx context.Context, symbols []string, output chan<- orderbook.Book) error {
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, f.URL, nil)
	if err != nil {
		return fmt.Errorf("connect bitget websocket: %w", err)
	}
	defer connection.Close()

	for start := 0; start < len(symbols); start += 50 {
		end := start + 50
		if end > len(symbols) {
			end = len(symbols)
		}
		args := make([]map[string]string, 0, end-start)
		for _, symbol := range symbols[start:end] {
			args = append(args, map[string]string{"instType": "USDT-FUTURES", "channel": "books15", "instId": symbol})
		}
		if err := connection.WriteJSON(map[string]any{"op": "subscribe", "args": args}); err != nil {
			return fmt.Errorf("subscribe bitget books: %w", err)
		}
	}

	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go bitgetHeartbeat(connection, heartbeatDone)

	for {
		if err := connection.SetReadDeadline(time.Now().Add(45 * time.Second)); err != nil {
			return err
		}
		_, payload, err := connection.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read bitget book: %w", err)
		}
		if string(payload) == "pong" {
			continue
		}
		var message bitgetBookMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			return fmt.Errorf("decode bitget book: %w", err)
		}
		if message.Arg.Symbol == "" || len(message.Data) == 0 {
			continue
		}
		for _, data := range message.Data {
			updatedAtMillis := parseInt64(data.Timestamp)
			if updatedAtMillis == 0 {
				updatedAtMillis = message.Timestamp
			}
			book := orderbook.Book{
				Exchange:  "bitget",
				Symbol:    message.Arg.Symbol,
				Bids:      parseBitgetLevels(data.Bids),
				Asks:      parseBitgetLevels(data.Asks),
				UpdatedAt: time.UnixMilli(updatedAtMillis),
			}
			book.Normalize()
			select {
			case output <- book:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

type bitgetBookMessage struct {
	Action    string `json:"action"`
	Timestamp int64  `json:"ts"`
	Arg    struct {
		Symbol string `json:"instId"`
	} `json:"arg"`
	Data []struct {
		Bids      [][]string `json:"bids"`
		Asks      [][]string `json:"asks"`
		Timestamp string     `json:"ts"`
	} `json:"data"`
}

func parseBitgetLevels(raw [][]string) []orderbook.Level {
	levels := make([]orderbook.Level, 0, len(raw))
	for _, item := range raw {
		if len(item) < 2 {
			continue
		}
		price, priceErr := strconv.ParseFloat(item[0], 64)
		size, sizeErr := strconv.ParseFloat(item[1], 64)
		if priceErr == nil && sizeErr == nil && price > 0 && size > 0 {
			levels = append(levels, orderbook.Level{Price: price, Size: size})
		}
	}
	return levels
}

func parseInt64(raw string) int64 {
	value, _ := strconv.ParseInt(raw, 10, 64)
	return value
}

func bitgetHeartbeat(connection *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = connection.WriteMessage(websocket.TextMessage, []byte("ping"))
		case <-done:
			return
		}
	}
}
