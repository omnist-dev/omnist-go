package xml

import (
	"strings"
	"testing"

	omnist "github.com/omnist-dev/omnist-go"
)

// Spec §2.5 (D-15, D-21): one leading U+FEFF is stripped, a second is rejected
// at 1:1 with parse.codec-syntax, before encoding/xml sees the text (it would
// otherwise treat the first as stray text outside the document element).
const bomMark = "\uFEFF"

func TestReadLeadingBOMIsStrippedAndDoubledIsRejected(t *testing.T) {
	const body = "<root><a>1</a></root>"
	plain, _, err := Read(body, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	withBOM, _, err := Read(bomMark+body, omnist.DefaultLimits())
	if err != nil {
		t.Fatalf("a single leading mark must be stripped: %v", err)
	}
	if !docEqual(plain, withBOM) {
		t.Errorf("a leading mark changed the Document")
	}
	if _, _, err := Read(bomMark+"<?xml version=\"1.0\"?>"+body, omnist.DefaultLimits()); err != nil {
		t.Errorf("a leading mark before the XML declaration: %v", err)
	}
	for _, in := range []string{bomMark + bomMark + body, bomMark + bomMark + bomMark + body} {
		_, _, err := Read(in, omnist.DefaultLimits())
		pe, ok := err.(*omnist.ParseError)
		if !ok || pe.Code != omnist.CodeParseCodecSyntax || pe.Path != "1:1" {
			t.Errorf("got %#v, want parse.codec-syntax at 1:1", err)
		}
	}
	// A lone mark leaves empty input, which is not a document.
	if _, _, err := Read(bomMark, omnist.DefaultLimits()); err == nil {
		t.Error("a lone mark is empty input and must be rejected")
	}
}

func wantRefusal(t *testing.T, src string, code omnist.Code) {
	t.Helper()
	_, _, err := Read(src, omnist.DefaultLimits())
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != code || pe.Path != "$" {
		t.Errorf("%q: got %#v, want %s at $", src, err, code)
	}
}

func wantSyntaxError(t *testing.T, src string) {
	t.Helper()
	_, _, err := Read(src, omnist.DefaultLimits())
	pe, ok := err.(*omnist.ParseError)
	if !ok || pe.Code != omnist.CodeParseCodecSyntax {
		t.Errorf("%q: got %#v, want parse.codec-syntax", src, err)
	}
}

// The data-XML profile (docs/formats/xml.md): three well-formed constructs are
// refused, each at path "$".
func TestProfileRefusals(t *testing.T) {
	// A DOCTYPE is refused on sight, even one that declares nothing usable and
	// references nothing.
	wantRefusal(t, `<?xml version="1.0"?><!DOCTYPE d [<!ELEMENT d ANY>]><d><f>hi</f></d>`, omnist.CodeFormatDTDForbidden)
	wantRefusal(t, `<!DOCTYPE d><d/>`, omnist.CodeFormatDTDForbidden)
	wantRefusal(t, `<!DOCTYPE d [<!ENTITY x "never used">]><d>plain</d>`, omnist.CodeFormatDTDForbidden)

	// An entity reference other than the five predefined ones.
	wantRefusal(t, `<d><f>&nbsp;</f></d>`, omnist.CodeFormatEntityForbidden)
	wantRefusal(t, `<d f="&copy;">x</d>`, omnist.CodeFormatEntityForbidden)
	wantRefusal(t, `<d>&é;</d>`, omnist.CodeFormatEntityForbidden)
	wantRefusal(t, `<d><a>&x1;</a><b>&x2;</b></d>`, omnist.CodeFormatEntityForbidden)

	// Mixed content: text alongside child elements, before, between or after.
	wantRefusal(t, `<d>text<f>hi</f></d>`, omnist.CodeFormatMixedContent)
	wantRefusal(t, `<d><f>hi</f>text</d>`, omnist.CodeFormatMixedContent)
	wantRefusal(t, `<d><f>hi</f>text<g/></d>`, omnist.CodeFormatMixedContent)
	wantRefusal(t, `<r><d>hello<f>x</f>world</d></r>`, omnist.CodeFormatMixedContent)

	// The first refusal in document order is the one reported.
	wantRefusal(t, `<!DOCTYPE d><d>text<f/>&nbsp;</d>`, omnist.CodeFormatDTDForbidden)
	wantRefusal(t, `<d>&nbsp;<f/>text</d>`, omnist.CodeFormatEntityForbidden)
}

// Everything that stays legal: the five predefined entities, numeric character
// references, CDATA, comments and processing instructions (all inert), and
// whitespace between child elements.
func TestProfileLegalConstructs(t *testing.T) {
	cases := map[string]string{
		"predefined entities":        `<d>&lt;&gt;&amp;&quot;&apos;</d>`,
		"numeric references":         `<d>&#65;&#x42;</d>`,
		"entity syntax in a comment": `<d><!-- &nbsp; --><f>x</f></d>`,
		"entity syntax in CDATA":     `<d><![CDATA[&nbsp; <not-a-tag>]]></d>`,
		"entity syntax in a PI":      `<d><?pi &nbsp;?><f>x</f></d>`,
		"comment in the prolog":      `<!-- c --><d><f>x</f></d>`,
		"whitespace between kids":    "<d>\n  <f>x</f>\n  <g>y</g>\n</d>",
		"whitespace-only CDATA":      `<d><![CDATA[  ]]><f>x</f></d>`,
	}
	for name, src := range cases {
		if _, _, err := Read(src, omnist.DefaultLimits()); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	d, _, err := Read(`<d>&lt;&#65;&amp;</d>`, omnist.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := omnist.NodeDocument(omnist.NewNode().AddValue("d", omnist.ScalarValue(omnist.NewStringScalar("<A&"))))
	if !docEqual(d, want) {
		t.Errorf("got %+v, want %+v", d, want)
	}
}

// The refusal runs AFTER well-formedness: input that is not well-formed is a
// parse.codec-syntax failure even when it also contains a profile violation,
// because the profile only speaks about well-formed XML.
func TestMalformedXMLIsASyntaxErrorNotAProfileRefusal(t *testing.T) {
	wantSyntaxError(t, `<!DOCTYPE d><d><f>`)
	wantSyntaxError(t, `<!DOCTYPE d><d></x>`)
	wantSyntaxError(t, `<d>&nbsp;<f></d>`)
	wantSyntaxError(t, `<d>&nbsp;`)
	wantSyntaxError(t, `<d>text<f>hi</g></d>`)
	wantSyntaxError(t, `<d>&nbsp</d>`) // not an entity reference at all: no ';'
	wantSyntaxError(t, `<d>&1x;</d>`)  // not a name
	wantSyntaxError(t, `<d/><e/>`)
	wantSyntaxError(t, `<!DOCTYPE d><d/><!DOCTYPE e>`)
}

// A <!...> construct that is not a DOCTYPE in the prolog is simply not XML;
// a DOCTYPE anywhere but the prolog is not well-formed either.
func TestDirectivesOutsideTheProlog(t *testing.T) {
	wantSyntaxError(t, `<!ELEMENT d ANY><d/>`)
	wantSyntaxError(t, `<!doctype d><d/>`) // DOCTYPE is case-sensitive in XML
	wantSyntaxError(t, `<d><!DOCTYPE d></d>`)
	wantSyntaxError(t, `<d/><!DOCTYPE d>`)
}

// The sentinel that stands in for an entity's expansion must not collide with
// characters the document really contains.
func TestEntitySentinelAvoidsCharactersInTheSource(t *testing.T) {
	src := "<d></d>"
	if r := entitySentinel(src); r != 0xE002 {
		t.Errorf("sentinel = %U, want U+E002", r)
	}
	if _, _, err := Read(src, omnist.DefaultLimits()); err != nil {
		t.Errorf("private-use characters in the source are ordinary text: %v", err)
	}
	wantRefusal(t, "<d>&nbsp;</d>", omnist.CodeFormatEntityForbidden)
	if entitySentinel("plain") != 0xE000 {
		t.Error("the first free private-use rune should be chosen")
	}
}

func TestEntityMapSkipsPredefinedEntities(t *testing.T) {
	m := entityMap("&lt;&gt;&amp;&quot;&apos;&foo;&bar;&foo;", 0xE000)
	if len(m) != 2 || m["foo"] == "" || m["bar"] == "" {
		t.Errorf("entityMap = %v, want exactly foo and bar", m)
	}
}

// XML 1.0 cannot represent a raw C0 control character other than tab, LF and
// CR; the write fails unconditionally at the leaf's path.
func TestWriteRefusesC0ControlCharacters(t *testing.T) {
	str := func(s string) omnist.Value { return omnist.ScalarValue(omnist.NewStringScalar(s)) }
	for _, c := range []string{"\x00", "\x01", "\x08", "\x0b", "\x0c", "\x0e", "\x1f"} {
		d := omnist.NodeDocument(omnist.NewNode().AddNode("root", omnist.NewNode().AddValue("s", str("a"+c+"b"))))
		_, _, err := Write(d)
		diag, ok := err.(omnist.Diagnostic)
		if !ok || diag.Code != omnist.CodeWriteUnsupportedValue || diag.Path != "$.root.s" {
			t.Errorf("%q: got %#v, want write.unsupported-value at $.root.s", c, err)
		}
		if ok && !strings.Contains(diag.Message, "U+") {
			t.Errorf("message should name the character: %q", diag.Message)
		}
	}
	for _, c := range []string{"\t", "\n", "\r", " "} {
		d := omnist.NodeDocument(omnist.NewNode().AddNode("root", omnist.NewNode().AddValue("s", str("a"+c+"b"))))
		if _, _, err := Write(d); err != nil {
			t.Errorf("%q is legal XML text but the write failed: %v", c, err)
		}
	}
}
