package orderbook

import "sync"

type Store struct {
	mu    sync.RWMutex
	books map[string]Book
}

func NewStore() *Store { return &Store{books: make(map[string]Book)} }

func (s *Store) Put(book Book) {
	book.Normalize()
	s.mu.Lock()
	s.books[book.Exchange+":"+book.Symbol] = cloneBook(book)
	s.mu.Unlock()
}

func (s *Store) Get(exchange, symbol string) (Book, bool) {
	s.mu.RLock()
	book, ok := s.books[exchange+":"+symbol]
	s.mu.RUnlock()
	return cloneBook(book), ok
}

func cloneBook(book Book) Book {
	book.Bids = append([]Level(nil), book.Bids...)
	book.Asks = append([]Level(nil), book.Asks...)
	return book
}
