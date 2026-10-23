package contracts

import (
	"fmt"
	"strconv"
)

// A pattern is the dialect's own regular expression, implemented the same way
// in Go and Python (tradestocker_research.contracts) instead of with each
// language's regex engine, whose syntax and matching differ.
//
// Grammar (printable ASCII only):
//
//	pattern  = "^" body "$"            the body must match the whole string
//	body     = branch                  a top-level | is refused: group it
//	alt      = branch ("|" branch)*    only inside a group
//	branch   = (atom quantifier?)*
//	atom     = literal | escape | class | "(" alt ")"
//	escape   = "\" one of  \ ^ $ . | ? * + ( ) [ ] { } - /
//	class    = "[" item+ "]"           item: char or char-char; no negation
//	quantifier = "?" | "*" | "+" | "{m}" | "{m,n}"   with m <= n <= 1000
//
// Anything else (\d, \w, \Q, back-references, lookaround, flags, ".", POSIX
// classes, lazy or doubled quantifiers) does not compile. Matching runs a
// Thompson automaton of at most MaxPatternStates states, so its time is
// linear in the input; there is no backtracking.
const (
	MaxPatternLength = 1000
	MaxPatternStates = 10000
	maxRepeat        = 1000
)

type charset [2]uint64

func (c *charset) add(lo, hi byte) {
	for b := int(lo); b <= int(hi); b++ {
		c[b/64] |= 1 << (uint(b) % 64)
	}
}

func (c *charset) has(r rune) bool {
	return r >= 0 && r < 128 && c[r/64]&(1<<(uint(r)%64)) != 0
}

// Syntax tree.
type pnode struct {
	kind     int // pLit, pConcat, pAlt, pRepeat
	set      charset
	children []*pnode
	min, max int // pRepeat; max -1 means unbounded
}

const (
	pLit = iota
	pConcat
	pAlt
	pRepeat
)

type pparser struct {
	s     string
	i     int
	depth int
}

func (p *pparser) fail(format string, args ...any) error {
	return fmt.Errorf("pattern %q at %d: %s", p.s, p.i, fmt.Sprintf(format, args...))
}

const metachars = `\^$.|?*+()[]{}`

func parsePattern(s string) (*pnode, error) {
	if len(s) > MaxPatternLength {
		return nil, fmt.Errorf("pattern longer than %d characters", MaxPatternLength)
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return nil, fmt.Errorf("pattern %q must be printable ASCII", s)
		}
	}
	if len(s) < 2 || s[0] != '^' || s[len(s)-1] != '$' {
		return nil, fmt.Errorf("pattern %q must be ^body$", s)
	}
	p := &pparser{s: s, i: 1}
	n, err := p.alt(len(s)-1, true)
	if err != nil {
		return nil, err
	}
	if p.i != len(s)-1 {
		return nil, p.fail("unexpected %q", s[p.i])
	}
	return n, nil
}

// alt parses up to end (exclusive) or a closing parenthesis.
func (p *pparser) alt(end int, top bool) (*pnode, error) {
	branches := []*pnode{}
	for {
		b, err := p.branch(end)
		if err != nil {
			return nil, err
		}
		branches = append(branches, b)
		if p.i < end && p.s[p.i] == '|' {
			if top {
				return nil, p.fail("top-level | is refused: write ^(a|b)$")
			}
			p.i++
			continue
		}
		break
	}
	if len(branches) == 1 {
		return branches[0], nil
	}
	return &pnode{kind: pAlt, children: branches}, nil
}

func (p *pparser) branch(end int) (*pnode, error) {
	n := &pnode{kind: pConcat}
	for p.i < end && p.s[p.i] != '|' && p.s[p.i] != ')' {
		atom, err := p.atom(end)
		if err != nil {
			return nil, err
		}
		if atom, err = p.quantifier(end, atom); err != nil {
			return nil, err
		}
		n.children = append(n.children, atom)
	}
	return n, nil
}

func (p *pparser) atom(end int) (*pnode, error) {
	c := p.s[p.i]
	switch c {
	case '(':
		if p.depth++; p.depth > 32 {
			return nil, p.fail("groups nested deeper than 32")
		}
		p.i++
		if p.i < end && p.s[p.i] == '?' {
			return nil, p.fail("(? groups, flags and lookaround are not supported")
		}
		if p.i < end && p.s[p.i] == ')' {
			return nil, p.fail("empty group")
		}
		n, err := p.alt(end, false)
		if err != nil {
			return nil, err
		}
		if p.i >= end || p.s[p.i] != ')' {
			return nil, p.fail("missing )")
		}
		p.i++
		p.depth--
		return n, nil
	case '[':
		return p.class(end)
	case '\\':
		b, err := p.escape(end, metachars+"-/")
		if err != nil {
			return nil, err
		}
		n := &pnode{kind: pLit}
		n.set.add(b, b)
		return n, nil
	case '^', '$', '.', '?', '*', '+', ')', ']', '{', '}':
		return nil, p.fail("%q must be escaped here", c)
	}
	p.i++
	n := &pnode{kind: pLit}
	n.set.add(c, c)
	return n, nil
}

func (p *pparser) escape(end int, allowed string) (byte, error) {
	if p.i+1 >= end {
		return 0, p.fail("pattern ends inside an escape")
	}
	e := p.s[p.i+1]
	for i := 0; i < len(allowed); i++ {
		if allowed[i] == e {
			p.i += 2
			return e, nil
		}
	}
	return 0, p.fail(`escape \%c is not supported`, e)
}

func (p *pparser) class(end int) (*pnode, error) {
	p.i++ // [
	n := &pnode{kind: pLit}
	if p.i < end && p.s[p.i] == '^' {
		return nil, p.fail("negated classes are not supported")
	}
	items := 0
	for {
		if p.i >= end {
			return nil, p.fail("missing ]")
		}
		if p.s[p.i] == ']' {
			break
		}
		lo, err := p.classChar(end)
		if err != nil {
			return nil, err
		}
		hi := lo
		if p.i+1 < end && p.s[p.i] == '-' && p.s[p.i+1] != ']' {
			p.i++
			if hi, err = p.classChar(end); err != nil {
				return nil, err
			}
			if hi < lo {
				return nil, p.fail("range %c-%c is reversed", lo, hi)
			}
		}
		n.set.add(lo, hi)
		items++
	}
	if items == 0 {
		return nil, p.fail("empty class")
	}
	p.i++ // ]
	return n, nil
}

func (p *pparser) classChar(end int) (byte, error) {
	c := p.s[p.i]
	switch c {
	case '\\':
		return p.escape(end, `\]-[^`)
	case '[', '^':
		return 0, p.fail("%q must be escaped inside a class", c)
	}
	p.i++
	return c, nil
}

func (p *pparser) quantifier(end int, atom *pnode) (*pnode, error) {
	if p.i >= end {
		return atom, nil
	}
	min, max := 0, 0
	switch p.s[p.i] {
	case '?':
		min, max = 0, 1
		p.i++
	case '*':
		min, max = 0, -1
		p.i++
	case '+':
		min, max = 1, -1
		p.i++
	case '{':
		close := p.i + 1
		for close < end && p.s[close] != '}' {
			close++
		}
		if close >= end {
			return nil, p.fail("missing }")
		}
		body := p.s[p.i+1 : close]
		lo, hi, ok := splitCount(body)
		if !ok {
			return nil, p.fail("{%s} is not {m} or {m,n} with m <= n <= %d", body, maxRepeat)
		}
		min, max = lo, hi
		p.i = close + 1
	default:
		return atom, nil
	}
	if p.i < end && (p.s[p.i] == '?' || p.s[p.i] == '*' || p.s[p.i] == '+' || p.s[p.i] == '{') {
		return nil, p.fail("a quantifier cannot follow a quantifier")
	}
	return &pnode{kind: pRepeat, children: []*pnode{atom}, min: min, max: max}, nil
}

func splitCount(s string) (int, int, bool) {
	// Unsigned ASCII digits only, no leading zero: Atoi alone would also take
	// "+1" and "-1".
	digits := func(t string) (int, bool) {
		if t == "" || len(t) > 4 || (len(t) > 1 && t[0] == '0') {
			return 0, false
		}
		for i := 0; i < len(t); i++ {
			if t[i] < '0' || t[i] > '9' {
				return 0, false
			}
		}
		n, err := strconv.Atoi(t)
		return n, err == nil && n <= maxRepeat
	}
	lo, rest, hasComma := cutByte(s, ',')
	m, ok := digits(lo)
	if !ok {
		return 0, 0, false
	}
	if !hasComma {
		return m, m, true
	}
	n, ok := digits(rest)
	return m, n, ok && m <= n
}

func cutByte(s string, b byte) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

// Automaton.
type pstate struct {
	match     bool // consumes one character in set, then goes to out
	set       charset
	out, out1 int // -1 for none; a non-matching state is a split (out, out1) or the accept state (both -1)
}

type compiledPattern struct {
	states []pstate
	start  int
}

type builder struct{ states []pstate }

func (b *builder) add(s pstate) (int, error) {
	if len(b.states) >= MaxPatternStates {
		return 0, fmt.Errorf("pattern needs more than %d automaton states", MaxPatternStates)
	}
	b.states = append(b.states, s)
	return len(b.states) - 1, nil
}

// frag is a piece of automaton: its start and the dangling exits to patch.
type frag struct {
	start int
	exits []*int
}

func (b *builder) patch(exits []*int, to int) {
	for _, e := range exits {
		*e = to
	}
}

func (b *builder) build(n *pnode) (frag, error) {
	switch n.kind {
	case pLit:
		i, err := b.add(pstate{match: true, set: n.set, out: -1, out1: -1})
		if err != nil {
			return frag{}, err
		}
		return frag{i, []*int{&b.states[i].out}}, nil
	case pConcat:
		if len(n.children) == 0 {
			i, err := b.add(pstate{out: -1, out1: -2}) // epsilon
			if err != nil {
				return frag{}, err
			}
			return frag{i, []*int{&b.states[i].out}}, nil
		}
		first, err := b.build(n.children[0])
		if err != nil {
			return frag{}, err
		}
		exits := first.exits
		for _, c := range n.children[1:] {
			next, err := b.build(c)
			if err != nil {
				return frag{}, err
			}
			b.patch(exits, next.start)
			exits = next.exits
		}
		return frag{first.start, exits}, nil
	case pAlt:
		var exits []*int
		start := -1
		for _, c := range n.children {
			f, err := b.build(c)
			if err != nil {
				return frag{}, err
			}
			exits = append(exits, f.exits...)
			if start == -1 {
				start = f.start
				continue
			}
			i, err := b.add(pstate{out: start, out1: f.start})
			if err != nil {
				return frag{}, err
			}
			start = i
		}
		return frag{start, exits}, nil
	case pRepeat:
		return b.repeat(n.children[0], n.min, n.max)
	}
	return frag{}, fmt.Errorf("unknown pattern node")
}

// repeat builds child{min,max}: min copies, then max-min optional copies, or
// a loop when max is unbounded.
func (b *builder) repeat(child *pnode, min, max int) (frag, error) {
	parts := []frag{}
	for i := 0; i < min; i++ {
		f, err := b.build(child)
		if err != nil {
			return frag{}, err
		}
		parts = append(parts, f)
	}
	if max == -1 {
		f, err := b.build(child)
		if err != nil {
			return frag{}, err
		}
		split, err := b.add(pstate{out: f.start, out1: -1})
		if err != nil {
			return frag{}, err
		}
		b.patch(f.exits, split)
		parts = append(parts, frag{split, []*int{&b.states[split].out1}})
	} else {
		for i := min; i < max; i++ {
			f, err := b.build(child)
			if err != nil {
				return frag{}, err
			}
			split, err := b.add(pstate{out: f.start, out1: -1})
			if err != nil {
				return frag{}, err
			}
			parts = append(parts, frag{split, append(f.exits, &b.states[split].out1)})
		}
	}
	if len(parts) == 0 {
		i, err := b.add(pstate{out: -1, out1: -2}) // {0} or {0,0}: matches empty
		if err != nil {
			return frag{}, err
		}
		return frag{i, []*int{&b.states[i].out}}, nil
	}
	for i := 1; i < len(parts); i++ {
		b.patch(parts[i-1].exits, parts[i].start)
	}
	return frag{parts[0].start, parts[len(parts)-1].exits}, nil
}

// compilePattern parses and builds a pattern. State slices are preallocated so
// the exit pointers stay valid.
func compilePattern(s string) (*compiledPattern, error) {
	tree, err := parsePattern(s)
	if err != nil {
		return nil, err
	}
	b := &builder{states: make([]pstate, 0, MaxPatternStates)}
	f, err := b.build(tree)
	if err != nil {
		return nil, err
	}
	accept, err := b.add(pstate{out: -1, out1: -1})
	if err != nil {
		return nil, err
	}
	b.patch(f.exits, accept)
	return &compiledPattern{states: b.states, start: f.start}, nil
}

// matches reports whether the whole string matches. It keeps the set of live
// character-consuming states (marked per step), so the work per character is
// bounded by the number of states.
func (c *compiledPattern) matches(s string) bool {
	mark := make([]int, len(c.states))
	stack := make([]int, 0, 16)
	step := 0
	// closure adds to list every consuming state reachable from i without
	// consuming a character, and reports whether the accept state is reachable.
	closure := func(list []int, i int) ([]int, bool) {
		accept := false
		stack = append(stack[:0], i)
		for len(stack) > 0 {
			j := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if j < 0 || mark[j] == step {
				continue
			}
			mark[j] = step
			st := c.states[j]
			switch {
			case st.match:
				list = append(list, j)
			case st.out == -1 && st.out1 == -1:
				accept = true
			default:
				stack = append(stack, st.out)
				if st.out1 >= 0 {
					stack = append(stack, st.out1)
				}
			}
		}
		return list, accept
	}
	step++
	current, accept := closure(nil, c.start)
	for _, r := range s {
		step++
		next, nextAccept := []int(nil), false
		for _, i := range current {
			if st := c.states[i]; st.set.has(r) {
				var a bool
				next, a = closure(next, st.out)
				nextAccept = nextAccept || a
			}
		}
		if len(next) == 0 && !nextAccept {
			return false
		}
		current, accept = next, nextAccept
	}
	return accept
}
