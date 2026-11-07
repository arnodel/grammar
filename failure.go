package grammar

import (
	"errors"
	"fmt"
	"strings"
)

// A ParseError describes where a parse failed: the position and token, the
// tokens that were expected there (TokenParseOptions, from "tok" tags) and
// any other reason (Err).  Parse returns the furthest failure of the parse,
// which is where the input went wrong.
//
// During a parse, the *ParseError returned by a failing rule is shared by all
// failures and describes the furthest one so far, not necessarily the rule's
// own failure (see ParserState.Fail).
type ParseError struct {
	Err error
	Token
	TokenParseOptions []TokenParseOptions
	Pos               int
}

// errRecursion marks the failure of a left-recursive reference to a rule
// (see memo.go); it never reaches users when there is anything better to
// say.
var errRecursion = errors.New("recursion")

// Fail records that parsing failed at stream position pos, on token tok, for
// the given reason, and returns the error for the Parse method to return.  A
// custom rule uses it to report a failure other than a token mismatch (which
// TokenOptions.MatchNextToken records itself):
//
//	pos := s.Save()
//	tok, err := opts.MatchNextToken(s)
//	if err != nil {
//		return err
//	}
//	if !valid(tok) {
//		return s.Fail(pos, tok, errors.New("invalid literal"))
//	}
//
// Only the furthest failure of a parse is recorded (with every reason given
// for it), as it is where the input actually went wrong; earlier ones are
// dropped.  The *ParseError returned is the same for all failures of a parse
// and describes the furthest one, so a rule should not expect it to describe
// its own failure; it only means "failed".
func (s *ParserState) Fail(pos int, tok Token, err error) *ParseError {
	return s.recordFailure(pos, tok, nil, err)
}

// recordFailure merges a failure into s.fail, the furthest failure record,
// and returns a pointer to the record, which is what every failing rule
// returns.  This way no error is allocated during parsing; Parse returns a
// copy of the record at the end.
func (s *ParserState) recordFailure(pos int, tok Token, opts []TokenParseOptions, err error) *ParseError {
	f := &s.fail
	switch {
	case f.Token == nil || pos > f.Pos:
		f.Pos = pos
		f.Token = tok
		f.TokenParseOptions = append(f.TokenParseOptions[:0], opts...)
		f.Err = err
	case pos == f.Pos:
		// Duplicates are removed when the result is read, not here: dozens
		// of options may be recorded at a position, and checking each new
		// one against them was the biggest cost of a failed match.
		f.TokenParseOptions = append(f.TokenParseOptions, opts...)
		if len(f.TokenParseOptions) > maxRecordedOptions {
			f.TokenParseOptions = uniqueOptions(f.TokenParseOptions, f.TokenParseOptions[:0])
		}
		f.Err = mergeErrs(f.Err, err)
	default:
		return f // Not as far as the record: dropped.
	}
	if f.Err == errRecursion && len(f.TokenParseOptions) > 0 {
		// The expected tokens are the useful message; the marker is not.
		f.Err = nil
	}
	return f
}

// beyondRecord reports whether a failure at pos would be dropped by
// recordFailure, so that callers can skip computing its details.
func (s *ParserState) beyondRecord(pos int) bool {
	return s.fail.Token != nil && pos < s.fail.Pos
}

// fold records an error allocated by a custom rule (recognised by not being
// the shared record) and returns the shared record.
func (s *ParserState) fold(err *ParseError) *ParseError {
	if err == nil || err == &s.fail {
		return err
	}
	return s.recordFailure(err.Pos, err.Token, err.TokenParseOptions, err.Err)
}

// result returns the error for Parse to return: a copy of the record, so
// that it does not keep the parser state alive.
func (s *ParserState) result() *ParseError {
	result := s.fail
	if len(result.TokenParseOptions) > 0 {
		result.TokenParseOptions = uniqueOptions(result.TokenParseOptions, nil)
	}
	return &result
}

// emptyMatchError is the error for a Seq rule none of whose fields matched
// anything.
func emptyMatchError(s *ParserState, ruleName string) *ParseError {
	pos := s.Save()
	if s.beyondRecord(pos) {
		return &s.fail
	}
	tok := s.Next()
	s.Restore(pos)
	return s.Fail(pos, tok, fmt.Errorf("empty match for rule %s", ruleName))
}

// maxRecordedOptions is the number of options recorded at the furthest
// position beyond which they are deduplicated, bounding the record.
const maxRecordedOptions = 128

// uniqueOptions appends the options to dst without duplicates, in order.
// dst may be opts[:0] to compact in place.
func uniqueOptions(opts, dst []TokenParseOptions) []TokenParseOptions {
	for i, o := range opts {
		if !containsOption(opts[:i], o) {
			dst = append(dst, o)
		}
	}
	return dst
}

func containsOption(opts []TokenParseOptions, o TokenParseOptions) bool {
	for _, p := range opts {
		if p == o {
			return true
		}
	}
	return false
}

func (e *ParseError) Error() string {
	var hint string
	if len(e.TokenParseOptions) != 0 {
		var b strings.Builder
		b.WriteString("expected token with ")
		types, values := summariseOptions(e.TokenParseOptions)
		if len(types) > 0 {
			b.WriteString("type ")
			for i, t := range types {
				if i > 0 {
					b.WriteString(" or ")
				}
				b.WriteString(t)
			}
		}
		if len(values) > 0 {
			if len(types) > 0 {
				b.WriteString(", or ")
			}
			b.WriteString("value ")
			for i, v := range values {
				if i > 0 {
					b.WriteString(" or ")
				}
				fmt.Fprintf(&b, "%q", v)
			}
		}
		hint = b.String()
	}
	if e.Err != nil {
		if hint != "" {
			hint += "; "
		}
		hint += e.Err.Error()
	}
	return fmt.Sprintf("%s: %s with value %q: %s", e.Token.Position(), e.Token.Type(), e.Token.Value(), hint)
}

// summariseOptions returns the token types and the token values of the
// options, without duplicates and in the order they were given, which is
// the order the grammar tried them in.
func summariseOptions(opts []TokenParseOptions) (types, values []string) {
	for _, opt := range opts {
		switch {
		case opt.TokenValue != "":
			values = appendUnique(values, opt.TokenValue)
		case opt.TokenType != "":
			types = appendUnique(types, opt.TokenType)
		}
	}
	return types, values
}

func appendUnique(ss []string, s string) []string {
	for _, x := range ss {
		if x == s {
			return ss
		}
	}
	return append(ss, s)
}

// Merge returns the error describing the furthest of the two failures, or
// both if they are at the same position.  Errors returned by the rules of
// this package are all the shared record of the furthest failure (see
// ParserState.Fail), so merging them is a no-op; a custom rule combining
// errors it allocated itself may need it.
func (e *ParseError) Merge(e2 *ParseError) *ParseError {
	if e == nil {
		return e2
	}
	if e2 == nil || e == e2 {
		return e
	}
	if e.Pos > e2.Pos {
		return e
	}
	if e.Pos < e2.Pos {
		return e2
	}
	// A fresh slice (append could write into e's array, shared with other
	// errors), each option once (merging errors that already share options
	// would otherwise multiply them with every level of nesting).
	opts := make([]TokenParseOptions, 0, len(e.TokenParseOptions)+len(e2.TokenParseOptions))
	for _, list := range [][]TokenParseOptions{e.TokenParseOptions, e2.TokenParseOptions} {
		for _, o := range list {
			if !containsOption(opts, o) {
				opts = append(opts, o)
			}
		}
	}
	mergedErr := mergeErrs(e.Err, e2.Err)
	if mergedErr == errRecursion && len(opts) > 0 {
		// The expected tokens are the useful message; the marker is not.
		mergedErr = nil
	}
	return &ParseError{
		Err:               mergedErr,
		Token:             e.Token,
		TokenParseOptions: opts,
		Pos:               e.Pos,
	}
}

// mergeErrs picks the Err to keep when merging two ParseErrors at the same
// position: the first one that is not the internal left-recursion marker,
// which is only meaningful to the memoisation and should not reach users
// when there is anything better to say.
func mergeErrs(err1, err2 error) error {
	switch {
	case err1 != nil && err1 != errRecursion:
		return err1
	case err2 != nil && err2 != errRecursion:
		return err2
	case err1 != nil:
		return err1
	default:
		return err2
	}
}
