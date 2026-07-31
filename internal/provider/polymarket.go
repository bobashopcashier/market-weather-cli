package provider

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"
)

type MarketOutcome struct {
	Name    string   `json:"name"`
	Price   *float64 `json:"price"`
	TokenID string   `json:"tokenId,omitempty"`
}

type Market struct {
	ID        string          `json:"id"`
	Question  string          `json:"question"`
	Slug      string          `json:"slug"`
	Active    bool            `json:"active"`
	Closed    bool            `json:"closed"`
	Volume    float64         `json:"volume"`
	Liquidity float64         `json:"liquidity"`
	Outcomes  []MarketOutcome `json:"outcomes"`
}

type MarketEvent struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Slug      string   `json:"slug"`
	Active    bool     `json:"active"`
	Closed    bool     `json:"closed"`
	EndDate   string   `json:"endDate,omitempty"`
	Volume    float64  `json:"volume"`
	Liquidity float64  `json:"liquidity"`
	Markets   []Market `json:"markets"`
}

type MarketSearchResult struct {
	Source    string        `json:"source"`
	FetchedAt string        `json:"fetchedAt"`
	Query     string        `json:"query"`
	Events    []MarketEvent `json:"events"`
}

type rawSearch struct {
	Events []rawEvent `json:"events"`
}

type rawEvent struct {
	ID        any         `json:"id"`
	Title     string      `json:"title"`
	Slug      string      `json:"slug"`
	Active    bool        `json:"active"`
	Closed    bool        `json:"closed"`
	EndDate   string      `json:"endDate"`
	Volume    any         `json:"volume"`
	Liquidity any         `json:"liquidity"`
	Markets   []rawMarket `json:"markets"`
}

type rawMarket struct {
	ID           any             `json:"id"`
	Question     string          `json:"question"`
	Slug         string          `json:"slug"`
	Active       bool            `json:"active"`
	Closed       bool            `json:"closed"`
	Volume       any             `json:"volume"`
	VolumeNum    any             `json:"volumeNum"`
	Liquidity    any             `json:"liquidity"`
	LiquidityNum any             `json:"liquidityNum"`
	Outcomes     json.RawMessage `json:"outcomes"`
	Prices       json.RawMessage `json:"outcomePrices"`
	TokenIDs     json.RawMessage `json:"clobTokenIds"`
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		data, _ := json.Marshal(value)
		return string(data)
	}
}

func numberValue(values ...any) float64 {
	for _, value := range values {
		switch typed := value.(type) {
		case float64:
			return typed
		case json.Number:
			parsed, _ := typed.Float64()
			return parsed
		case string:
			parsed, err := strconv.ParseFloat(typed, 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}

func decodeStringList(raw json.RawMessage) []string {
	var direct []string
	if json.Unmarshal(raw, &direct) == nil {
		return direct
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		_ = json.Unmarshal([]byte(encoded), &direct)
	}
	return direct
}

func normalizeMarket(raw rawMarket) Market {
	outcomes := decodeStringList(raw.Outcomes)
	prices := decodeStringList(raw.Prices)
	tokens := decodeStringList(raw.TokenIDs)
	normalized := make([]MarketOutcome, 0, len(outcomes))
	for i, outcome := range outcomes {
		var price *float64
		if i < len(prices) {
			if parsed, err := strconv.ParseFloat(prices[i], 64); err == nil {
				price = &parsed
			}
		}
		token := ""
		if i < len(tokens) {
			token = tokens[i]
		}
		normalized = append(normalized, MarketOutcome{Name: outcome, Price: price, TokenID: token})
	}
	return Market{
		ID: stringValue(raw.ID), Question: raw.Question, Slug: raw.Slug, Active: raw.Active, Closed: raw.Closed,
		Volume: numberValue(raw.VolumeNum, raw.Volume), Liquidity: numberValue(raw.LiquidityNum, raw.Liquidity), Outcomes: normalized,
	}
}

func (c *Client) SearchMarkets(ctx context.Context, queryText string, limit int, includeClosed bool) (MarketSearchResult, error) {
	status := "active"
	if includeClosed {
		status = "all"
	}
	query := url.Values{
		"q": {queryText}, "events_status": {status}, "limit_per_type": {strconv.Itoa(limit)},
		"search_profiles": {"false"},
	}
	endpoint := "https://gamma-api.polymarket.com/public-search?" + query.Encode()
	var raw rawSearch
	if _, err := c.GetJSON(ctx, endpoint, nil, &raw, false); err != nil {
		return MarketSearchResult{}, err
	}
	events := make([]MarketEvent, 0, len(raw.Events))
	for _, event := range raw.Events {
		if !includeClosed && (event.Closed || !event.Active) {
			continue
		}
		if len(events) >= limit {
			break
		}
		markets := make([]Market, 0, len(event.Markets))
		for _, market := range event.Markets {
			if !includeClosed && market.Closed {
				continue
			}
			markets = append(markets, normalizeMarket(market))
		}
		events = append(events, MarketEvent{
			ID: stringValue(event.ID), Title: event.Title, Slug: event.Slug, Active: event.Active, Closed: event.Closed,
			EndDate: event.EndDate, Volume: numberValue(event.Volume), Liquidity: numberValue(event.Liquidity), Markets: markets,
		})
	}
	return MarketSearchResult{
		Source: "polymarket-public-api", FetchedAt: time.Now().UTC().Format(time.RFC3339), Query: queryText, Events: events,
	}, nil
}

type OrderBookResult struct {
	Source    string         `json:"source"`
	FetchedAt string         `json:"fetchedAt"`
	Book      map[string]any `json:"book"`
}

func (c *Client) GetOrderBook(ctx context.Context, tokenID string) (OrderBookResult, error) {
	endpoint := "https://clob.polymarket.com/book?" + url.Values{"token_id": {tokenID}}.Encode()
	book := map[string]any{}
	if _, err := c.GetJSON(ctx, endpoint, nil, &book, false); err != nil {
		return OrderBookResult{}, err
	}
	return OrderBookResult{Source: "polymarket-clob", FetchedAt: time.Now().UTC().Format(time.RFC3339), Book: book}, nil
}
