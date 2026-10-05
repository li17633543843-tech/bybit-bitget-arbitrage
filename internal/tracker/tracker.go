package tracker

import (
	"fmt"
	"time"

	"github.com/li17633543843-tech/bybit-bitget-arbitrage/internal/scanner"
)

type Config struct {
	MinSamples  int
	MinDuration time.Duration
	ExpireAfter time.Duration
}

type Observation struct {
	Opportunity scanner.Opportunity `json:"opportunity"`
	FirstSeen   time.Time           `json:"first_seen"`
	LastSeen    time.Time           `json:"last_seen"`
	Samples     int                 `json:"samples"`
	Confirmed   bool                `json:"confirmed"`
}

type Tracker struct {
	config Config
	active map[string]Observation
}

func New(config Config) *Tracker {
	if config.MinSamples < 1 {
		config.MinSamples = 1
	}
	if config.ExpireAfter <= 0 {
		config.ExpireAfter = 15 * time.Second
	}
	return &Tracker{config: config, active: make(map[string]Observation)}
}

func (t *Tracker) Update(now time.Time, opportunities []scanner.Opportunity) []Observation {
	seen := make(map[string]struct{}, len(opportunities))
	confirmed := make([]Observation, 0, len(opportunities))
	for _, opportunity := range opportunities {
		key := opportunityKey(opportunity)
		seen[key] = struct{}{}
		observation, exists := t.active[key]
		if !exists || now.Sub(observation.LastSeen) > t.config.ExpireAfter {
			observation = Observation{FirstSeen: now}
		}
		observation.Opportunity = opportunity
		observation.LastSeen = now
		observation.Samples++
		observation.Confirmed = observation.Samples >= t.config.MinSamples && now.Sub(observation.FirstSeen) >= t.config.MinDuration
		t.active[key] = observation
		if observation.Confirmed {
			confirmed = append(confirmed, observation)
		}
	}

	for key, observation := range t.active {
		if _, ok := seen[key]; !ok && now.Sub(observation.LastSeen) > t.config.ExpireAfter {
			delete(t.active, key)
		}
	}
	return confirmed
}

func opportunityKey(opportunity scanner.Opportunity) string {
	return fmt.Sprintf("%s:%s:%s", opportunity.Symbol, opportunity.BuyExchange, opportunity.SellExchange)
}
