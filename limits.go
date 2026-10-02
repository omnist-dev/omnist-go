package omnist

import "fmt"

// Limits bounds the work a Document builder will do before refusing to
// continue, per spec §2.4. The existence and meaning of the three limits
// is normative; the specific numbers are not, so Limits is a configurable
// struct rather than package-level constants. Every conformant
// implementation MUST enforce a finite limit on all three — "no limit" is
// not a legal value for any field.
type Limits struct {
	// MaxDepth is the maximum levels of node nesting, counted from the
	// Document root.
	MaxDepth int
	// MaxNodes is the maximum nodes materialized while building one
	// Document.
	MaxNodes int
	// MaxIntDigits is the maximum decimal digits in an integer literal,
	// sign excluded.
	MaxIntDigits int
	// MaxAliasExpansion is the maximum expansion factor E(a) = W(a) / S(a)
	// of any candidate node in a format with an anchor/reference mechanism
	// (spec D-18, §2.4.1): every anchored definition and every mapping and
	// sequence, anchored or not, including the document root and an inline
	// merge source. YAML is the only such codec today;
	// every other reader ignores the field. It is a finite bound like the
	// others (D-10), never "unbounded": zero means "unset" and selects
	// DefaultMaxAliasExpansion, so a Limits literal written before this
	// field existed keeps a finite limit; a negative value is invalid
	// (Validate reports it) and readers treat it as "unset" too, never as
	// "no limit". EffectiveMaxAliasExpansion gives the value a reader
	// enforces.
	MaxAliasExpansion int
	// MaxExpandedSlots is the maximum number of value slots a YAML input may
	// materialize, W(root), for an input that contains at least one alias or
	// merge key (spec D-22, §2.4.1). An input with neither is not subject to
	// it. Like MaxAliasExpansion it is a finite bound (D-10): zero means
	// "unset" and selects DefaultMaxExpandedSlots, and a negative value is
	// invalid (Validate reports it) and treated as "unset" by readers, never
	// as "no limit". EffectiveMaxExpandedSlots gives the value a reader
	// enforces.
	MaxExpandedSlots int
}

// DefaultMaxExpandedSlots is the spec §2.4 reference default for the expanded
// size (D-22): 1,000,000 value slots.
const DefaultMaxExpandedSlots = 1_000_000

// EffectiveMaxExpandedSlots returns the expanded-size cap a YAML reader
// enforces for l: MaxExpandedSlots when it is positive, otherwise
// DefaultMaxExpandedSlots. A non-positive configuration never widens the cap
// and never disables it (D-10).
func (l Limits) EffectiveMaxExpandedSlots() int {
	if l.MaxExpandedSlots > 0 {
		return l.MaxExpandedSlots
	}
	return DefaultMaxExpandedSlots
}

// DefaultMaxAliasExpansion is the spec §2.4 reference default for the alias
// expansion factor (D-18): 50.
const DefaultMaxAliasExpansion = 50

// EffectiveMaxAliasExpansion returns the alias expansion factor a YAML
// reader enforces for l: MaxAliasExpansion when it is positive, otherwise
// DefaultMaxAliasExpansion. A non-positive configuration never widens the
// limit and never disables it (D-10).
func (l Limits) EffectiveMaxAliasExpansion() int {
	if l.MaxAliasExpansion > 0 {
		return l.MaxAliasExpansion
	}
	return DefaultMaxAliasExpansion
}

// DefaultLimits returns the spec §2.4 reference defaults: depth 200, node
// count 1,000,000, integer digits 4,300, alias expansion factor 50, expanded
// size 1,000,000 slots.
func DefaultLimits() Limits {
	return Limits{
		MaxDepth:          200,
		MaxNodes:          1_000_000,
		MaxIntDigits:      4300,
		MaxAliasExpansion: DefaultMaxAliasExpansion,
		MaxExpandedSlots:  DefaultMaxExpandedSlots,
	}
}

// LimitChecker tracks running depth and node count as a tree is walked,
// and validates integer literal digit counts, against a fixed Limits
// configuration. It is stateful and not safe for concurrent use.
//
// Every format reader in this repository (OML, OSD, JSON, YAML, TOML, XML)
// constructs one LimitChecker per Document it builds and invokes
// EnterNode, LeaveNode, and CheckIntDigits as it walks input.
type LimitChecker struct {
	limits       Limits
	currentDepth int
	nodeCount    int
}

// NewLimitChecker returns a LimitChecker enforcing limits.
func NewLimitChecker(limits Limits) *LimitChecker {
	return &LimitChecker{limits: limits}
}

// EnterNode records descending into one more level of nesting and
// materializing one more node. Call it when a reader starts building a new
// node (including the root). Call LeaveNode when done with that node's
// children. Returns a Diagnostic with code CodeDocumentLimitDepth or
// CodeDocumentLimitNodes if the corresponding limit is exceeded, otherwise
// nil.
func (c *LimitChecker) EnterNode(path string) *Diagnostic {
	c.currentDepth++
	if c.currentDepth > c.limits.MaxDepth {
		return &Diagnostic{
			Path:     path,
			Code:     CodeDocumentLimitDepth,
			Message:  "nesting exceeds the configured depth limit",
			Severity: SeverityError,
		}
	}
	c.nodeCount++
	if c.nodeCount > c.limits.MaxNodes {
		return &Diagnostic{
			Path:     path,
			Code:     CodeDocumentLimitNodes,
			Message:  "node count exceeds the configured node limit",
			Severity: SeverityError,
		}
	}
	return nil
}

// LeaveNode records ascending back out of one level of nesting entered via
// EnterNode. Callers MUST call it exactly once for each successful
// EnterNode call, after that node's children have all been processed.
func (c *LimitChecker) LeaveNode() {
	if c.currentDepth > 0 {
		c.currentDepth--
	}
}

// Depth returns the current nesting depth.
func (c *LimitChecker) Depth() int { return c.currentDepth }

// NodeCount returns the number of nodes entered so far.
func (c *LimitChecker) NodeCount() int { return c.nodeCount }

// CheckIntDigits validates that digitCount (the number of decimal digits
// in an integer literal, sign excluded) does not exceed the configured
// limit. Returns a Diagnostic with code CodeDocumentLimitIntDigits if it
// does, otherwise nil.
func (c *LimitChecker) CheckIntDigits(path string, digitCount int) *Diagnostic {
	if digitCount > c.limits.MaxIntDigits {
		return &Diagnostic{
			Path:     path,
			Code:     CodeDocumentLimitIntDigits,
			Message:  "integer literal exceeds the configured digit limit",
			Severity: SeverityError,
		}
	}
	return nil
}

// Recommended limit ceilings for Limits.Validate:
// While spec §2.4 mandates that every implementation must enforce finite positive
// limits for MaxDepth, MaxNodes, and MaxIntDigits, setting astronomically large limits
// (e.g. math.MaxInt) practically defeats the safety purpose of resource bounding.
const (
	// MaxRecommendedDepth is the upper bound above which recursive algorithms risk stack exhaustion.
	MaxRecommendedDepth = 10_000
	// MaxRecommendedNodes is the upper bound above which node materialization risks heap exhaustion.
	MaxRecommendedNodes = 100_000_000
	// MaxRecommendedIntDigits is the upper bound above which arbitrary-precision integer parsing risks CPU exhaustion.
	MaxRecommendedIntDigits = 1_000_000
	// MaxRecommendedAliasExpansion is the upper bound above which the alias expansion check no
	// longer meaningfully bounds amplification (the spec measures its weakest dangerous document
	// at about E = 170).
	MaxRecommendedAliasExpansion = 10_000
	// MaxRecommendedExpandedSlots is the spec's recommended ceiling for the expanded size (D-22):
	// at about 780 bytes per slot measured in Go, 10,000,000 slots is roughly 8 GB.
	MaxRecommendedExpandedSlots = 10_000_000
)

// Validate checks that l specifies strictly positive values within sane, recommended safety
// bounds (issue #78). It returns an error if any field is <= 0 or exceeds recommended ceilings.
//
// Validate is purely opt-in: NewLimitChecker does not enforce it, so callers who require
// custom or unusually large limits retain full control. For standard production use,
// DefaultLimits() is recommended.
func (l Limits) Validate() error {
	if l.MaxDepth <= 0 {
		return fmt.Errorf("MaxDepth must be positive, got %d", l.MaxDepth)
	}
	if l.MaxDepth > MaxRecommendedDepth {
		return fmt.Errorf("MaxDepth %d exceeds recommended safety ceiling (%d)", l.MaxDepth, MaxRecommendedDepth)
	}
	if l.MaxNodes <= 0 {
		return fmt.Errorf("MaxNodes must be positive, got %d", l.MaxNodes)
	}
	if l.MaxNodes > MaxRecommendedNodes {
		return fmt.Errorf("MaxNodes %d exceeds recommended safety ceiling (%d)", l.MaxNodes, MaxRecommendedNodes)
	}
	if l.MaxIntDigits <= 0 {
		return fmt.Errorf("MaxIntDigits must be positive, got %d", l.MaxIntDigits)
	}
	if l.MaxIntDigits > MaxRecommendedIntDigits {
		return fmt.Errorf("MaxIntDigits %d exceeds recommended safety ceiling (%d)", l.MaxIntDigits, MaxRecommendedIntDigits)
	}
	if l.MaxAliasExpansion < 0 {
		return fmt.Errorf("MaxAliasExpansion must not be negative, got %d (zero selects the default %d)", l.MaxAliasExpansion, DefaultMaxAliasExpansion)
	}
	if l.MaxAliasExpansion > MaxRecommendedAliasExpansion {
		return fmt.Errorf("MaxAliasExpansion %d exceeds recommended safety ceiling (%d)", l.MaxAliasExpansion, MaxRecommendedAliasExpansion)
	}
	if l.MaxExpandedSlots < 0 {
		return fmt.Errorf("MaxExpandedSlots must not be negative, got %d (zero selects the default %d)", l.MaxExpandedSlots, DefaultMaxExpandedSlots)
	}
	if l.MaxExpandedSlots > MaxRecommendedExpandedSlots {
		return fmt.Errorf("MaxExpandedSlots %d exceeds recommended safety ceiling (%d)", l.MaxExpandedSlots, MaxRecommendedExpandedSlots)
	}
	return nil
}
