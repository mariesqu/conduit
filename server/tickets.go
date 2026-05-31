package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ticketTTL is how long a freshly issued ticket stays valid. Short on
// purpose: a ticket only needs to survive the round-trip from "client
// asks for one" to "client opens the WebSocket / starts the download".
const ticketTTL = 30 * time.Second

// TicketManager issues short-lived, single-use-ish tokens that stand in
// for the long-lived auth token on the two request paths that can only
// carry credentials in the URL: the WebSocket upgrade (browsers can't
// set headers on it) and plain <a download> links.
//
// Why this exists: query strings get written to reverse-proxy access
// logs and browser history. Putting the *permanent* token there means a
// single logged line is permanent shell access. A ticket in the URL is
// worthless ~30 seconds later, so the long-lived token never has to
// appear in a loggable position during normal use.
//
// Tickets are reusable within their TTL (not strictly single-use) so
// that HTTP range requests and download retries — which reuse the same
// URL — don't fail. The TTL is the security boundary, not single-use.
type TicketManager struct {
	mu      sync.Mutex
	tickets map[string]time.Time // token → expiry
	stop    chan struct{}
}

func NewTicketManager() *TicketManager {
	m := &TicketManager{
		tickets: make(map[string]time.Time),
		stop:    make(chan struct{}),
	}
	go m.sweepLoop()
	return m
}

// Issue creates a new ticket valid for ticketTTL and returns its token.
func (m *TicketManager) Issue() string {
	tok := generateTicket()
	m.mu.Lock()
	m.tickets[tok] = time.Now().Add(ticketTTL)
	m.mu.Unlock()
	return tok
}

// Valid reports whether the ticket exists and has not expired. Expired
// tickets are removed lazily on lookup.
func (m *TicketManager) Valid(tok string) bool {
	if tok == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	exp, ok := m.tickets[tok]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(m.tickets, tok)
		return false
	}
	return true
}

// Shutdown stops the background sweeper. Idempotent.
func (m *TicketManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
}

func (m *TicketManager) sweepLoop() {
	t := time.NewTicker(ticketTTL)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
			now := time.Now()
			m.mu.Lock()
			for tok, exp := range m.tickets {
				if now.After(exp) {
					delete(m.tickets, tok)
				}
			}
			m.mu.Unlock()
		}
	}
}

func generateTicket() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("crypto/rand failed: %w", err))
	}
	return hex.EncodeToString(b)
}
