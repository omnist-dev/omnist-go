package omnist

import "strings"

// byteOrderMark is U+FEFF, the Unicode byte-order mark. It is spelled as an
// escape, never as a raw character: a raw U+FEFF in Go source is invisible
// to a reader and to grep, which is exactly how an open-coded second strip
// hides. TestNoRawByteOrderMarkInTrackedFiles keeps it that way.
const byteOrderMark = "\uFEFF"

// StripLeadingBOM is the one place every read surface (OML, OSD, JSON, YAML,
// TOML, XML) applies spec §2.5's byte-order-mark rules to its input text.
// Nothing else in this repository strips or rejects a U+FEFF; a reader calls
// this first and hands the returned text to its own lexer or library.
//
// D-15: a U+FEFF at offset zero is consumed and contributes nothing. It is
// consumed exactly once.
//
// D-21: if a second U+FEFF still stands at offset zero of what remains, the
// read fails with a *ParseError at text position 1:1 -- computed on the text
// after the strip, not on the original input -- carrying code, which is
// CodeParseUnexpectedToken for OML and OSD and CodeParseCodecSyntax for the
// four codecs (§8.3.1). The check runs here, on the raw text, before any
// library sees it, because YAML and XML libraries would otherwise discard the
// second mark themselves; that is a second undeclared strip.
//
// A U+FEFF anywhere other than offset zero is ordinary content and is never
// touched, so a reader that calls this cannot corrupt a label or a value.
func StripLeadingBOM(text string, code Code) (string, *ParseError) {
	rest, had := strings.CutPrefix(text, byteOrderMark)
	if !had {
		return text, nil
	}
	if strings.HasPrefix(rest, byteOrderMark) {
		return "", &ParseError{
			Line:    1,
			Col:     1,
			Path:    "1:1",
			Code:    code,
			Message: "a second leading U+FEFF (byte-order mark) follows the one already stripped; exactly one is permitted (spec §2.5, D-21)",
		}
	}
	return rest, nil
}
