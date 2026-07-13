// Package ai provides the pre-trade risk analysers. A deterministic heuristic
// analyser is the safety floor; an optional LLM-backed analyser can only RAISE
// the risk score (never lower it) and degrades to the heuristic on any error, so
// the model is advisory and can never wave a dangerous order through.
package ai

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/domain"
	"github.com/ABHIJEET-MUNESHWAR/tradedesk/internal/ports"
)

// Thresholds configures the heuristic risk rules.
type Thresholds struct {
	// MaxNotional rejects orders whose limit notional exceeds this amount.
	MaxNotional domain.Money
	// MaxQuantity rejects orders larger than this share count.
	MaxQuantity domain.Quantity
	// RejectScore is the score at or above which an order is rejected.
	RejectScore int
}

// DefaultThresholds returns conservative production defaults.
func DefaultThresholds() Thresholds {
	return Thresholds{
		MaxNotional: 1_000_000 * 10000, // $1,000,000
		MaxQuantity: 100_000 * 1000,    // 100,000 shares
		RejectScore: 80,
	}
}

// HeuristicAnalyzer implements ports.RiskGate with deterministic rules.
type HeuristicAnalyzer struct {
	th Thresholds
}

// NewHeuristicAnalyzer builds a heuristic analyser.
func NewHeuristicAnalyzer(th Thresholds) *HeuristicAnalyzer {
	if th.RejectScore <= 0 {
		th.RejectScore = 80
	}
	return &HeuristicAnalyzer{th: th}
}

// assess computes a deterministic risk score and reason for an order.
func (h *HeuristicAnalyzer) assess(o *domain.Order) (int, string) {
	score := 0
	reasons := make([]string, 0, 3)

	if o.OrderType() == domain.OrderTypeMarket {
		score += 15 // market orders carry execution-price uncertainty
	}
	// Breaching a hard limit is an automatic rejection (score 100), not merely
	// additive risk, so an oversized order can never slip through on balance.
	if h.th.MaxQuantity > 0 && o.Quantity() > h.th.MaxQuantity {
		score += 100
		reasons = append(reasons, "quantity exceeds limit")
	}
	if o.OrderType() == domain.OrderTypeLimit && h.th.MaxNotional > 0 {
		if notional, err := o.LimitPrice().MulQuantity(o.Quantity()); err == nil {
			if notional > h.th.MaxNotional {
				score += 100
				reasons = append(reasons, "notional exceeds limit")
			}
		}
	}
	if o.TIF() == domain.TIFFOK || o.TIF() == domain.TIFIOC {
		score += 5 // immediate-or-cancel style adds fill-risk
	}
	if score > 100 {
		score = 100
	}
	return score, strings.Join(reasons, "; ")
}

// Check implements ports.RiskGate.
func (h *HeuristicAnalyzer) Check(_ context.Context, o *domain.Order) (ports.RiskDecision, error) {
	score, reason := h.assess(o)
	approved := score < h.th.RejectScore
	if approved {
		reason = ""
	} else if reason == "" {
		reason = "risk score above threshold"
	}
	return ports.RiskDecision{Approved: approved, Reason: reason, Score: score}, nil
}

// ChatCompleter is the minimal seam over an LLM chat endpoint, kept tiny so it
// is trivial to fake in tests and swap for any provider (OpenAI, Bedrock, ...).
type ChatCompleter interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// LLMAnalyzer augments the heuristic with an agentic model score. The model may
// only push the score UP; it can never approve an order the heuristic rejected.
type LLMAnalyzer struct {
	heuristic *HeuristicAnalyzer
	model     ChatCompleter
	th        Thresholds
}

// NewLLMAnalyzer wraps a heuristic analyser with an LLM completer.
func NewLLMAnalyzer(h *HeuristicAnalyzer, model ChatCompleter, th Thresholds) *LLMAnalyzer {
	if th.RejectScore <= 0 {
		th.RejectScore = h.th.RejectScore
	}
	return &LLMAnalyzer{heuristic: h, model: model, th: th}
}

// Check implements ports.RiskGate with a heuristic floor + model ceiling.
func (l *LLMAnalyzer) Check(ctx context.Context, o *domain.Order) (ports.RiskDecision, error) {
	base, reason := l.heuristic.assess(o)
	score := base
	if l.model != nil {
		if raised, ok := l.modelScore(ctx, o); ok && raised > score {
			score = raised
			if reason == "" {
				reason = "model-elevated risk"
			}
		}
	}
	approved := score < l.th.RejectScore
	if approved {
		reason = ""
	} else if reason == "" {
		reason = "risk score above threshold"
	}
	return ports.RiskDecision{Approved: approved, Reason: reason, Score: score}, nil
}

// modelScore asks the model for a 0-100 score. Any error or unparsable answer
// returns ok=false so the caller falls back to the heuristic floor (fail-safe).
func (l *LLMAnalyzer) modelScore(ctx context.Context, o *domain.Order) (int, bool) {
	prompt := fmt.Sprintf(
		"You are a pre-trade risk reviewer. Reply with ONLY an integer 0-100 risk score for this order: "+
			"symbol=%s side=%s type=%s tif=%s qty=%s limit=%s.",
		o.Symbol(), o.Side(), o.OrderType(), o.TIF(), o.Quantity(), o.LimitPrice())
	out, err := l.model.Complete(ctx, prompt)
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || n < 0 || n > 100 {
		return 0, false
	}
	return n, true
}

// Compile-time checks that both analysers satisfy the port.
var (
	_ ports.RiskGate = (*HeuristicAnalyzer)(nil)
	_ ports.RiskGate = (*LLMAnalyzer)(nil)
)
