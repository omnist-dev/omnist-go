package omnist

import "testing"

func TestDefaultLimits(t *testing.T) {
	l := DefaultLimits()
	if l.MaxDepth != 200 || l.MaxNodes != 1_000_000 || l.MaxIntDigits != 4300 {
		t.Errorf("DefaultLimits() = %+v, want spec §2.4 reference defaults", l)
	}
}

func TestLimitCheckerDepthOK(t *testing.T) {
	c := NewLimitChecker(Limits{MaxDepth: 2, MaxNodes: 100, MaxIntDigits: 10})
	if diag := c.EnterNode("$"); diag != nil {
		t.Fatalf("unexpected diagnostic at depth 1: %+v", diag)
	}
	if diag := c.EnterNode("$.a"); diag != nil {
		t.Fatalf("unexpected diagnostic at depth 2: %+v", diag)
	}
	if c.Depth() != 2 {
		t.Errorf("Depth() = %d, want 2", c.Depth())
	}
	if c.NodeCount() != 2 {
		t.Errorf("NodeCount() = %d, want 2", c.NodeCount())
	}
	c.LeaveNode()
	if c.Depth() != 1 {
		t.Errorf("Depth() after LeaveNode = %d, want 1", c.Depth())
	}
}

func TestLimitCheckerDepthExceeded(t *testing.T) {
	c := NewLimitChecker(Limits{MaxDepth: 1, MaxNodes: 100, MaxIntDigits: 10})
	if diag := c.EnterNode("$"); diag != nil {
		t.Fatalf("unexpected diagnostic within depth limit: %+v", diag)
	}
	diag := c.EnterNode("$.a")
	if diag == nil {
		t.Fatal("expected a depth-limit diagnostic, got nil")
	}
	if diag.Code != CodeDocumentLimitDepth {
		t.Errorf("Code = %q, want %q", diag.Code, CodeDocumentLimitDepth)
	}
	if diag.Path != "$.a" {
		t.Errorf("Path = %q, want %q", diag.Path, "$.a")
	}
	if diag.Severity != SeverityError {
		t.Errorf("Severity = %v, want SeverityError", diag.Severity)
	}
}

func TestLimitCheckerNodesExceeded(t *testing.T) {
	c := NewLimitChecker(Limits{MaxDepth: 100, MaxNodes: 2, MaxIntDigits: 10})
	if diag := c.EnterNode("$"); diag != nil {
		t.Fatalf("unexpected diagnostic for node 1: %+v", diag)
	}
	if diag := c.EnterNode("$.a"); diag != nil {
		t.Fatalf("unexpected diagnostic for node 2: %+v", diag)
	}
	diag := c.EnterNode("$.b")
	if diag == nil {
		t.Fatal("expected a node-count-limit diagnostic, got nil")
	}
	if diag.Code != CodeDocumentLimitNodes {
		t.Errorf("Code = %q, want %q", diag.Code, CodeDocumentLimitNodes)
	}
}

func TestLimitCheckerLeaveNodeAtZeroIsNoop(t *testing.T) {
	c := NewLimitChecker(DefaultLimits())
	c.LeaveNode() // must not panic or go negative
	if c.Depth() != 0 {
		t.Errorf("Depth() = %d, want 0", c.Depth())
	}
}

func TestLimitCheckerIntDigits(t *testing.T) {
	c := NewLimitChecker(Limits{MaxDepth: 100, MaxNodes: 100, MaxIntDigits: 5})
	if diag := c.CheckIntDigits("$.n", 5); diag != nil {
		t.Fatalf("unexpected diagnostic at exactly the limit: %+v", diag)
	}
	diag := c.CheckIntDigits("$.n", 6)
	if diag == nil {
		t.Fatal("expected an int-digits-limit diagnostic, got nil")
	}
	if diag.Code != CodeDocumentLimitIntDigits {
		t.Errorf("Code = %q, want %q", diag.Code, CodeDocumentLimitIntDigits)
	}
	if diag.Path != "$.n" {
		t.Errorf("Path = %q, want %q", diag.Path, "$.n")
	}
}
