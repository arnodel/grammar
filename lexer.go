package grammar

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"regexp/syntax"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Position struct {
	Line, Column int
}

func (p *Position) Advance(s string) {
	for len(s) > 0 {
		i := strings.IndexAny(s, "\r\n")
		if i == -1 {
			p.Column += utf8.RuneCountInString(s)
			return
		}
		p.Column = 1
		p.Line++
		if s[0] == '\r' && len(s) > 1 && s[1] == '\n' {
			i++
		}
		s = s[i+1:]
	}
}

func (p Position) String() string {
	return fmt.Sprintf("Ln %d, Col %d", p.Line, p.Column)
}

// SimpleToken is a simple implementation of the both the Token and the Parser
// interfaces.  So it can be used in rules to match a token field.  It is also
// the concrete type of Tokens returned by SimpleTokenStream.
type SimpleToken struct {
	TokType  string
	TokValue string
	TokPos   Position
}

var _ Token = SimpleToken{}
var _ Parser = &SimpleToken{}
var _ PrettyWriter = SimpleToken{}

// EOF is the token that is returned when trying to consument an exhausted token
// stream.
var EOF = SimpleToken{
	TokType:  "EOF",
	TokValue: "EOF",
}

// Type returns the token type.
func (t SimpleToken) Type() string {
	return t.TokType
}

// Value returns the value of the token.
func (t SimpleToken) Value() string {
	return t.TokValue
}

func (t SimpleToken) Position() Position {
	return t.TokPos
}

func (t SimpleToken) PrettyWrite(out io.Writer, r any, pfx string, indent int) (err error) {
	return writeLine(out, pfx, indent, fmt.Sprintf("%s = %q %s", t.TokType, t.TokValue, t.TokPos))
}

// Parse tries to match the next token in the given TokenStream with the
// ParseOptions, using opts.MatchToken.  If they match, the receiver is loaded
// with the token data, if not a non-nil *ParseError is returned.  In any event
// the next token in the token stream has been consumed.
func (t *SimpleToken) Parse(_ interface{}, s *ParserState, opts TokenOptions) *ParseError {
	tok, err := opts.MatchNextToken(s)
	if err != nil {
		return err
	}
	t.TokType = tok.Type()
	t.TokValue = tok.Value()
	t.TokPos = tok.Position()
	// log.Printf("+++ parsed tok #%d: (%s, %q)", pos, t.TokType, t.TokValue)
	return nil
}

// SimpleTokenStream is a very simple implementation of the TokenStream
// interface which the Parse function requires.
type SimpleTokenStream struct {
	tokens     []Token
	currentPos int
	eof        Token // returned once the tokens are exhausted
}

// NewSimpleTokenStream makes a token stream from a slice of tokens.  The EOF
// token it returns at the end is positioned just after the last token.
func NewSimpleTokenStream(toks []Token) *SimpleTokenStream {
	pos := Position{Line: 1, Column: 1}
	if n := len(toks); n > 0 {
		pos = toks[n-1].Position()
		pos.Advance(toks[n-1].Value())
	}
	return &SimpleTokenStream{
		tokens: toks,
		eof:    eofAt(pos),
	}
}

func eofAt(pos Position) SimpleToken {
	return SimpleToken{TokType: EOF.TokType, TokValue: EOF.TokValue, TokPos: pos}
}

var _ TokenStream = (*SimpleTokenStream)(nil)

// Next consumes the next token in the token stream and returns it.  If the
// stream is exhausted, EOF is returned.
func (s *SimpleTokenStream) Next() Token {
	if s.currentPos >= len(s.tokens) {
		if s.eof != nil {
			return s.eof
		}
		return EOF
	}
	tok := s.tokens[s.currentPos]
	s.currentPos++
	// log.Printf("Next token: %d %q", s.currentPos-1, tok.Value())
	return tok
}

// Save returns the current position in the token stream.
func (s *SimpleTokenStream) Save() int {
	// log.Print("Saving pos: ", s.currentPos)
	return s.currentPos
}

// Restore rewinds the token stream to the given position (which should have
// been obtained by s.Save()).  In general this may panic - in this
// implementation it's always possible to rewind.
func (s *SimpleTokenStream) Restore(pos int) {
	// log.Print("Restoring pos: ", pos)
	s.currentPos = pos
}

func (s *SimpleTokenStream) Dump(w io.Writer) {
	for i, tok := range s.tokens {
		currentMarker := " "
		if i == s.currentPos {
			currentMarker = "*"
		}
		fmt.Fprintf(w, "%3d%s %s %q\n", i, currentMarker, tok.Type(), tok.Value())
	}
}

// A TokenDef defines a type of token and how to recognise it.  A list of them
// makes a tokeniser with SimpleTokeniser.
//
// A token is recognised by its pattern (Ptn), a regular expression matched at
// the start of the remaining input, or by a Scan function, or both: when both
// are given the pattern must match first and Scan then decides the length of
// the token.  Scan is given the remaining input and returns the length of the
// token at its start, or 0 if there is none, which lets it recognise what a
// regular expression cannot, like nested comments.
//
// Modes allow different sets of tokens in different contexts (e.g. inside a
// string template).  A definition belongs to the mode named by Mode; the
// initial mode is that of the first definition.  Matching a token with a
// PushMode enters that mode, matching one with PopMode returns to the
// previous one.  As an empty PushMode means no push, a mode that is pushed
// needs a name, so if the initial mode is, give it one.
type TokenDef struct {
	Ptn      string           // Regular expression matching the token
	Name     string           // Token type; empty means the token is skipped
	Scan     func(string) int // Length of the token at the start of the input (0: no match)
	Mode     string           // Mode the definition belongs to
	PushMode string           // Mode to enter after this token
	PopMode  bool             // Return to the previous mode after this token
}

// SimpleTokeniser takes a list of TokenDefs and returns a function that
// tokenises a string, returning a stream whose tokens are of type *SimpleToken.
// It panics if a definition is invalid (bad pattern, unknown PushMode, ...).
//
// At each point in the input the definition with the longest match wins, and
// among those matching the same length the first one listed, as with lex.  So
// keywords can be listed before an identifier pattern that also matches them
// without clashing with longer identifiers that start with a keyword.  A
// match of length 0 does not count, and if no definition matches, an error is
// returned.
func SimpleTokeniser(tokenDefs []TokenDef) func(string) (*SimpleTokenStream, error) {
	if len(tokenDefs) == 0 {
		panic("SimpleTokeniser: no token definitions")
	}
	modes := map[string]*tokeniserMode{}
	for i, def := range tokenDefs {
		m := modes[def.Mode]
		if m == nil {
			m = &tokeniserMode{}
			modes[def.Mode] = m
		}
		matcher, err := newTokenMatcher(def)
		if err != nil {
			panic(fmt.Sprintf("SimpleTokeniser: token definition %d (%q): %s", i, def.Name, err))
		}
		m.matchers = append(m.matchers, matcher)
	}
	for i, def := range tokenDefs {
		if def.PushMode != "" && modes[def.PushMode] == nil {
			panic(fmt.Sprintf("SimpleTokeniser: token definition %d (%q): PushMode %q is not a mode", i, def.Name, def.PushMode))
		}
	}
	initialMode := modes[tokenDefs[0].Mode]
	return func(s string) (*SimpleTokenStream, error) {
		mode := initialMode
		var prevModes []*tokeniserMode
		var tokens []Token
		var block []SimpleToken // tokens are allocated in blocks, not boxed one by one
		pos := Position{Line: 1, Column: 1}
		for len(s) > 0 {
			def, n := mode.match(s)
			if n == 0 {
				return nil, fmt.Errorf("invalid token starting at %v", pos)
			}
			tokValue := s[:n]
			if def.Name != "" {
				if len(block) == cap(block) {
					block = make([]SimpleToken, 0, min(max(64, len(tokens)), 4096))
				}
				block = append(block, SimpleToken{TokType: def.Name, TokValue: tokValue, TokPos: pos})
				tokens = append(tokens, &block[len(block)-1])
			}
			switch {
			case def.PushMode != "":
				prevModes = append(prevModes, mode)
				mode = modes[def.PushMode]
			case def.PopMode:
				last := len(prevModes) - 1
				if last < 0 {
					return nil, errors.New("no mode to pop")
				}
				mode = prevModes[last]
				prevModes = prevModes[:last]
			}
			s = s[n:]
			pos.Advance(tokValue)
		}
		return &SimpleTokenStream{tokens: tokens, eof: eofAt(pos)}, nil
	}
}

type tokeniserMode struct {
	matchers []tokenMatcher
}

// match returns the definition with the longest match at the start of s, and
// the length of that match (0 if none).
func (m *tokeniserMode) match(s string) (def *TokenDef, best int) {
	c := s[0]
	for i := range m.matchers {
		tm := &m.matchers[i]
		if !tm.first.has(c) {
			continue
		}
		if n := tm.match(s); n > best {
			best = n
			def = &tm.def
		}
	}
	return def, best
}

type matchKind int

const (
	matchScan      matchKind = iota // Scan only
	matchLiteral                    // the pattern is a literal string
	matchClass                      // the pattern is a single ASCII character class
	matchClassPlus                  // the pattern is an ASCII character class repeated (e.g. \s+)
	matchRegexp                     // anything else
)

type tokenMatcher struct {
	def   TokenDef
	kind  matchKind
	first byteSet // bytes a match can start with
	lit   string
	class byteSet // for matchClass and matchClassPlus
	re    *regexp.Regexp
}

func newTokenMatcher(def TokenDef) (tokenMatcher, error) {
	tm := tokenMatcher{def: def}
	if def.Ptn == "" {
		if def.Scan == nil {
			return tm, errors.New("needs a pattern or a Scan function")
		}
		tm.first.addAll()
		return tm, nil
	}
	tree, err := syntax.Parse(def.Ptn, syntax.Perl)
	if err != nil {
		return tm, err
	}
	tree = tree.Simplify()
	tm.first, _ = firstBytes(tree)
	switch {
	case tree.Op == syntax.OpLiteral && tree.Flags&syntax.FoldCase == 0:
		tm.kind = matchLiteral
		tm.lit = string(tree.Rune)
	case tree.Op == syntax.OpCharClass && isASCIIClass(tree):
		tm.kind = matchClass
		tm.class = tm.first
	case tree.Op == syntax.OpPlus && tree.Sub[0].Op == syntax.OpCharClass && isASCIIClass(tree.Sub[0]):
		tm.kind = matchClassPlus
		tm.class = tm.first
	default:
		tm.kind = matchRegexp
		tm.re, err = regexp.Compile(`^(?:` + def.Ptn + `)`)
		if err != nil {
			return tm, err
		}
	}
	return tm, nil
}

func (tm *tokenMatcher) match(s string) int {
	var n int
	switch tm.kind {
	case matchScan:
		return tm.def.Scan(s)
	case matchLiteral:
		if !strings.HasPrefix(s, tm.lit) {
			return 0
		}
		n = len(tm.lit)
	case matchClass:
		n = 1 // the first byte is in the class (see tokeniserMode.match)
	case matchClassPlus:
		n = 1
		for n < len(s) && tm.class.has(s[n]) {
			n++
		}
	case matchRegexp:
		loc := tm.re.FindStringIndex(s)
		if loc == nil {
			return 0
		}
		n = loc[1]
	}
	if n > 0 && tm.def.Scan != nil {
		n = tm.def.Scan(s)
	}
	return n
}

func isASCIIClass(re *syntax.Regexp) bool {
	for i := 1; i < len(re.Rune); i += 2 {
		if re.Rune[i] >= utf8.RuneSelf {
			return false
		}
	}
	return len(re.Rune) > 0
}

// byteSet is a set of bytes.
type byteSet [4]uint64

func (b *byteSet) add(c byte)      { b[c/64] |= 1 << (c % 64) }
func (b *byteSet) has(c byte) bool { return b[c/64]&(1<<(c%64)) != 0 }
func (b *byteSet) addAll()         { *b = byteSet{^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0)} }
func (b *byteSet) union(o byteSet) {
	for i := range b {
		b[i] |= o[i]
	}
}

// addRune adds the first byte of the UTF-8 encoding of r.
func (b *byteSet) addRune(r rune) {
	if r < utf8.RuneSelf {
		b.add(byte(r))
		return
	}
	var buf [utf8.UTFMax]byte
	utf8.EncodeRune(buf[:], r)
	b.add(buf[0])
}

// addRange adds the first bytes of the UTF-8 encodings of the runes in
// [lo, hi].
func (b *byteSet) addRange(lo, hi rune) {
	for r := lo; r <= hi && r < utf8.RuneSelf; r++ {
		b.add(byte(r))
	}
	if hi < utf8.RuneSelf {
		return
	}
	// Lead bytes of multi-byte sequences; over-approximating is fine.
	for c := 0xC0; c <= 0xF4; c++ {
		b.add(byte(c))
	}
}

// firstBytes returns the set of bytes a match of re can start with, and
// whether re can match the empty string.
func firstBytes(re *syntax.Regexp) (set byteSet, nullable bool) {
	switch re.Op {
	case syntax.OpLiteral:
		if len(re.Rune) == 0 {
			return set, true
		}
		r := re.Rune[0]
		set.addRune(r)
		if re.Flags&syntax.FoldCase != 0 {
			for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
				set.addRune(f)
			}
		}
		return set, false
	case syntax.OpCharClass:
		for i := 0; i+1 < len(re.Rune); i += 2 {
			set.addRange(re.Rune[i], re.Rune[i+1])
		}
		return set, false
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		set.addAll()
		return set, false
	case syntax.OpNoMatch:
		return set, false
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText,
		syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return set, true
	case syntax.OpCapture, syntax.OpPlus:
		return firstBytes(re.Sub[0])
	case syntax.OpStar, syntax.OpQuest:
		set, _ = firstBytes(re.Sub[0])
		return set, true
	case syntax.OpRepeat:
		set, nullable = firstBytes(re.Sub[0])
		return set, nullable || re.Min == 0
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			subSet, subNullable := firstBytes(sub)
			set.union(subSet)
			if !subNullable {
				return set, false
			}
		}
		return set, true
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			subSet, subNullable := firstBytes(sub)
			set.union(subSet)
			nullable = nullable || subNullable
		}
		return set, nullable
	default:
		set.addAll()
		return set, true
	}
}
