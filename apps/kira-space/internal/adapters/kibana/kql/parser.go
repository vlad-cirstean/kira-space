package kql

type node interface{ isNode() }

type (
	orNode  struct{ items []node }
	andNode struct{ items []node }
	notNode struct{ child node }
	// fieldNode is field:value, or field op value when op is one of < <= > >=.
	fieldNode struct {
		field value
		op    string // ":" or a range operator
		val   value
	}
	// textNode is a value with no field.
	textNode struct{ val value }
	// nestedNode is path:{ child }.
	nestedNode struct {
		path  value
		child node
	}
)

func (orNode) isNode()     {}
func (andNode) isNode()    {}
func (notNode) isNode()    {}
func (fieldNode) isNode()  {}
func (textNode) isNode()   {}
func (nestedNode) isNode() {}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token {
	t := p.toks[p.pos]
	if t.kind != tEOF {
		p.pos++
	}
	return t
}

func (p *parser) fail(expected ...string) error {
	t := p.peek()
	found := t.kind.String()
	if t.kind == tWord || t.kind == tQuoted {
		found = "“" + t.raw + "”"
	}
	return &SyntaxError{Pos: t.pos, Expected: expected, Found: found}
}

// parse returns nil for a blank query.
func parse(src string) (node, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	if p.peek().kind == tEOF {
		return nil, nil
	}
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, p.fail("“and”", "“or”", "end of input")
	}
	return n, nil
}

func (p *parser) parseOr() (node, error) {
	first, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	items := []node{first}
	for p.peek().kind == tOr {
		p.next()
		n, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	if len(items) == 1 {
		return first, nil
	}
	return orNode{items}, nil
}

func (p *parser) parseAnd() (node, error) {
	first, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	items := []node{first}
	for p.peek().kind == tAnd {
		p.next()
		n, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	if len(items) == 1 {
		return first, nil
	}
	return andNode{items}, nil
}

func (p *parser) parseNot() (node, error) {
	if p.peek().kind == tNot {
		p.next()
		child, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return notNode{child}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (node, error) {
	if p.peek().kind == tLParen {
		p.next()
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tRParen {
			return nil, p.fail("“)”", "“and”", "“or”")
		}
		p.next()
		return n, nil
	}
	v, err := p.parseValue("a field", "a value", "“(”", "“not”")
	if err != nil {
		return nil, err
	}
	switch k := p.peek().kind; k {
	case tColon:
		p.next()
		return p.parseFieldRest(v)
	case tLt, tLte, tGt, tGte:
		op := p.next().raw
		rv, err := p.parseValue("a value")
		if err != nil {
			return nil, err
		}
		return fieldNode{field: v, op: op, val: rv}, nil
	}
	return textNode{val: v}, nil
}

// parseFieldRest parses what follows "field:".
func (p *parser) parseFieldRest(field value) (node, error) {
	switch p.peek().kind {
	case tLParen:
		p.next()
		n, err := p.parseValueOr(field)
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tRParen {
			return nil, p.fail("“)”", "“and”", "“or”")
		}
		p.next()
		return n, nil
	case tLBrace:
		p.next()
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tRBrace {
			return nil, p.fail("“}”", "“and”", "“or”")
		}
		p.next()
		return nestedNode{path: field, child: n}, nil
	}
	v, err := p.parseValue("a value", "“(”", "“{”")
	if err != nil {
		return nil, err
	}
	return fieldNode{field: field, op: ":", val: v}, nil
}

func (p *parser) parseValueOr(field value) (node, error) {
	first, err := p.parseValueAnd(field)
	if err != nil {
		return nil, err
	}
	items := []node{first}
	for p.peek().kind == tOr {
		p.next()
		n, err := p.parseValueAnd(field)
		if err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	if len(items) == 1 {
		return first, nil
	}
	return orNode{items}, nil
}

func (p *parser) parseValueAnd(field value) (node, error) {
	first, err := p.parseValueNot(field)
	if err != nil {
		return nil, err
	}
	items := []node{first}
	for p.peek().kind == tAnd {
		p.next()
		n, err := p.parseValueNot(field)
		if err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	if len(items) == 1 {
		return first, nil
	}
	return andNode{items}, nil
}

func (p *parser) parseValueNot(field value) (node, error) {
	switch p.peek().kind {
	case tNot:
		p.next()
		child, err := p.parseValueNot(field)
		if err != nil {
			return nil, err
		}
		return notNode{child}, nil
	case tLParen:
		p.next()
		n, err := p.parseValueOr(field)
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tRParen {
			return nil, p.fail("“)”", "“and”", "“or”")
		}
		p.next()
		return n, nil
	}
	v, err := p.parseValue("a value", "“(”", "“not”")
	if err != nil {
		return nil, err
	}
	return fieldNode{field: field, op: ":", val: v}, nil
}

// parseValue reads a quoted string, or consecutive unquoted words joined by their original
// whitespace, as Kibana does ("hello world" is one value).
func (p *parser) parseValue(expected ...string) (value, error) {
	t := p.peek()
	switch t.kind {
	case tQuoted:
		p.next()
		return t.val, nil
	case tWord:
		p.next()
		v := value{parts: append([]part(nil), t.val.parts...)}
		for p.peek().kind == tWord {
			w := p.next()
			v.parts = append(v.parts, part{text: w.space})
			v.parts = append(v.parts, w.val.parts...)
		}
		return v, nil
	}
	return value{}, p.fail(expected...)
}
