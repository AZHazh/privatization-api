package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	settlementEventsPath = "/api/v1/settlement/events"
	settlementLeasePath  = "/api/v1/settlement/lease"
)

var (
	errSettlementLeaseRequired = errors.New("settlement lease required")
	errSettlementQueueFull     = errors.New("settlement event queue full")
)

type SettlementService struct {
	cfg    config.SettlementConfig
	client *http.Client

	mu           sync.Mutex
	lease        settlementLease
	sequence     int64
	prevHash     string
	queue        []settlementUsageEvent
	lastFlush    time.Time
	lastLeaseTry time.Time
}

type settlementLease struct {
	LeaseID       string    `json:"lease_id,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	BalanceUSD    float64   `json:"balance_usd,omitempty"`
	SpentUSD      float64   `json:"spent_usd,omitempty"`
	Revoked       bool      `json:"revoked,omitempty"`
	LastUpdatedAt time.Time `json:"last_updated_at,omitempty"`
}

type settlementUsageEvent struct {
	SiteID                string     `json:"site_id"`
	Sequence              int64      `json:"sequence"`
	PreviousHash          string     `json:"previous_hash"`
	EventHash             string     `json:"event_hash,omitempty"`
	RequestID             string     `json:"request_id"`
	AccountID             int64      `json:"account_id"`
	AccountPlatform       string     `json:"account_platform,omitempty"`
	AccountType           string     `json:"account_type,omitempty"`
	Model                 string     `json:"model,omitempty"`
	RequestedModel        string     `json:"requested_model,omitempty"`
	UpstreamModel         *string    `json:"upstream_model,omitempty"`
	InputTokens           int        `json:"input_tokens"`
	OutputTokens          int        `json:"output_tokens"`
	CacheCreationTokens   int        `json:"cache_creation_tokens"`
	CacheReadTokens       int        `json:"cache_read_tokens"`
	CacheCreation5mTokens int        `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int        `json:"cache_creation_1h_tokens"`
	ImageOutputTokens     int        `json:"image_output_tokens"`
	ImageCount            int        `json:"image_count"`
	BillingMode           *string    `json:"billing_mode,omitempty"`
	BillingType           int8       `json:"billing_type"`
	TotalCost             float64    `json:"total_cost"`
	ActualCost            float64    `json:"actual_cost"`
	RateMultiplier        float64    `json:"rate_multiplier"`
	AccountRateMultiplier *float64   `json:"account_rate_multiplier,omitempty"`
	AccountStatsCost      *float64   `json:"account_stats_cost,omitempty"`
	RequestType           string     `json:"request_type"`
	Stream                bool       `json:"stream"`
	OpenAIWSMode          bool       `json:"openai_ws_mode"`
	CreatedAt             time.Time  `json:"created_at"`
	RecordedAt            time.Time  `json:"recorded_at"`
	Signature             string     `json:"signature,omitempty"`
}

type settlementEventBatch struct {
	SiteID string                 `json:"site_id"`
	Events []settlementUsageEvent `json:"events"`
}

type settlementBatchResponse struct {
	Lease *settlementLease `json:"lease,omitempty"`
}

var settlementServices struct {
	mu       sync.Mutex
	byConfig map[*config.Config]*SettlementService
}

func settlementServiceForConfig(cfg *config.Config) *SettlementService {
	if cfg == nil {
		return nil
	}
	settlementServices.mu.Lock()
	defer settlementServices.mu.Unlock()
	if settlementServices.byConfig == nil {
		settlementServices.byConfig = make(map[*config.Config]*SettlementService)
	}
	if svc := settlementServices.byConfig[cfg]; svc != nil {
		return svc
	}
	svc := NewSettlementService(cfg.Settlement)
	settlementServices.byConfig[cfg] = svc
	return svc
}

func NewSettlementService(cfg config.SettlementConfig) *SettlementService {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	svc := &SettlementService{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
	}
	if cfg.Enabled {
		go svc.flushLoop()
	}
	return svc
}

func (s *SettlementService) Enabled() bool {
	return s != nil &&
		s.cfg.Enabled &&
		strings.TrimSpace(s.cfg.CenterURL) != "" &&
		strings.TrimSpace(s.cfg.SiteID) != "" &&
		strings.TrimSpace(s.cfg.Secret) != ""
}

func (s *SettlementService) CheckLease(ctx context.Context) error {
	if !s.Enabled() || !s.cfg.LeaseRequired {
		return nil
	}
	if s.hasUsableLease(time.Now()) {
		return nil
	}
	if err := s.refreshLease(ctx); err != nil {
		if s.cfg.FailOpen {
			slog.Warn("settlement lease refresh failed; fail_open allows request", "error", err)
			return nil
		}
		return fmt.Errorf("%w: %v", errSettlementLeaseRequired, err)
	}
	if s.hasUsableLease(time.Now()) {
		return nil
	}
	if s.cfg.FailOpen {
		slog.Warn("settlement lease is unavailable after refresh; fail_open allows request")
		return nil
	}
	return errSettlementLeaseRequired
}

func (s *SettlementService) EnqueueUsage(ctx context.Context, usageLog *UsageLog, account *Account) {
	if !s.Enabled() || usageLog == nil {
		return
	}
	event, err := s.buildUsageEvent(usageLog, account)
	if err != nil {
		slog.Warn("settlement event build failed", "error", err, "request_id", usageLog.RequestID)
		return
	}

	shouldFlush := false
	s.mu.Lock()
	maxQueue := s.cfg.MaxQueueSize
	if maxQueue <= 0 {
		maxQueue = 10000
	}
	if len(s.queue) >= maxQueue {
		s.mu.Unlock()
		slog.Warn("settlement event dropped because queue is full", "error", errSettlementQueueFull, "request_id", usageLog.RequestID)
		return
	}
	s.queue = append(s.queue, event)
	if s.lease.LeaseID != "" {
		s.lease.SpentUSD += event.ActualCost
	}
	batchSize := s.cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 200
	}
	flushInterval := time.Duration(s.cfg.FlushIntervalSeconds) * time.Second
	if flushInterval <= 0 {
		flushInterval = 10 * time.Second
	}
	shouldFlush = len(s.queue) >= batchSize || time.Since(s.lastFlush) >= flushInterval
	s.mu.Unlock()

	if shouldFlush {
		s.Flush(ctx)
	}
}

func (s *SettlementService) Flush(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	batch := s.takeBatch()
	if len(batch) == 0 {
		return
	}
	if err := s.postEvents(ctx, batch); err != nil {
		s.requeueFront(batch)
		slog.Warn("settlement events upload failed", "error", err, "count", len(batch))
	}
}

func (s *SettlementService) flushLoop() {
	interval := time.Duration(s.cfg.FlushIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		s.Flush(context.Background())
	}
}

func (s *SettlementService) hasUsableLease(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lease.LeaseID == "" || s.lease.Revoked {
		return false
	}
	if !s.lease.ExpiresAt.IsZero() && !now.Before(s.lease.ExpiresAt) {
		return false
	}
	if s.lease.BalanceUSD > 0 {
		remaining := s.lease.BalanceUSD - s.lease.SpentUSD
		if remaining <= s.cfg.LeaseRenewThreshold {
			return false
		}
	}
	return true
}

func (s *SettlementService) refreshLease(ctx context.Context) error {
	s.mu.Lock()
	if time.Since(s.lastLeaseTry) < time.Second {
		s.mu.Unlock()
		return errors.New("lease refresh throttled")
	}
	s.lastLeaseTry = time.Now()
	s.mu.Unlock()

	url := s.centerURL(settlementLeasePath)
	body := map[string]string{"site_id": strings.TrimSpace(s.cfg.SiteID)}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	s.signRequest(req, payload)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("lease response status %d: %s", resp.StatusCode, strings.TrimSpace(string(limited)))
	}
	var decoded struct {
		Lease settlementLease `json:"lease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return err
	}
	s.applyLease(&decoded.Lease)
	return nil
}

func (s *SettlementService) buildUsageEvent(usageLog *UsageLog, account *Account) (settlementUsageEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sequence++
	now := time.Now().UTC()
	event := settlementUsageEvent{
		SiteID:                strings.TrimSpace(s.cfg.SiteID),
		Sequence:              s.sequence,
		PreviousHash:          s.prevHash,
		RequestID:             usageLog.RequestID,
		AccountID:             usageLog.AccountID,
		Model:                 usageLog.Model,
		RequestedModel:        usageLog.RequestedModel,
		UpstreamModel:         usageLog.UpstreamModel,
		InputTokens:           usageLog.InputTokens,
		OutputTokens:          usageLog.OutputTokens,
		CacheCreationTokens:   usageLog.CacheCreationTokens,
		CacheReadTokens:       usageLog.CacheReadTokens,
		CacheCreation5mTokens: usageLog.CacheCreation5mTokens,
		CacheCreation1hTokens: usageLog.CacheCreation1hTokens,
		ImageOutputTokens:     usageLog.ImageOutputTokens,
		ImageCount:            usageLog.ImageCount,
		BillingMode:           usageLog.BillingMode,
		BillingType:           usageLog.BillingType,
		TotalCost:             usageLog.TotalCost,
		ActualCost:            usageLog.ActualCost,
		RateMultiplier:        usageLog.RateMultiplier,
		AccountRateMultiplier: usageLog.AccountRateMultiplier,
		AccountStatsCost:      usageLog.AccountStatsCost,
		RequestType:           usageLog.EffectiveRequestType().String(),
		Stream:                usageLog.Stream,
		OpenAIWSMode:          usageLog.OpenAIWSMode,
		CreatedAt:             usageLog.CreatedAt.UTC(),
		RecordedAt:            now,
	}
	if account != nil {
		event.AccountPlatform = account.Platform
		event.AccountType = account.Type
	}
	hash, err := s.hashEvent(event)
	if err != nil {
		return settlementUsageEvent{}, err
	}
	event.EventHash = hash
	event.Signature = s.hmacHex(hash)
	s.prevHash = hash
	return event, nil
}

func (s *SettlementService) takeBatch() []settlementUsageEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return nil
	}
	batchSize := s.cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 200
	}
	n := batchSize
	if len(s.queue) < n {
		n = len(s.queue)
	}
	batch := append([]settlementUsageEvent(nil), s.queue[:n]...)
	copy(s.queue, s.queue[n:])
	s.queue = s.queue[:len(s.queue)-n]
	s.lastFlush = time.Now()
	return batch
}

func (s *SettlementService) requeueFront(batch []settlementUsageEvent) {
	if len(batch) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	maxQueue := s.cfg.MaxQueueSize
	if maxQueue <= 0 {
		maxQueue = 10000
	}
	available := maxQueue - len(s.queue)
	if available <= 0 {
		return
	}
	if len(batch) > available {
		batch = batch[len(batch)-available:]
	}
	next := make([]settlementUsageEvent, 0, len(batch)+len(s.queue))
	next = append(next, batch...)
	next = append(next, s.queue...)
	s.queue = next
}

func (s *SettlementService) postEvents(ctx context.Context, events []settlementUsageEvent) error {
	payload, err := json.Marshal(settlementEventBatch{
		SiteID: strings.TrimSpace(s.cfg.SiteID),
		Events: events,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.centerURL(settlementEventsPath), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	s.signRequest(req, payload)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("events response status %d: %s", resp.StatusCode, strings.TrimSpace(string(limited)))
	}
	var decoded settlementBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if decoded.Lease != nil {
		s.applyLease(decoded.Lease)
	}
	return nil
}

func (s *SettlementService) applyLease(lease *settlementLease) {
	if lease == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lease.LastUpdatedAt = time.Now().UTC()
	s.lease = *lease
}

func (s *SettlementService) centerURL(path string) string {
	return strings.TrimRight(strings.TrimSpace(s.cfg.CenterURL), "/") + path
}

func (s *SettlementService) signRequest(req *http.Request, payload []byte) {
	now := time.Now().UTC().Format(time.RFC3339)
	siteID := strings.TrimSpace(s.cfg.SiteID)
	signature := s.hmacHex(siteID + "\n" + now + "\n" + string(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Settlement-Site-ID", siteID)
	req.Header.Set("X-Settlement-Timestamp", now)
	req.Header.Set("X-Settlement-Signature", signature)
}

func (s *SettlementService) hashEvent(event settlementUsageEvent) (string, error) {
	event.EventHash = ""
	event.Signature = ""
	payload, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (s *SettlementService) hmacHex(data string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.Secret))
	_, _ = mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func checkSettlementLeaseForGateway(ctx context.Context, c interface{ JSON(int, interface{}) }, cfg *config.Config) error {
	svc := settlementServiceForConfig(cfg)
	if svc == nil {
		return nil
	}
	if err := svc.CheckLease(ctx); err != nil {
		if c != nil {
			c.JSON(http.StatusPaymentRequired, map[string]any{
				"error": map[string]any{
					"type":    "settlement_lease_required",
					"message": "Settlement lease is unavailable",
				},
			})
		}
		return err
	}
	return nil
}

func enqueueSettlementUsage(ctx context.Context, cfg *config.Config, usageLog *UsageLog, account *Account) {
	if svc := settlementServiceForConfig(cfg); svc != nil {
		svc.EnqueueUsage(ctx, usageLog, account)
	}
}
