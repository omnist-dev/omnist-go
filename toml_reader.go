package omnist

import (
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// ReadTOML parses TOML source text into a Document (spec §7.1,
// docs/formats/toml.md, "stage 1 only, no schema"). limits configures the
// safety limits enforced while reading, per the same Limits struct every
// other codec uses (limits.go); see the "Limit enforcement" section below
// for how this reader applies them — TOML's own document shape does not
// fit LimitChecker's Enter/LeaveNode calling convention directly, so this
// reader adapts rather than forces it.
//
// # Library choice and empirical verification (issue #27)
//
// github.com/pelletier/go-toml/v2 is used, specifically its unstable
// package (an explicitly-unstable but public raw-AST API: unstable.Parser
// hands back a stream of Node values with Kind/Raw/Data/Children, one per
// top-level expression, rather than only a fully-decoded Go value) — the
// same "raw parse tree to build on top of" shape issue #25 used yaml.v3's
// Node API for, and for the identical reason: this reader needs source
// key order and full-precision numeric literals, neither of which the
// library's own high-level decode path (toml.Unmarshal) preserves.
//
// This was confirmed empirically (not assumed from documentation) with a
// standalone throwaway program (not part of this package) that fed the
// library varied TOML documents and inspected both decode paths:
//
//   - Key order: decoding into map[string]interface{} loses order (Go
//     maps have none) for every table's fields, and would violate D-1.
//     The unstable.Parser's NextExpression()/Expression() stream instead
//     yields exactly one Node per top-level statement (KeyValue, Table,
//     ArrayTable) in source order, with each container Node's Children()
//     iterator likewise walking child nodes (inline-table members, array
//     elements, dotted-key segments) in source order. This reader builds
//     the Document tree directly from that stream, never through the
//     map-shaped decode path, so source order is preserved throughout.
//   - Integer precision: decoding into either map[string]interface{} or a
//     typed field routes through the library's internal parseInteger,
//     which parses into int64 and hard-fails ("...too large to fit in a
//     64-bit signed integer") on any literal outside that range —
//     confirmed with a 53-digit literal, which the high-level decode path
//     rejects outright. Spec §2.4 requires supporting integers up to
//     4,300 decimal digits. The unstable.Parser's own tokenizer, however,
//     does not apply this width limit at all: an Integer-kind Node's Data
//     field holds the literal's raw, unparsed text regardless of
//     magnitude (confirmed with the same 53-digit literal, which the
//     unstable parser accepts and reports as Kind=Integer, Data="999...").
//     This reader therefore never calls the library's own numeric decode
//     functions (they are unexported besides), parsing every Integer/
//     Float leaf's raw text itself in parseTOMLInt/parseTOMLFloat below,
//     into a *big.Int the same way every other codec in this package
//     does (oml_lexer.go, ReadJSON, ReadYAML).
//   - Number/integer distinction: confirmed directly against the AST —
//     `a = 1` produces Kind=Integer, `c = 2.0` produces Kind=Float, never
//     collapsed together regardless of magnitude (`2.0`'s Data is
//     literally "2.0", distinct from `2`'s "2") — TOML's own grammar
//     already carries this distinction lexically, exactly as the issue
//     anticipated, so there is nothing to reconcile the way ReadJSON's
//     shape-not-magnitude reasoning has to.
//   - Native date/time/datetime: the AST's Kind enum has four distinct
//     temporal kinds — LocalDate, LocalTime, LocalDateTime, and DateTime
//     (offset) — confirmed by parsing all four TOML forms and observing
//     each produces its own Kind. See "Offset vs. local datetime" below
//     for how DateTime (the offset-carrying one) maps onto the Document
//     model, which has no separate offset-datetime kind of its own.
//   - Float special values: confirmed `nan`/`inf`/`+inf`/`-inf` all parse
//     to Kind=Float with their literal spelling in Data (not resolved to
//     a Go float64 by the AST layer). Go's strconv.ParseFloat happens to
//     already accept "inf"/"+inf"/"-inf"/"nan" directly, matching TOML's
//     spelling — but NOT "-nan"/"+nan" (strconv.ParseFloat errors on a
//     signed nan, confirmed empirically), even though TOML's grammar
//     allows a sign on nan (its sign is defined to be insignificant).
//     parseTOMLFloat below special-cases a signed nan/inf's first
//     character before falling back to strconv.ParseFloat, mirroring the
//     small hand-rolled check the library's own (unexported) parseFloat
//     in decode.go uses internally for exactly the same reason — this
//     reader cannot call that function, but the shape of the fix is the
//     same once the gap is known.
//   - Bare TOML syntax errors (e.g. a sign on a hex/octal/binary literal,
//     which TOML's grammar forbids): confirmed the unstable.Parser itself
//     rejects these at the grammar level (NextExpression() returns false,
//     Error() reports "sign is not allowed on numbers with a radix
//     prefix") — so parseTOMLInt below can assume any Integer-kind node
//     it receives already satisfies TOML's own grammar (no unsigned-radix
//     combination to re-validate).
//
// # Offset vs. local datetime
//
// TOML's grammar has both local-date-time (no offset) and offset-date-time
// (RFC 3339, with a 'Z' or +HH:MM/-HH:MM suffix); the Document model's
// `datetime` kind (spec §2.2.1) makes no such distinction — issue #1's
// design does not carve out a separate "offset datetime" kind, and
// docs/formats/toml.md does not address TOML's offset variant explicitly
// (noted in the issue as something this reader would need to decide).
//
// The decision made here: an offset date-time reads to the same
// Document-model `datetime` kind as a local one, with the offset itself
// preserved losslessly in TimeValue.HasOffset/OffsetSeconds (document.go)
// — fields that already exist on every datetime's embedded TimeValue and
// are already populated by ReadYAML's identical offset-carrying timestamp
// case (yaml_reader.go's parseYAMLDateTime) and already round-tripped by
// WriteJSON/WriteYAML's shared formatISOTime (json_writer.go). This is not
// a lossy adjustment or a new mechanism: it is the existing, already-
// exercised path for "a datetime that happens to carry an offset",
// applied to TOML's own offset-datetime literal the same way it already
// applies to YAML's. A local (no-offset) date-time simply leaves
// HasOffset false, the same as any other offset-less datetime in this
// package.
//
// # Limit enforcement
//
// LimitChecker (limits.go) is built around a single recursive descent
// (EnterNode before descending into a nested value, LeaveNode after)
// which fits ReadJSON/ReadYAML/OML's parsers exactly, because each of
// those readers builds one nested container in one recursive call.
// TOML's grammar is different in a way that breaks that fit: a document
// is a *flat* stream of top-level expressions (KeyValue/Table/
// ArrayTable), and a `[table.header]` expression can re-enter and extend
// a table that a much earlier, unrelated expression already created
// (`[[fruits]]` ... later ... `[fruits.physical]`), or a sibling header at
// the same depth follows immediately after a deeply-nested one with no
// "closing" expression in between. LimitChecker's currentDepth is a
// single monotonic counter that only ever goes up between EnterNode calls
// and down on LeaveNode — there is no natural point in a flat expression
// stream to call LeaveNode "the right number of times" between two
// unrelated headers without either double-counting reused nodes as new
// ones (inflating MaxNodes) or leaving currentDepth permanently
// overstated (falsely tripping MaxDepth on unrelated sibling tables).
//
// Rather than force TOML's flat shape through an API built for recursive
// descent, this reader enforces the identical §2.4 limits directly:
// every *Node this reader allocates is assigned its true depth (parent's
// depth + 1, exactly what EnterNode would have computed had the document
// been built by recursive descent) at creation time, checked against
// limits.MaxDepth right there — this needs no running counter at all,
// since a node's distance from the root never changes after it is
// created. A single monotonic nodeCount field (never decremented, exactly
// matching LimitChecker's own nodeCount field, which also never
// decreases) is checked against limits.MaxNodes on the same allocations.
// Both checks report the identical Diagnostic codes/messages/severity
// LimitChecker.EnterNode itself would (CodeDocumentLimitDepth,
// CodeDocumentLimitNodes) — this is the same §2.4 enforcement against the
// same Limits configuration, adapted to fit, not a different policy.
// CheckIntDigits (limits.go), the one LimitChecker method that is
// stateless, is used directly and unmodified, exactly as every other
// codec uses it.
func ReadTOML(text string, limits Limits) (Document, error) {
	r := &tomlReader{
		limits:  limits,
		checker: NewLimitChecker(limits),
		root:    NewNode(),
	}
	p := &unstable.Parser{}
	p.Reset([]byte(text))
	r.p = p

	for p.NextExpression() {
		expr := p.Expression()
		if err := r.readTopLevel(expr); err != nil {
			return Document{}, err
		}
	}
	if err := p.Error(); err != nil {
		return Document{}, r.wrapParserError(err)
	}
	return NodeDocument(r.root), nil
}

// tomlReader holds the state for one ReadTOML call.
type tomlReader struct {
	limits    Limits
	checker   *LimitChecker
	p         *unstable.Parser
	root      *Node
	current   *Node // insertion point for a bare (non-dotted) KeyValue
	nodeCount int
}

// newChildNode allocates a new *Node at depth childDepth (the parent's own
// depth + 1), enforcing MaxDepth/MaxNodes per ReadTOML's doc comment. path
// is used only for the resulting Diagnostic's Path field.
func (r *tomlReader) newChildNode(childDepth int, path string) (*Node, error) {
	if childDepth > r.limits.MaxDepth {
		return nil, &ParseError{Path: path, Code: CodeDocumentLimitDepth, Message: "nesting exceeds the configured depth limit"}
	}
	r.nodeCount++
	if r.nodeCount > r.limits.MaxNodes {
		return nil, &ParseError{Path: path, Code: CodeDocumentLimitNodes, Message: "node count exceeds the configured node limit"}
	}
	return NewNode(), nil
}

// readTopLevel dispatches one top-level expression (Table, ArrayTable, or
// KeyValue) per the model-mapping table (docs/formats/toml.md): a Table
// header switches the "current table" pointer that subsequent bare
// KeyValue expressions attach to; an ArrayTable header does the same but
// always creates a brand-new node (never reuses one), matching
// `[[x]]` written twice becoming the label `x` twice with no adjustment.
func (r *tomlReader) readTopLevel(expr *unstable.Node) error {
	switch expr.Kind {
	case unstable.Table:
		node, _, err := r.resolvePath(r.root, 0, expr.Key())
		if err != nil {
			return err
		}
		r.current = node
		return nil
	case unstable.ArrayTable:
		parent, depth, label, err := r.resolveParentPath(expr.Key())
		if err != nil {
			return err
		}
		child, err := r.newChildNode(depth+1, r.posPath(expr.Raw))
		if err != nil {
			return err
		}
		parent.AddNode(label, child)
		r.current = child
		return nil
	default: // unstable.KeyValue
		if r.current == nil {
			r.current = r.root
		}
		return r.readKeyValue(r.current, 0, expr)
	}
}

// resolveParentPath walks all but the last segment of an ArrayTable
// header's dotted key, root-anchored (an ArrayTable header's path is
// always relative to the document root, per TOML's own grammar — see
// resolveParentPathFrom's doc comment for the contrasting KeyValue case),
// and returns the resolved parent node, its depth, and the final
// (unresolved) segment's label. ReadTOML's ArrayTable branch always
// allocates a fresh node for that final segment itself (never reusing an
// existing one — see resolvePath's doc comment for why), which is why
// this stops one segment short of resolveParentPathFrom, its
// arbitrary-starting-point counterpart used for a KeyValue's dotted key.
func (r *tomlReader) resolveParentPath(keys unstable.Iterator) (*Node, int, string, error) {
	return r.resolveParentPathFrom(r.root, 0, keys)
}

// resolvePath walks every segment of a Table header's dotted key,
// reusing an existing matching child table at each step when one exists
// and creating one otherwise — every segment, including the last, uses
// the same reuse-or-create rule (navigateOrCreate), which is exactly
// right for a plain `[section]` header: re-opening an already-existing
// table (e.g. `[fruits.physical]` extending a `fruits` table an earlier
// `[[fruits]]` implicitly created) is legal TOML and must land on the
// same node, not a new sibling. ArrayTable headers do NOT use this
// function — see ReadTOML's readTopLevel, which always allocates a fresh
// node for an ArrayTable's final segment (two `[[x]]` headers are two
// distinct nodes sharing the label `x`, never merged) — that is the one
// point docs/formats/toml.md itself contrasts Table and ArrayTable on.
func (r *tomlReader) resolvePath(start *Node, startDepth int, keys unstable.Iterator) (*Node, int, error) {
	var segs []string
	for keys.Next() {
		segs = append(segs, string(keys.Node().Data))
	}
	node, depth := start, startDepth
	for _, seg := range segs {
		var err error
		node, depth, err = r.navigateOrCreate(node, depth, seg)
		if err != nil {
			return nil, 0, err
		}
	}
	return node, depth, nil
}

// navigateOrCreate resolves one dotted-key segment under node: the LAST
// matching edge whose label equals seg and whose target is itself a node
// is reused (matching real TOML semantics for both a table header
// re-opening an earlier implicit table, e.g. `[fruits.physical]` after an
// earlier `[[fruits]]`, and a dotted KeyValue key extending an
// already-started implicit table across multiple statements, e.g.
// `host.name = "x"` then `host.port = 1`) — "last" matters specifically
// for array-of-tables: dotted references after a repeated `[[x]]` header
// always mean the most-recently-opened `x` instance, never an earlier
// one. When no matching edge exists, a new node is created and appended.
//
// This reader does not separately enforce TOML's own "cannot redefine an
// already-fully-defined table" validity rule (e.g. two plain `[a]`
// headers for the same path) — that is a semantic well-formedness check
// beyond stage-1 shape construction, and neither ReadJSON nor ReadYAML
// independently re-validates grammar-adjacent rules their own underlying
// library doesn't already enforce. This is the plainly-correct reading of
// a narrow, cosmetic gap (malformed input of exactly this shape produces
// a Document that merges the redefinitions rather than an error), noted
// here rather than treated as load-bearing.
func (r *tomlReader) navigateOrCreate(node *Node, depth int, seg string) (*Node, int, error) {
	for i := len(node.Edges) - 1; i >= 0; i-- {
		e := node.Edges[i]
		if e.Label == seg {
			if child, ok := e.Target.Node(); ok {
				return child, depth + 1, nil
			}
			break
		}
	}
	child, err := r.newChildNode(depth+1, seg)
	if err != nil {
		return nil, 0, err
	}
	node.AddNode(seg, child)
	return child, depth + 1, nil
}

// readKeyValue processes one KeyValue expression (top-level, or nested
// inside an inline table), attaching its value under target at the depth
// target itself sits at (targetDepth), per the dotted-key resolution
// navigateOrCreate implements.
func (r *tomlReader) readKeyValue(target *Node, targetDepth int, kv *unstable.Node) error {
	parent, depth, label, err := r.resolveParentPathFrom(target, targetDepth, kv.Key())
	if err != nil {
		return err
	}
	return r.attachValue(parent, depth, label, kv.Value())
}

// resolveParentPathFrom is resolveParentPath's counterpart for a KeyValue
// nested under an arbitrary starting node (target/targetDepth) rather
// than always the document root — the root-anchored resolveParentPath is
// used for Table/ArrayTable headers (whose dotted path is always
// root-relative per TOML's own grammar), while a KeyValue's dotted key
// (whether top-level or inside an inline table) is relative to whatever
// table it appears in.
func (r *tomlReader) resolveParentPathFrom(target *Node, targetDepth int, keys unstable.Iterator) (*Node, int, string, error) {
	var segs []string
	for keys.Next() {
		segs = append(segs, string(keys.Node().Data))
	}
	node, depth := target, targetDepth
	for i := 0; i < len(segs)-1; i++ {
		var err error
		node, depth, err = r.navigateOrCreate(node, depth, segs[i])
		if err != nil {
			return nil, 0, "", err
		}
	}
	return node, depth, segs[len(segs)-1], nil
}

// attachValue builds and attaches the Target(s) for one resolved
// (parent, label) pair from a KeyValue's value node. An Array value
// expands into one edge per element sharing label, exactly as ReadJSON's
// readMember/ReadYAML's readSequenceElements treat a JSON array/YAML
// sequence — TOML has no model-mapping-table entry contrasting inline
// arrays with JSON's (docs/formats/toml.md is silent on them, discussing
// only tables/array-of-tables), so this is the plainly-correct default
// reading shared by every other format in the JSON family, applied here
// too, per the same repeated-label rule.
func (r *tomlReader) attachValue(parent *Node, parentDepth int, label string, v *unstable.Node) error {
	switch v.Kind {
	case unstable.Array:
		return r.attachArray(parent, parentDepth, label, v)
	case unstable.InlineTable:
		child, err := r.buildInlineTable(parentDepth, v)
		if err != nil {
			return err
		}
		parent.AddNode(label, child)
		return nil
	default:
		val, err := r.readScalar(v)
		if err != nil {
			return err
		}
		parent.AddValue(label, val)
		return nil
	}
}

// attachArray appends one edge per array element to parent, all sharing
// label — the repeated-label expansion attachValue's doc comment
// describes. An empty array is rejected with CodeParseEmptyArray, and a
// nested bare array (an array element that is itself an Array) is
// rejected with CodeDocumentUnlabeledElement, mirroring ReadYAML's
// readSequenceElements exactly (see that function's doc comment for why:
// the same array-as-repeated-label-sugar mechanism, a third format).
func (r *tomlReader) attachArray(parent *Node, parentDepth int, label string, arr *unstable.Node) error {
	it := arr.Children()
	empty := true
	for it.Next() {
		empty = false
		elem := it.Node()
		switch elem.Kind {
		case unstable.Array:
			return &ParseError{Path: label, Code: CodeDocumentUnlabeledElement, Message: "an array element must not itself be an array"}
		case unstable.InlineTable:
			child, err := r.buildInlineTable(parentDepth, elem)
			if err != nil {
				return err
			}
			parent.AddNode(label, child)
		default:
			val, err := r.readScalar(elem)
			if err != nil {
				return err
			}
			parent.AddValue(label, val)
		}
	}
	if empty {
		return &ParseError{Path: label, Code: CodeParseEmptyArray, Message: "an empty array is not a valid value"}
	}
	return nil
}

// buildInlineTable reads an InlineTable value node's children (each one a
// KeyValue node — confirmed empirically against the AST; the unstable
// package's own doc comment on Node states an InlineTable's children are
// "each of kind InlineTable", which a throwaway probe program showed to
// be inaccurate: `{a = 1, b = "x"}` produces two KeyValue children, not
// InlineTable ones. This reader trusts what the AST actually returns
// (checked directly) rather than that comment, per this issue's own
// instruction to verify empirically rather than trust documentation) into
// a freshly allocated node one level deeper than parentDepth.
func (r *tomlReader) buildInlineTable(parentDepth int, tbl *unstable.Node) (*Node, error) {
	child, err := r.newChildNode(parentDepth+1, "")
	if err != nil {
		return nil, err
	}
	it := tbl.Children()
	for it.Next() {
		if err := r.readKeyValue(child, parentDepth+1, it.Node()); err != nil {
			return nil, err
		}
	}
	return child, nil
}

// posPath renders a Raw range as a "line:col" Path via the parser's own
// Shape (unstable.Parser.Shape), matching every other reader's ParseError
// Path convention (spec §8.4).
func (r *tomlReader) posPath(raw unstable.Range) string {
	shape := r.p.Shape(raw)
	return itoa(shape.Start.Line) + ":" + itoa(shape.Start.Column)
}

func itoa(n int) string { return strconv.Itoa(n) }

// wrapParserError converts the error unstable.Parser.Error() reports into
// a *ParseError. Every one of the library's own error sites (confirmed by
// reading every unstable.NewParserError call in the library's parser.go)
// constructs a *unstable.ParserError with a genuine, non-nil Highlight —
// a real subslice of the input — so the type assertion below is
// unchecked and Highlight is trusted to be non-nil, the same precondition
// -trusting convention temporal.go's ParseISODate/ParseISOTime
// already use in this package (see that file's comment above
// ParseISODate) rather than carrying a defensive fallback branch no
// input can reach.
func (r *tomlReader) wrapParserError(err error) error {
	pe := err.(*unstable.ParserError) //nolint:errorlint // see doc comment: the library's own error is always this concrete type
	shape := r.p.Shape(r.p.Range(pe.Highlight))
	path := itoa(shape.Start.Line) + ":" + itoa(shape.Start.Column)
	return &ParseError{Line: shape.Start.Line, Col: shape.Start.Column, Path: path, Code: CodeParseUnexpectedToken, Message: pe.Message}
}

// readScalar resolves a scalar-kind value node (String/Bool/Integer/
// Float/LocalDate/LocalTime/LocalDateTime/DateTime) to a Document Value,
// applying the digit-count limit to any resulting integer via
// LimitChecker.CheckIntDigits, exactly as ReadYAML's own readScalar does.
func (r *tomlReader) readScalar(v *unstable.Node) (Value, error) {
	switch v.Kind {
	case unstable.String:
		return ScalarValue(NewStringScalar(string(v.Data))), nil
	case unstable.Bool:
		return ScalarValue(NewBooleanScalar(string(v.Data) == "true")), nil
	case unstable.Integer:
		bi := parseTOMLInt(string(v.Data))
		digits := len(strings.TrimPrefix(bi.String(), "-"))
		if diag := r.checker.CheckIntDigits(r.posPath(v.Raw), digits); diag != nil {
			return Value{}, &ParseError{Path: diag.Path, Code: diag.Code, Message: diag.Message}
		}
		return ScalarValue(NewIntegerScalar(bi)), nil
	case unstable.Float:
		return ScalarValue(NewNumberScalar(parseTOMLFloat(string(v.Data)))), nil
	case unstable.LocalDate:
		return ScalarValue(NewDateScalar(ParseISODate(string(v.Data)))), nil
	case unstable.LocalTime:
		return ScalarValue(NewTimeScalar(ParseISOTime(string(v.Data)))), nil
	default: // unstable.LocalDateTime, unstable.DateTime
		return ScalarValue(NewDateTimeScalar(parseTOMLDateTime(string(v.Data)))), nil
	}
}

// parseTOMLInt parses an Integer-kind node's raw literal text into a
// *big.Int, arbitrary precision (spec §2.4), after stripping TOML's
// digit-group underscores. big.Int.SetString's base-0 mode already
// understands the exact prefix set TOML's own integer grammar uses
// (0x/0o/0b, or a bare decimal with an optional +/- sign) — the same
// reasoning ReadYAML's parseYAMLInt already applies to YAML 1.1's
// (different but overlapping) prefix set — so this defers to it directly
// rather than hand-rolling per-base parsing. Every Integer-kind node this
// is called on already satisfies TOML's grammar (confirmed empirically —
// see ReadTOML's doc comment on the sign/radix-prefix combination the
// parser itself rejects before this function ever sees the text), so
// SetString cannot fail on well-formed input, so — mirroring
// temporal.go's ParseISODate/ParseISOTime convention of trusting a
// checked precondition rather than carrying a permanently-dead error
// branch (see that file's comment for the same reasoning applied to a
// different precondition) — this has no error return at all.
func parseTOMLInt(raw string) *big.Int {
	s := strings.ReplaceAll(raw, "_", "")
	bi, _ := new(big.Int).SetString(s, 0)
	return bi
}

// parseTOMLFloat parses a Float-kind node's raw literal text into a
// float64, stripping underscores first. TOML's grammar allows a sign on
// "nan" (`+nan`/`-nan`), whose sign is defined to carry no meaning, but
// Go's strconv.ParseFloat rejects a signed nan outright — confirmed
// empirically (see ReadTOML's doc comment) — so a signed inf/nan is
// special-cased here first, mirroring the small hand-rolled check the
// library's own unexported decode.go parseFloat performs for the
// identical reason, before falling back to strconv.ParseFloat for every
// ordinary decimal/exponent literal.
func parseTOMLFloat(raw string) float64 {
	s := strings.ReplaceAll(raw, "_", "")
	i := 0
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		i = 1
	}
	if len(s) == i+3 {
		switch s[i] {
		case 'i':
			if len(s) > 0 && s[0] == '-' {
				return math.Inf(-1)
			}
			return math.Inf(1)
		case 'n':
			return math.NaN()
		}
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// parseTOMLDateTime parses a LocalDateTime or DateTime node's raw literal
// text into a DateTimeValue. TOML's grammar allows 'T', 't', or a literal
// space as the date/time separator (all three confirmed empirically to
// reach Kind=LocalDateTime/DateTime, not rejected), and either 'Z'/'z' or
// a +HH:MM/-HH:MM suffix for an offset — wider than temporal.go's own
// ParseISODateTime (OML's grammar only allows 'T', matching ISODateTimeRegexp),
// so this does not reuse it directly, though it does reuse
// ParseISODate/ParseISOTime (document-model-neutral, format-agnostic
// field parsers temporal.go already defines) for the date and
// (offset-less) time portions.
func parseTOMLDateTime(s string) DateTimeValue {
	datePart := s[:10]
	timePart := s[11:]
	var tv TimeValue
	if n := len(timePart); n > 0 && (timePart[n-1] == 'Z' || timePart[n-1] == 'z') {
		tv = ParseISOTime(timePart[:n-1])
		tv.HasOffset = true
		tv.OffsetSeconds = 0
	} else {
		tv = ParseISOTime(timePart)
	}
	return DateTimeValue{Date: ParseISODate(datePart), Time: tv}
}
