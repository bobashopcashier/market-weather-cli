package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	MaximumMarketsPerEvent = 100
	MaximumOrderBookLevels = 500
)

type Truncation struct {
	Path         string `json:"path"`
	ParentID     string `json:"parentId,omitempty"`
	SourceCount  int    `json:"sourceCount"`
	EmittedCount int    `json:"emittedCount"`
}

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
	Volume    *float64        `json:"volume,omitempty"`
	Liquidity *float64        `json:"liquidity,omitempty"`
	Outcomes  []MarketOutcome `json:"outcomes"`
}

type MarketEvent struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Slug      string   `json:"slug"`
	Active    bool     `json:"active"`
	Closed    bool     `json:"closed"`
	EndDate   string   `json:"endDate,omitempty"`
	Volume    *float64 `json:"volume,omitempty"`
	Liquidity *float64 `json:"liquidity,omitempty"`
	Markets   []Market `json:"markets"`
}

type MarketSearchResult struct {
	Source     string        `json:"source"`
	FetchedAt  string        `json:"fetchedAt"`
	Query      string        `json:"query"`
	Events     []MarketEvent `json:"events"`
	Truncation []Truncation  `json:"truncation,omitempty"`
}

type rawSearch struct {
	Events []rawEvent `json:"events"`
}

type rawEvent struct {
	ID        json.RawMessage `json:"id"`
	Title     string          `json:"title"`
	Slug      string          `json:"slug"`
	Active    bool            `json:"active"`
	Closed    bool            `json:"closed"`
	EndDate   string          `json:"endDate,omitempty"`
	Volume    json.RawMessage `json:"volume,omitempty"`
	Liquidity json.RawMessage `json:"liquidity,omitempty"`
	Markets   []rawMarket     `json:"markets"`
}

type rawMarket struct {
	ID           json.RawMessage `json:"id"`
	Question     string          `json:"question"`
	Slug         string          `json:"slug"`
	Active       bool            `json:"active"`
	Closed       bool            `json:"closed"`
	Volume       json.RawMessage `json:"volume,omitempty"`
	VolumeNum    json.RawMessage `json:"volumeNum,omitempty"`
	Liquidity    json.RawMessage `json:"liquidity,omitempty"`
	LiquidityNum json.RawMessage `json:"liquidityNum,omitempty"`
	Outcomes     json.RawMessage `json:"outcomes"`
	Prices       json.RawMessage `json:"outcomePrices,omitempty"`
	TokenIDs     json.RawMessage `json:"clobTokenIds,omitempty"`
}

func normalizePolymarketSearch(raw rawSearch, includeClosed bool) ([]MarketEvent, schemaValidationResult) {
	collector := newSchemaValidationCollector()
	events := make([]MarketEvent, 0, len(raw.Events))
	for _, event := range raw.Events {
		if !includeClosed && (event.Closed || !event.Active) {
			continue
		}
		eventPath := "events[]"
		markets := make([]Market, 0, len(event.Markets))
		for _, market := range event.Markets {
			if !includeClosed && market.Closed {
				continue
			}
			markets = append(markets, normalizeMarket(market, eventPath+".markets[]", collector))
		}
		var volume *float64
		if len(event.Volume) > 0 {
			value := normalizePolymarketNumber(event.Volume, eventPath+".volume", collector)
			volume = &value
		}
		var liquidity *float64
		if len(event.Liquidity) > 0 {
			value := normalizePolymarketNumber(event.Liquidity, eventPath+".liquidity", collector)
			liquidity = &value
		}
		events = append(events, MarketEvent{
			ID:        normalizePolymarketID(event.ID, eventPath+".id", collector),
			Title:     event.Title,
			Slug:      event.Slug,
			Active:    event.Active,
			Closed:    event.Closed,
			EndDate:   event.EndDate,
			Volume:    volume,
			Liquidity: liquidity,
			Markets:   markets,
		})
	}
	return events, collector.result()
}

func normalizeMarket(raw rawMarket, path string, collector *schemaValidationCollector) Market {
	outcomes, outcomesValid := normalizePolymarketStringList(raw.Outcomes, path+".outcomes", collector)
	prices := []string(nil)
	pricesValid := false
	if len(raw.Prices) > 0 {
		prices, pricesValid = normalizePolymarketStringList(raw.Prices, path+".outcomePrices", collector)
	}
	tokens := []string(nil)
	tokensValid := true
	if len(raw.TokenIDs) > 0 {
		tokens, tokensValid = normalizePolymarketStringList(raw.TokenIDs, path+".clobTokenIds", collector)
	}
	if outcomesValid && pricesValid && len(outcomes) != len(prices) {
		collector.addTypeMismatch(path+".outcomePrices", "same length as outcomes", "different length")
	}
	if outcomesValid && len(outcomes) == 0 {
		collector.addTypeMismatch(path+".outcomes", "non-empty array of strings", "array")
	}
	for _, outcome := range outcomes {
		if strings.TrimSpace(outcome) == "" {
			collector.addTypeMismatch(path+".outcomes[]", "non-empty string", "string")
		}
	}
	if outcomesValid && tokensValid && raw.TokenIDs != nil && len(outcomes) != len(tokens) {
		collector.addTypeMismatch(path+".clobTokenIds", "same length as outcomes", "different length")
	}
	for _, token := range tokens {
		if !decimalTokenPattern.MatchString(token) {
			collector.addTypeMismatch(path+".clobTokenIds[]", "decimal string of at most 128 digits", "string")
		}
	}

	normalized := make([]MarketOutcome, 0, len(outcomes))
	for i, outcome := range outcomes {
		var price *float64
		if i < len(prices) {
			if parsed, ok := parseFiniteNumber(prices[i]); ok && parsed >= 0 && parsed <= 1 {
				price = &parsed
			} else {
				collector.addTypeMismatch(path+".outcomePrices[]", "numeric string from 0 to 1", "string")
			}
		}
		token := ""
		if i < len(tokens) {
			token = tokens[i]
		}
		normalized = append(normalized, MarketOutcome{Name: outcome, Price: price, TokenID: token})
	}
	var volume *float64
	if len(raw.Volume) > 0 {
		value := normalizePolymarketNumber(raw.Volume, path+".volume", collector)
		volume = &value
	}
	if len(raw.VolumeNum) > 0 {
		value := normalizePolymarketNumber(raw.VolumeNum, path+".volumeNum", collector)
		volume = &value
	}
	var liquidity *float64
	if len(raw.Liquidity) > 0 {
		value := normalizePolymarketNumber(raw.Liquidity, path+".liquidity", collector)
		liquidity = &value
	}
	if len(raw.LiquidityNum) > 0 {
		value := normalizePolymarketNumber(raw.LiquidityNum, path+".liquidityNum", collector)
		liquidity = &value
	}
	return Market{
		ID: normalizePolymarketID(raw.ID, path+".id", collector), Question: raw.Question, Slug: raw.Slug, Active: raw.Active, Closed: raw.Closed,
		Volume: volume, Liquidity: liquidity, Outcomes: normalized,
	}
}

func normalizePolymarketID(raw json.RawMessage, path string, collector *schemaValidationCollector) string {
	value, ok := decodePolymarketValue(raw)
	if !ok {
		collector.addMissingField(path)
		return ""
	}
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			return typed
		}
		collector.addTypeMismatch(path, "non-empty string or non-negative integer", "string")
	case json.Number:
		integer := new(big.Int)
		if _, valid := integer.SetString(string(typed), 10); valid && integer.Sign() >= 0 {
			return string(typed)
		}
		collector.addTypeMismatch(path, "non-empty string or non-negative integer", "number")
	default:
		collector.addTypeMismatch(path, "non-empty string or non-negative integer", jsonValueType(value))
	}
	return ""
}

func normalizePolymarketNumber(raw json.RawMessage, path string, collector *schemaValidationCollector) float64 {
	value, ok := decodePolymarketValue(raw)
	if !ok {
		collector.addMissingField(path)
		return 0
	}
	return normalizePolymarketNumberValue(value, path, collector)
}

func normalizePolymarketNumberValue(value any, path string, collector *schemaValidationCollector) float64 {
	var text string
	switch typed := value.(type) {
	case json.Number:
		text = string(typed)
	case string:
		text = typed
	default:
		collector.addTypeMismatch(path, "non-negative finite number or numeric string", jsonValueType(value))
		return 0
	}
	parsed, valid := parseFiniteNumber(text)
	if !valid || parsed < 0 {
		collector.addTypeMismatch(path, "non-negative finite number or numeric string", jsonValueType(value))
		return 0
	}
	return parsed
}

func normalizePolymarketStringList(raw json.RawMessage, path string, collector *schemaValidationCollector) ([]string, bool) {
	value, ok := decodePolymarketValue(raw)
	if !ok {
		collector.addMissingField(path)
		return nil, false
	}
	if encoded, isString := value.(string); isString {
		value, ok = decodePolymarketValue(json.RawMessage(encoded))
		if !ok {
			collector.addTypeMismatch(path, "array of strings or JSON-encoded array of strings", "string")
			return nil, false
		}
	}
	items, ok := value.([]any)
	if !ok {
		collector.addTypeMismatch(path, "array of strings or JSON-encoded array of strings", jsonValueType(value))
		return nil, false
	}
	result := make([]string, 0, len(items))
	valid := true
	for _, item := range items {
		text, isString := item.(string)
		if !isString {
			collector.addTypeMismatch(path+"[]", "string", jsonValueType(item))
			valid = false
			continue
		}
		result = append(result, text)
	}
	if !valid {
		return nil, false
	}
	return result, valid
}

func decodePolymarketValue(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	return value, decoder.Decode(&struct{}{}) == io.EOF
}

func parseFiniteNumber(value string) (float64, bool) {
	text := strings.TrimSpace(value)
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return 0, false
	}
	if parsed == 0 {
		exact, _, err := big.ParseFloat(text, 10, 256, big.ToNearestEven)
		if err != nil || exact.Sign() != 0 {
			return 0, false
		}
	}
	return parsed, true
}

func (c *Client) SearchMarkets(ctx context.Context, queryText string, limit int, includeClosed bool) (MarketSearchResult, error) {
	queryText, err := validateFreeText("market search query", queryText, 512)
	if err != nil {
		return MarketSearchResult{}, err
	}
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
	normalizedEvents, validation := normalizePolymarketSearch(raw, includeClosed)
	if !validation.valid() {
		return MarketSearchResult{}, upstreamSchemaMismatchError(endpoint, validation)
	}
	events := make([]MarketEvent, 0, len(normalizedEvents))
	truncation := []Truncation{}
	eligibleEvents := len(normalizedEvents)
	if eligibleEvents > limit {
		truncation = append(truncation, Truncation{Path: "events", SourceCount: eligibleEvents, EmittedCount: limit})
	}
	for _, event := range normalizedEvents {
		if len(events) >= limit {
			break
		}
		markets := make([]Market, 0, min(len(event.Markets), MaximumMarketsPerEvent))
		eligibleMarkets := 0
		for _, market := range event.Markets {
			eligibleMarkets++
			if len(markets) < MaximumMarketsPerEvent {
				markets = append(markets, market)
			}
		}
		if eligibleMarkets > len(markets) {
			truncation = append(truncation, Truncation{Path: "events[].markets", ParentID: event.ID, SourceCount: eligibleMarkets, EmittedCount: len(markets)})
		}
		event.Markets = markets
		events = append(events, event)
	}
	return MarketSearchResult{
		Source: "polymarket-public-api", FetchedAt: time.Now().UTC().Format(time.RFC3339), Query: queryText, Events: events, Truncation: truncation,
	}, nil
}

type OrderBookResult struct {
	Source     string         `json:"source"`
	FetchedAt  string         `json:"fetchedAt"`
	Book       map[string]any `json:"book"`
	Truncation []Truncation   `json:"truncation,omitempty"`
}

func (c *Client) GetOrderBook(ctx context.Context, tokenID string) (OrderBookResult, error) {
	tokenID, err := validateDecimalToken(tokenID)
	if err != nil {
		return OrderBookResult{}, err
	}
	endpoint := "https://clob.polymarket.com/book?" + url.Values{"token_id": {tokenID}}.Encode()
	rawBook := map[string]json.RawMessage{}
	if _, err := c.GetJSON(ctx, endpoint, nil, &rawBook, false); err != nil {
		return OrderBookResult{}, err
	}
	book, validation := normalizePolymarketOrderBook(rawBook)
	if !validation.valid() {
		return OrderBookResult{}, upstreamSchemaMismatchError(endpoint, validation)
	}
	truncation := truncateOrderBook(book)
	return OrderBookResult{Source: "polymarket-clob", FetchedAt: time.Now().UTC().Format(time.RFC3339), Book: book, Truncation: truncation}, nil
}

func normalizePolymarketOrderBook(raw map[string]json.RawMessage) (map[string]any, schemaValidationResult) {
	collector := newSchemaValidationCollector()
	book := make(map[string]any, len(raw))
	for key, encoded := range raw {
		value, ok := decodePolymarketValue(encoded)
		if !ok {
			collector.addTypeMismatch("book.*", "valid JSON value", "invalid")
			continue
		}
		book[key] = value
	}
	for _, side := range []string{"bids", "asks"} {
		path := "book." + side
		value, present := book[side]
		if !present {
			collector.addMissingField(path)
			continue
		}
		levels, ok := value.([]any)
		if !ok {
			collector.addTypeMismatch(path, "array", jsonValueType(value))
			continue
		}
		for _, level := range levels {
			levelPath := path + "[]"
			object, ok := level.(map[string]any)
			if !ok {
				collector.addTypeMismatch(levelPath, "object", jsonValueType(level))
				continue
			}
			for _, field := range []string{"price", "size"} {
				fieldPath := levelPath + "." + field
				fieldValue, present := object[field]
				if !present {
					collector.addMissingField(fieldPath)
					continue
				}
				normalized := normalizePolymarketNumberValue(fieldValue, fieldPath, collector)
				if field == "price" && normalized > 1 {
					collector.addTypeMismatch(fieldPath, "number from 0 to 1", jsonValueType(fieldValue))
				}
			}
		}
	}
	return book, collector.result()
}

func truncateOrderBook(book map[string]any) []Truncation {
	truncation := []Truncation{}
	for _, side := range []string{"bids", "asks"} {
		levels, ok := book[side].([]any)
		if !ok || len(levels) <= MaximumOrderBookLevels {
			continue
		}
		book[side] = levels[:MaximumOrderBookLevels]
		truncation = append(truncation, Truncation{Path: "book." + side, SourceCount: len(levels), EmittedCount: MaximumOrderBookLevels})
	}
	return truncation
}
