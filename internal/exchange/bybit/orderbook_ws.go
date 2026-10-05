package bybit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/orderbook"
)

const publicLinearWS = "wss://stream.bybit.com/v5/public/linear"

type BookFeed struct {
	URL string
}

func NewBookFeed() *BookFeed { return &BookFeed{URL: publicLinearWS} }

func (f *BookFeed) Run(ctx context.Context, symbols []string, output chan<- orderbook.Book) error {
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, f.URL, nil)
	if err != nil {
		return fmt.Errorf("connect bybit websocket: %w", err)
	}
	defer connection.Close()

	topics := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		topics = append(topics, "orderbook.50."+symbol)
	}
	if err := connection.WriteJSON(map[string]any{"op": "subscribe", "args": topics}); err != nil {
		return fmt.Errorf("subscribe bybit books: %w", err)
	}

	books := make(map[string]*mutableBook, len(symbols))
	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go bybitHeartbeat(connection, heartbeatDone)

	for {
		if err := connection.SetReadDeadline(time.Now().Add(45 * time.Second)); err != nil {
			return err
		}
		var message bybitBookMessage
		if err := connection.ReadJSON(&message); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read bybit book: %w", err)
		}
		if message.Topic == "" || message.Data.Symbol == "" {
			continue
		}
		book := books[message.Data.Symbol]
		if book == nil || message.Type == "snapshot" || message.Data.UpdateID == 1 {
			book = newMutableBook()
			books[message.Data.Symbol] = book
		}
		book.apply(message.Data.Bids, message.Data.Asks)
		select {
		case output <- book.snapshot("bybit", message.Data.Symbol, message.Data.Sequence, time.UnixMilli(message.Timestamp)):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

type bybitBookMessage struct {
	Topic     string `json:"topic"`
	Type      string `json:"type"`
	Timestamp int64  `json:"ts"`
	Data      struct {
		Symbol   string     `json:"s"`
		Bids     [][]string `json:"b"`
		Asks     [][]string `json:"a"`
		UpdateID int64      `json:"u"`
		Sequence int64      `json:"seq"`
	} `json:"data"`
}

type mutableBook struct {
	bids map[float64]float64
	asks map[float64]float64
}

func newMutableBook() *mutableBook {
	return &mutableBook{bids: make(map[float64]float64), asks: make(map[float64]float64)}
}

func (b *mutableBook) apply(bids, asks [][]string) {
	applyLevels(b.bids, bids)
	applyLevels(b.asks, asks)
}

func applyLevels(destination map[float64]float64, updates [][]string) {
	for _, level := range updates {
		if len(level) < 2 {
			continue
		}
		price, priceErr := strconv.ParseFloat(level[0], 64)
		size, sizeErr := strconv.ParseFloat(level[1], 64)
		if priceErr != nil || sizeErr != nil || price <= 0 {
			continue
		}
		if size == 0 {
			delete(destination, price)
		} else if size > 0 {
			destination[price] = size
		}
	}
}

func (b *mutableBook) snapshot(exchangeName, symbol string, sequence int64, updatedAt time.Time) orderbook.Book {
	book := orderbook.Book{Exchange: exchangeName, Symbol: symbol, Sequence: sequence, UpdatedAt: updatedAt}
	for price, size := range b.bids {
		book.Bids = append(book.Bids, orderbook.Level{Price: price, Size: size})
	}
	for price, size := range b.asks {
		book.Asks = append(book.Asks, orderbook.Level{Price: price, Size: size})
	}
	book.Normalize()
	return book
}

func bybitHeartbeat(connection *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = connection.WriteJSON(map[string]any{"op": "ping"})
		case <-done:
			return
		}
	}
}
