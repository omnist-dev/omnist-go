package omnist

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
}

// DefaultLimits returns the spec §2.4 reference defaults: depth 200, node
// count 1,000,000, integer digits 4,300.
func DefaultLimits() Limits {
	return Limits{
		MaxDepth:     200,
		MaxNodes:     1_000_000,
		MaxIntDigits: 4300,
	}
}

// LimitChecker tracks running depth and node count as a tree is walked,
// and validates integer literal digit counts, against a fixed Limits
// configuration. It is stateful and not safe for concurrent use.
//
// Nothing in this repository calls LimitChecker yet — issue #1 builds the
// Document model and its safety-limit machinery only; future OML/OSD/JSON
// etc. readers are expected to construct one LimitChecker per Document
// they build and call its methods as they walk the input.
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
