package ibkr

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/broker"
	"github.com/tathienbao/quant-bot/internal/types"
	"golang.org/x/time/rate"
)

// IB API message IDs.
const (
	msgTickPrice        = 1
	msgTickSize         = 2
	msgAccountSummary   = 63
	msgAccountSummaryEnd = 64
	msgPosition         = 61
	msgPositionEnd      = 62
)

// Client implements the broker.Broker interface for IBKR.
type Client struct {
	cfg    Config
	logger *slog.Logger

	// Connection
	conn       net.Conn
	state      atomic.Int32
	stateMu    sync.RWMutex
	lastError  error
	connectedAt time.Time

	// Rate limiting
	limiter *rate.Limiter

	// Request tracking
	nextReqID atomic.Int64
	requests  map[int64]chan any

	// Market data subscriptions
	mdMu          sync.RWMutex
	mdSubscriptions map[string]*marketDataSubscription

	// Account data
	accountMu sync.RWMutex
	account   *broker.AccountSummary

	// Positions
	positionsMu sync.RWMutex
	positions   map[string]*broker.Position

	// Orders
	ordersMu sync.RWMutex
	orders   map[string]*broker.Order

	// Shutdown
	done     chan struct{}
	wg       sync.WaitGroup
}

type marketDataSubscription struct {
	symbol    string
	tickerID  int64
	ch        chan types.MarketEvent
	lastEvent types.MarketEvent
}

// NewClient creates a new IBKR client.
func NewClient(cfg Config, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}

	c := &Client{
		cfg:             cfg,
		logger:          logger,
		limiter:         rate.NewLimiter(rate.Limit(cfg.MaxRequestsPerSecond), cfg.MaxRequestsPerSecond),
		requests:        make(map[int64]chan any),
		mdSubscriptions: make(map[string]*marketDataSubscription),
		positions:       make(map[string]*broker.Position),
		orders:          make(map[string]*broker.Order),
		done:            make(chan struct{}),
	}

	c.state.Store(int32(broker.StateDisconnected))
	c.nextReqID.Store(1000)

	return c
}

// Connect establishes connection to TWS/Gateway.
func (c *Client) Connect(ctx context.Context) error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.State() == broker.StateConnected {
		return nil
	}

	c.state.Store(int32(broker.StateConnecting))

	c.logger.Info("connecting to IBKR",
		"host", c.cfg.Host,
		"port", c.cfg.Port,
		"client_id", c.cfg.ClientID,
		"paper", c.cfg.PaperTrading,
	)

	// Create connection with timeout
	addr := fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)

	dialer := net.Dialer{
		Timeout: c.cfg.ConnectTimeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		c.state.Store(int32(broker.StateError))
		c.lastError = fmt.Errorf("dial: %w", err)
		return fmt.Errorf("%w: %v", broker.ErrConnectionTimeout, err)
	}

	c.conn = conn
	c.connectedAt = time.Now()

	// Perform IB API handshake
	if err := c.handshake(); err != nil {
		_ = conn.Close()
		c.state.Store(int32(broker.StateError))
		c.lastError = err
		return fmt.Errorf("handshake: %w", err)
	}

	c.state.Store(int32(broker.StateConnected))

	// Start message reader
	c.wg.Add(1)
	go c.readLoop()

	// Request initial data
	if err := c.requestInitialData(ctx); err != nil {
		c.logger.Warn("failed to request initial data", "err", err)
	}

	c.logger.Info("connected to IBKR",
		"connected_at", c.connectedAt,
	)

	return nil
}

// handshake performs the IB API connection handshake.
func (c *Client) handshake() error {
	// IB API v100+ protocol handshake
	// Send: "API\0" + version string
	handshake := []byte("API\x00")

	// Send min/max version range
	versionStr := fmt.Sprintf("v%d..%d", 100, 151) // Support IB API versions
	handshake = append(handshake, []byte(versionStr)...)
	handshake = append(handshake, 0) // null terminator

	if _, err := c.conn.Write(handshake); err != nil {
		return fmt.Errorf("write handshake: %w", err)
	}

	// Read server response
	buf := make([]byte, 1024)
	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := c.conn.Read(buf)
	_ = c.conn.SetReadDeadline(time.Time{})

	if err != nil {
		return fmt.Errorf("read handshake response: %w", err)
	}

	c.logger.Debug("handshake response", "bytes", n, "data", string(buf[:n]))

	// Send startAPI message
	startAPI := c.buildStartAPIMessage(c.cfg.ClientID)
	if _, err := c.conn.Write(startAPI); err != nil {
		return fmt.Errorf("write startAPI: %w", err)
	}

	return nil
}

// buildStartAPIMessage creates the startAPI message.
func (c *Client) buildStartAPIMessage(clientID int) []byte {
	// Message format: size (4 bytes) + message ID + version + clientID
	msg := fmt.Sprintf("71\x002\x00%d\x00", clientID)
	size := len(msg)

	// Prepend size as 4-byte big-endian
	result := make([]byte, 4+size)
	result[0] = byte(size >> 24)
	result[1] = byte(size >> 16)
	result[2] = byte(size >> 8)
	result[3] = byte(size)
	copy(result[4:], msg)

	return result
}

// readLoop reads messages from the connection.
func (c *Client) readLoop() {
	defer c.wg.Done()

	buf := make([]byte, 65536)
	for {
		select {
		case <-c.done:
			return
		default:
		}

		_ = c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := c.conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			c.logger.Error("read error", "err", err)
			c.handleDisconnect()
			return
		}

		if n > 0 {
			c.processMessage(buf[:n])
		}
	}
}

// processMessage processes an incoming message.
func (c *Client) processMessage(data []byte) {
	// Parse IB API message - fields separated by null bytes
	fields := bytes.Split(data, []byte{0})
	if len(fields) < 2 {
		c.logger.Debug("received incomplete message", "size", len(data))
		return
	}

	// First field is message ID
	msgID, err := strconv.Atoi(string(fields[0]))
	if err != nil {
		c.logger.Debug("invalid message ID", "data", string(fields[0]))
		return
	}

	c.logger.Debug("received message", "msg_id", msgID, "fields", len(fields))

	switch msgID {
	case msgTickPrice:
		c.handleTickPrice(fields)
	case msgTickSize:
		c.handleTickSize(fields)
	case msgAccountSummary:
		c.handleAccountSummary(fields)
	case msgPosition:
		c.handlePosition(fields)
	default:
		c.logger.Debug("unhandled message type", "msg_id", msgID)
	}
}

// handleTickPrice handles tick price messages.
func (c *Client) handleTickPrice(fields [][]byte) {
	// Format: msgID, version, tickerID, tickType, price, size, attribs
	if len(fields) < 5 {
		return
	}

	tickerID, _ := strconv.ParseInt(string(fields[2]), 10, 64)
	tickType, _ := strconv.Atoi(string(fields[3]))
	priceStr := string(fields[4])

	price, err := decimal.NewFromString(priceStr)
	if err != nil {
		return
	}

	// tickType: 1=bid, 2=ask, 4=last, 6=high, 7=low, 9=close
	event := types.MarketEvent{
		Timestamp: time.Now(),
	}

	switch tickType {
	case 4: // Last price
		event.Close = price
	case 6: // High
		event.High = price
	case 7: // Low
		event.Low = price
	default:
		return
	}

	c.publishMarketData(tickerID, event)
}

// handleTickSize handles tick size messages.
func (c *Client) handleTickSize(fields [][]byte) {
	// Format: msgID, version, tickerID, tickType, size
	if len(fields) < 5 {
		return
	}

	tickerID, _ := strconv.ParseInt(string(fields[2]), 10, 64)
	tickType, _ := strconv.Atoi(string(fields[3]))
	size, _ := strconv.ParseInt(string(fields[4]), 10, 64)

	// tickType: 0=bid size, 3=ask size, 5=last size, 8=volume
	if tickType == 8 { // Volume
		event := types.MarketEvent{
			Timestamp: time.Now(),
			Volume:    size,
		}
		c.publishMarketData(tickerID, event)
	}
}

// handleAccountSummary handles account summary messages.
func (c *Client) handleAccountSummary(fields [][]byte) {
	// Format: msgID, version, reqID, account, tag, value, currency
	if len(fields) < 7 {
		return
	}

	tag := string(fields[4])
	valueStr := string(fields[5])

	value, err := decimal.NewFromString(valueStr)
	if err != nil {
		return
	}

	c.accountMu.Lock()
	if c.account == nil {
		c.account = &broker.AccountSummary{
			AccountID:   string(fields[3]),
			Currency:    string(fields[6]),
			LastUpdated: time.Now(),
		}
	}

	switch tag {
	case "NetLiquidation":
		c.account.NetLiquidation = value
	case "TotalCashValue":
		c.account.TotalCashValue = value
	case "BuyingPower":
		c.account.BuyingPower = value
	case "AvailableFunds":
		c.account.AvailableFunds = value
	}
	c.account.LastUpdated = time.Now()
	c.accountMu.Unlock()

	c.logger.Debug("account summary updated", "tag", tag, "value", value)
}

// handlePosition handles position messages.
func (c *Client) handlePosition(fields [][]byte) {
	// Format: msgID, version, account, conId, symbol, secType, expiry, strike, right, multiplier, exchange, currency, localSymbol, tradingClass, position, avgCost
	if len(fields) < 15 {
		return
	}

	symbol := string(fields[4])
	contracts, _ := strconv.Atoi(string(fields[14]))
	avgCostStr := string(fields[15])
	avgCost, _ := decimal.NewFromString(avgCostStr)

	side := types.SideLong
	if contracts < 0 {
		side = types.SideShort
		contracts = -contracts
	}

	pos := &broker.Position{
		Symbol:      symbol,
		Contracts:   contracts,
		Side:        side,
		AvgCost:     avgCost,
		LastUpdated: time.Now(),
	}

	c.updatePosition(pos)
	c.logger.Debug("position updated", "symbol", symbol, "contracts", contracts, "side", side)
}

// handleDisconnect handles connection loss.
func (c *Client) handleDisconnect() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.State() == broker.StateDisconnected {
		return
	}

	c.state.Store(int32(broker.StateDisconnected))
	c.logger.Warn("disconnected from IBKR")

	if c.cfg.AutoReconnect {
		go c.reconnectLoop()
	}
}

// reconnectLoop attempts to reconnect.
func (c *Client) reconnectLoop() {
	for i := 0; i < c.cfg.MaxReconnectTries; i++ {
		select {
		case <-c.done:
			return
		case <-time.After(c.cfg.ReconnectInterval):
		}

		c.logger.Info("attempting reconnect", "attempt", i+1)

		ctx, cancel := context.WithTimeout(context.Background(), c.cfg.ConnectTimeout)
		err := c.Connect(ctx)
		cancel()

		if err == nil {
			c.logger.Info("reconnected successfully")
			return
		}

		c.logger.Warn("reconnect failed", "err", err)
	}

	c.logger.Error("max reconnect attempts reached")
}

// requestInitialData requests account and position data.
func (c *Client) requestInitialData(ctx context.Context) error {
	// Request account summary
	if err := c.requestAccountSummary(); err != nil {
		return fmt.Errorf("request account summary: %w", err)
	}

	// Request positions
	if err := c.requestPositions(); err != nil {
		return fmt.Errorf("request positions: %w", err)
	}

	return nil
}

// requestAccountSummary requests account summary data.
func (c *Client) requestAccountSummary() error {
	// REQ_ACCOUNT_SUMMARY = 62
	reqID := c.nextReqID.Add(1)
	msg := fmt.Sprintf("62\x001\x00%d\x00All\x00NetLiquidation,TotalCashValue,BuyingPower,AvailableFunds\x00", reqID)
	return c.sendMessage(msg)
}

// requestPositions requests current positions.
func (c *Client) requestPositions() error {
	// REQ_POSITIONS = 61
	msg := "61\x001\x00"
	return c.sendMessage(msg)
}

// sendMessage sends a message to TWS/Gateway.
func (c *Client) sendMessage(msg string) error {
	if c.State() != broker.StateConnected {
		return broker.ErrNotConnected
	}

	// Rate limit
	if err := c.limiter.Wait(context.Background()); err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}

	// Add message size prefix
	size := len(msg)
	data := make([]byte, 4+size)
	data[0] = byte(size >> 24)
	data[1] = byte(size >> 16)
	data[2] = byte(size >> 8)
	data[3] = byte(size)
	copy(data[4:], msg)

	_, err := c.conn.Write(data)
	return err
}

// Disconnect closes the connection.
func (c *Client) Disconnect() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.State() == broker.StateDisconnected {
		return nil
	}

	close(c.done)

	if c.conn != nil {
		_ = c.conn.Close()
	}

	c.wg.Wait()
	c.state.Store(int32(broker.StateDisconnected))

	c.logger.Info("disconnected from IBKR")
	return nil
}

// State returns the current connection state.
func (c *Client) State() broker.ConnectionState {
	return broker.ConnectionState(c.state.Load())
}

// IsConnected returns true if connected.
func (c *Client) IsConnected() bool {
	return c.State() == broker.StateConnected
}

// GetAccountSummary returns account summary.
func (c *Client) GetAccountSummary(ctx context.Context) (*broker.AccountSummary, error) {
	if !c.IsConnected() {
		return nil, broker.ErrNotConnected
	}

	c.accountMu.RLock()
	defer c.accountMu.RUnlock()

	if c.account == nil {
		return nil, fmt.Errorf("account data not available")
	}

	return c.account, nil
}

// SubscribeMarketData subscribes to market data for a symbol.
func (c *Client) SubscribeMarketData(ctx context.Context, symbol string) (<-chan types.MarketEvent, error) {
	if !c.IsConnected() {
		return nil, broker.ErrNotConnected
	}

	c.mdMu.Lock()
	defer c.mdMu.Unlock()

	// Check if already subscribed
	if sub, ok := c.mdSubscriptions[symbol]; ok {
		return sub.ch, nil
	}

	// Create subscription
	tickerID := c.nextReqID.Add(1)
	ch := make(chan types.MarketEvent, 100)

	sub := &marketDataSubscription{
		symbol:   symbol,
		tickerID: tickerID,
		ch:       ch,
	}

	c.mdSubscriptions[symbol] = sub

	// Send market data request
	if err := c.requestMarketData(tickerID, symbol); err != nil {
		delete(c.mdSubscriptions, symbol)
		close(ch)
		return nil, err
	}

	c.logger.Info("subscribed to market data",
		"symbol", symbol,
		"ticker_id", tickerID,
	)

	return ch, nil
}

// requestMarketData sends a market data request.
func (c *Client) requestMarketData(tickerID int64, symbol string) error {
	// Get contract
	expiry := broker.GetFrontMonthExpiry(time.Now())
	var contract broker.Contract

	switch symbol {
	case "MES":
		contract = broker.MESContract(expiry)
	case "MGC":
		contract = broker.MGCContract(expiry)
	default:
		return broker.ErrInvalidContract
	}

	// REQ_MKT_DATA = 1
	msg := fmt.Sprintf("1\x0011\x00%d\x000\x00%s\x00%s\x00\x00%s\x00%s\x00%s\x00%d\x00\x00\x00\x00\x00\x000\x00\x00\x00",
		tickerID,
		contract.Symbol,
		contract.SecType,
		contract.Expiry,
		contract.Exchange,
		contract.Currency,
		contract.Multiplier,
	)

	return c.sendMessage(msg)
}

// UnsubscribeMarketData unsubscribes from market data.
func (c *Client) UnsubscribeMarketData(symbol string) error {
	c.mdMu.Lock()
	defer c.mdMu.Unlock()

	sub, ok := c.mdSubscriptions[symbol]
	if !ok {
		return nil
	}

	// CANCEL_MKT_DATA = 2
	msg := fmt.Sprintf("2\x001\x00%d\x00", sub.tickerID)
	if err := c.sendMessage(msg); err != nil {
		return err
	}

	close(sub.ch)
	delete(c.mdSubscriptions, symbol)

	c.logger.Info("unsubscribed from market data", "symbol", symbol)
	return nil
}

// PlaceOrder places an order.
func (c *Client) PlaceOrder(ctx context.Context, intent types.OrderIntent) (*broker.OrderResult, error) {
	if !c.IsConnected() {
		return nil, broker.ErrNotConnected
	}

	orderID := c.nextReqID.Add(1)

	// Get contract
	expiry := broker.GetFrontMonthExpiry(time.Now())
	var contract broker.Contract

	switch intent.Symbol {
	case "MES":
		contract = broker.MESContract(expiry)
	case "MGC":
		contract = broker.MGCContract(expiry)
	default:
		return nil, broker.ErrInvalidContract
	}

	// Build order message
	action := "BUY"
	if intent.Side == types.SideShort {
		action = "SELL"
	}

	// PLACE_ORDER = 3
	msg := c.buildPlaceOrderMessage(orderID, contract, action, intent)
	if err := c.sendMessage(msg); err != nil {
		return nil, fmt.Errorf("send order: %w", err)
	}

	// Track order
	c.ordersMu.Lock()
	c.orders[intent.ClientOrderID] = &broker.Order{
		OrderID:       fmt.Sprintf("%d", orderID),
		ClientOrderID: intent.ClientOrderID,
		Symbol:        intent.Symbol,
		Side:          intent.Side,
		Quantity:      intent.Contracts,
		OrderType:     broker.OrderTypeMarket,
		Status:        broker.OrderStatusSubmitted,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	c.ordersMu.Unlock()

	c.logger.Info("order placed",
		"order_id", orderID,
		"client_order_id", intent.ClientOrderID,
		"symbol", intent.Symbol,
		"side", intent.Side,
		"contracts", intent.Contracts,
	)

	return &broker.OrderResult{
		OrderID:       fmt.Sprintf("%d", orderID),
		ClientOrderID: intent.ClientOrderID,
		Status:        broker.OrderStatusSubmitted,
		SubmittedAt:   time.Now(),
	}, nil
}

// buildPlaceOrderMessage builds a PLACE_ORDER message.
func (c *Client) buildPlaceOrderMessage(orderID int64, contract broker.Contract, action string, intent types.OrderIntent) string {
	// Simplified order message - real implementation needs all fields
	return fmt.Sprintf("3\x0045\x00%d\x000\x00%s\x00%s\x00\x00%s\x00%s\x00%s\x00%d\x00\x00\x00%s\x00%d\x00MKT\x00\x00\x00\x00\x00\x00\x00\x00\x00DAY\x00\x00\x00\x000\x000\x00%s\x000\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00",
		orderID,
		contract.Symbol,
		contract.SecType,
		contract.Expiry,
		contract.Exchange,
		contract.Currency,
		contract.Multiplier,
		action,
		intent.Contracts,
		intent.ClientOrderID,
	)
}

// CancelOrder cancels an order.
func (c *Client) CancelOrder(ctx context.Context, orderID string) error {
	if !c.IsConnected() {
		return broker.ErrNotConnected
	}

	// CANCEL_ORDER = 4
	msg := fmt.Sprintf("4\x001\x00%s\x00\x00", orderID)
	if err := c.sendMessage(msg); err != nil {
		return fmt.Errorf("send cancel: %w", err)
	}

	c.logger.Info("order cancel requested", "order_id", orderID)
	return nil
}

// GetOpenOrders returns open orders.
func (c *Client) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	if !c.IsConnected() {
		return nil, broker.ErrNotConnected
	}

	c.ordersMu.RLock()
	defer c.ordersMu.RUnlock()

	var orders []broker.Order
	for _, o := range c.orders {
		if o.Status == broker.OrderStatusSubmitted || o.Status == broker.OrderStatusPartial {
			orders = append(orders, *o)
		}
	}

	return orders, nil
}

// GetPositions returns all positions.
func (c *Client) GetPositions(ctx context.Context) ([]broker.Position, error) {
	if !c.IsConnected() {
		return nil, broker.ErrNotConnected
	}

	c.positionsMu.RLock()
	defer c.positionsMu.RUnlock()

	var positions []broker.Position
	for _, p := range c.positions {
		positions = append(positions, *p)
	}

	return positions, nil
}

// GetPosition returns position for a symbol.
func (c *Client) GetPosition(ctx context.Context, symbol string) (*broker.Position, error) {
	if !c.IsConnected() {
		return nil, broker.ErrNotConnected
	}

	c.positionsMu.RLock()
	defer c.positionsMu.RUnlock()

	pos, ok := c.positions[symbol]
	if !ok {
		return nil, nil
	}

	return pos, nil
}

// Shutdown gracefully shuts down the client.
func (c *Client) Shutdown(ctx context.Context) error {
	c.logger.Info("shutting down IBKR client")

	// Unsubscribe all market data
	c.mdMu.Lock()
	for symbol := range c.mdSubscriptions {
		c.mdMu.Unlock()
		_ = c.UnsubscribeMarketData(symbol)
		c.mdMu.Lock()
	}
	c.mdMu.Unlock()

	return c.Disconnect()
}

// updateAccountSummary updates account summary.
func (c *Client) updateAccountSummary(summary *broker.AccountSummary) {
	c.accountMu.Lock()
	defer c.accountMu.Unlock()
	c.account = summary
}

// updatePosition updates a position.
func (c *Client) updatePosition(pos *broker.Position) {
	c.positionsMu.Lock()
	defer c.positionsMu.Unlock()

	if pos.Contracts == 0 {
		delete(c.positions, pos.Symbol)
	} else {
		c.positions[pos.Symbol] = pos
	}
}

// publishMarketData publishes market data to subscribers.
func (c *Client) publishMarketData(tickerID int64, event types.MarketEvent) {
	c.mdMu.RLock()
	defer c.mdMu.RUnlock()

	for _, sub := range c.mdSubscriptions {
		if sub.tickerID == tickerID {
			select {
			case sub.ch <- event:
			default:
				c.logger.Warn("market data channel full", "symbol", sub.symbol)
			}
			return
		}
	}
}

// Ensure Client implements broker.Broker
var _ broker.Broker = (*Client)(nil)
