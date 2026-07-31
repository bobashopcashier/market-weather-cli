package dataframe

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// EvalExpression parses and evaluates a small, side-effect-free expression
// against values keyed by column name.
func EvalExpression(expression string, values map[string]any) (any, error) {
	tokens, err := lexExpression(expression, values)
	if err != nil {
		return nil, err
	}
	parser := expressionParser{tokens: tokens}
	node, err := parser.parse()
	if err != nil {
		return nil, err
	}
	result, err := node.eval(values)
	if err != nil {
		return nil, fmt.Errorf("evaluate expression: %w", err)
	}
	return result, nil
}

// EvalPredicate evaluates expression and requires a Boolean result.
func EvalPredicate(expression string, values map[string]any) (bool, error) {
	result, err := EvalExpression(expression, values)
	if err != nil {
		return false, err
	}
	predicate, ok := Bool(result)
	if !ok {
		return false, fmt.Errorf("expression result is %T, want bool", result)
	}
	return predicate, nil
}

type expressionTokenKind int

const (
	tokenEOF expressionTokenKind = iota
	tokenNumber
	tokenString
	tokenBool
	tokenNull
	tokenIdentifier
	tokenLeftParen
	tokenRightParen
	tokenComma
	tokenPlus
	tokenMinus
	tokenMultiply
	tokenDivide
	tokenModulo
	tokenNot
	tokenKeywordNot
	tokenEqual
	tokenNotEqual
	tokenLess
	tokenLessEqual
	tokenGreater
	tokenGreaterEqual
	tokenAnd
	tokenOr
	tokenIn
)

type expressionToken struct {
	kind  expressionTokenKind
	text  string
	value any
	pos   int
}

func lexExpression(input string, values map[string]any) ([]expressionToken, error) {
	lexer := expressionLexer{input: input, values: values}
	tokens := make([]expressionToken, 0, 16)
	for {
		token, err := lexer.next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
		if token.kind == tokenEOF {
			return tokens, nil
		}
	}
}

type expressionLexer struct {
	input  string
	values map[string]any
	pos    int
}

func (l *expressionLexer) next() (expressionToken, error) {
	for l.pos < len(l.input) && isExpressionSpace(l.input[l.pos]) {
		l.pos++
	}
	if l.pos == len(l.input) {
		return expressionToken{kind: tokenEOF, pos: l.pos}, nil
	}

	start := l.pos
	current := l.input[l.pos]
	switch current {
	case '(':
		l.pos++
		return expressionToken{kind: tokenLeftParen, text: "(", pos: start}, nil
	case ')':
		l.pos++
		return expressionToken{kind: tokenRightParen, text: ")", pos: start}, nil
	case ',':
		l.pos++
		return expressionToken{kind: tokenComma, text: ",", pos: start}, nil
	case '+':
		l.pos++
		return expressionToken{kind: tokenPlus, text: "+", pos: start}, nil
	case '-':
		l.pos++
		return expressionToken{kind: tokenMinus, text: "-", pos: start}, nil
	case '*':
		l.pos++
		return expressionToken{kind: tokenMultiply, text: "*", pos: start}, nil
	case '/':
		l.pos++
		return expressionToken{kind: tokenDivide, text: "/", pos: start}, nil
	case '%':
		l.pos++
		return expressionToken{kind: tokenModulo, text: "%", pos: start}, nil
	case '!':
		l.pos++
		if l.consume('=') {
			return expressionToken{kind: tokenNotEqual, text: "!=", pos: start}, nil
		}
		return expressionToken{kind: tokenNot, text: "!", pos: start}, nil
	case '=':
		l.pos++
		if l.consume('=') {
			return expressionToken{kind: tokenEqual, text: "==", pos: start}, nil
		}
		return expressionToken{}, fmt.Errorf("parse expression at byte %d: use == for equality", start)
	case '<':
		l.pos++
		if l.consume('=') {
			return expressionToken{kind: tokenLessEqual, text: "<=", pos: start}, nil
		}
		return expressionToken{kind: tokenLess, text: "<", pos: start}, nil
	case '>':
		l.pos++
		if l.consume('=') {
			return expressionToken{kind: tokenGreaterEqual, text: ">=", pos: start}, nil
		}
		return expressionToken{kind: tokenGreater, text: ">", pos: start}, nil
	case '&':
		l.pos++
		if l.consume('&') {
			return expressionToken{kind: tokenAnd, text: "&&", pos: start}, nil
		}
		return expressionToken{}, fmt.Errorf("parse expression at byte %d: use && for logical and", start)
	case '|':
		l.pos++
		if l.consume('|') {
			return expressionToken{kind: tokenOr, text: "||", pos: start}, nil
		}
		return expressionToken{}, fmt.Errorf("parse expression at byte %d: use || for logical or", start)
	case '\'', '"':
		return l.scanString(current)
	case '`':
		return l.scanQuotedIdentifier()
	}

	if isExpressionDigit(current) || (current == '.' && l.hasNextDigit()) {
		return l.scanNumber()
	}
	if isIdentifierStart(current) {
		return l.scanIdentifier(), nil
	}
	return expressionToken{}, fmt.Errorf("parse expression at byte %d: unexpected character %q", start, current)
}

func (l *expressionLexer) consume(expected byte) bool {
	if l.pos >= len(l.input) || l.input[l.pos] != expected {
		return false
	}
	l.pos++
	return true
}

func (l *expressionLexer) hasNextDigit() bool {
	return l.pos+1 < len(l.input) && isExpressionDigit(l.input[l.pos+1])
}

func (l *expressionLexer) scanNumber() (expressionToken, error) {
	start := l.pos
	if l.input[l.pos] == '.' {
		l.pos++
		for l.pos < len(l.input) && isExpressionDigit(l.input[l.pos]) {
			l.pos++
		}
	} else {
		for l.pos < len(l.input) && isExpressionDigit(l.input[l.pos]) {
			l.pos++
		}
		if l.pos < len(l.input) && l.input[l.pos] == '.' {
			l.pos++
			for l.pos < len(l.input) && isExpressionDigit(l.input[l.pos]) {
				l.pos++
			}
		}
	}
	if l.pos < len(l.input) && (l.input[l.pos] == 'e' || l.input[l.pos] == 'E') {
		exponent := l.pos
		l.pos++
		if l.pos < len(l.input) && (l.input[l.pos] == '+' || l.input[l.pos] == '-') {
			l.pos++
		}
		digits := l.pos
		for l.pos < len(l.input) && isExpressionDigit(l.input[l.pos]) {
			l.pos++
		}
		if digits == l.pos {
			l.pos = exponent
		}
	}

	text := l.input[start:l.pos]
	if l.pos < len(l.input) && isIdentifierStart(l.input[l.pos]) {
		return expressionToken{}, fmt.Errorf("parse expression at byte %d: invalid number %q", start, l.input[start:l.pos+1])
	}
	if !strings.ContainsAny(text, ".eE") {
		if integer, err := strconv.ParseInt(text, 10, 64); err == nil {
			return expressionToken{kind: tokenNumber, text: text, value: integer, pos: start}, nil
		}
	}
	number, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return expressionToken{}, fmt.Errorf("parse expression at byte %d: invalid number %q", start, text)
	}
	return expressionToken{kind: tokenNumber, text: text, value: number, pos: start}, nil
}

func (l *expressionLexer) scanString(quote byte) (expressionToken, error) {
	start := l.pos
	l.pos++
	var value strings.Builder
	for l.pos < len(l.input) {
		current := l.input[l.pos]
		l.pos++
		if current == quote {
			return expressionToken{kind: tokenString, text: l.input[start:l.pos], value: value.String(), pos: start}, nil
		}
		if current != '\\' {
			value.WriteByte(current)
			continue
		}
		if l.pos == len(l.input) {
			break
		}
		escaped := l.input[l.pos]
		l.pos++
		switch escaped {
		case 'n':
			value.WriteByte('\n')
		case 'r':
			value.WriteByte('\r')
		case 't':
			value.WriteByte('\t')
		case '\\', '\'', '"':
			value.WriteByte(escaped)
		default:
			return expressionToken{}, fmt.Errorf("parse expression at byte %d: unsupported escape \\%c", l.pos-2, escaped)
		}
	}
	return expressionToken{}, fmt.Errorf("parse expression at byte %d: unterminated string", start)
}

func (l *expressionLexer) scanQuotedIdentifier() (expressionToken, error) {
	start := l.pos
	l.pos++
	var value strings.Builder
	for l.pos < len(l.input) {
		current := l.input[l.pos]
		l.pos++
		if current == '`' {
			if l.pos < len(l.input) && l.input[l.pos] == '`' {
				value.WriteByte('`')
				l.pos++
				continue
			}
			if value.Len() == 0 {
				return expressionToken{}, fmt.Errorf("parse expression at byte %d: empty quoted identifier", start)
			}
			return expressionToken{kind: tokenIdentifier, text: value.String(), value: value.String(), pos: start}, nil
		}
		value.WriteByte(current)
	}
	return expressionToken{}, fmt.Errorf("parse expression at byte %d: unterminated quoted identifier", start)
}

func (l *expressionLexer) scanIdentifier() expressionToken {
	start := l.pos
	l.pos++
	for l.pos < len(l.input) && isIdentifierPart(l.input[l.pos]) {
		l.pos++
	}
	text := l.input[start:l.pos]
	// A hyphen may be part of a column name or the subtraction operator. An
	// exact column-name match wins; otherwise split at the first hyphen so
	// expressions such as age-fare retain normal arithmetic precedence.
	if hyphen := strings.IndexByte(text, '-'); hyphen > 0 {
		if _, exists := l.values[text]; !exists {
			l.pos = start + hyphen
			text = text[:hyphen]
		}
	}
	switch strings.ToLower(text) {
	case "true":
		return expressionToken{kind: tokenBool, text: text, value: true, pos: start}
	case "false":
		return expressionToken{kind: tokenBool, text: text, value: false, pos: start}
	case "null":
		return expressionToken{kind: tokenNull, text: text, value: nil, pos: start}
	case "not":
		return expressionToken{kind: tokenKeywordNot, text: text, pos: start}
	case "and":
		return expressionToken{kind: tokenAnd, text: text, pos: start}
	case "or":
		return expressionToken{kind: tokenOr, text: text, pos: start}
	case "in":
		return expressionToken{kind: tokenIn, text: text, pos: start}
	default:
		return expressionToken{kind: tokenIdentifier, text: text, value: text, pos: start}
	}
}

func isExpressionSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}

func isExpressionDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func isIdentifierStart(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || value == '_'
}

func isIdentifierPart(value byte) bool {
	return isIdentifierStart(value) || isExpressionDigit(value) || value == '.' || value == '-'
}

type expressionParser struct {
	tokens []expressionToken
	pos    int
}

func (p *expressionParser) parse() (expressionNode, error) {
	if p.current().kind == tokenEOF {
		return nil, fmt.Errorf("parse expression: expression is empty")
	}
	node, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if token := p.current(); token.kind != tokenEOF {
		return nil, fmt.Errorf("parse expression at byte %d: unexpected token %q", token.pos, token.text)
	}
	return node, nil
}

func (p *expressionParser) parseOr() (expressionNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.current().kind == tokenOr {
		operator := p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = binaryExpressionNode{operator: operator.kind, left: left, right: right}
	}
	return left, nil
}

func (p *expressionParser) parseAnd() (expressionNode, error) {
	left, err := p.parseKeywordNot()
	if err != nil {
		return nil, err
	}
	for p.current().kind == tokenAnd {
		operator := p.advance()
		right, err := p.parseKeywordNot()
		if err != nil {
			return nil, err
		}
		left = binaryExpressionNode{operator: operator.kind, left: left, right: right}
	}
	return left, nil
}

func (p *expressionParser) parseKeywordNot() (expressionNode, error) {
	if p.current().kind != tokenKeywordNot {
		return p.parseComparison()
	}
	p.advance()
	operand, err := p.parseKeywordNot()
	if err != nil {
		return nil, err
	}
	return unaryExpressionNode{operator: tokenNot, operand: operand}, nil
}

func (p *expressionParser) parseComparison() (expressionNode, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	operator := p.current()
	if operator.kind == tokenIn {
		p.advance()
		return p.parseMembership(left, operator)
	}
	if !isComparisonOperator(operator.kind) {
		return left, nil
	}
	p.advance()
	right, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	return binaryExpressionNode{operator: operator.kind, left: left, right: right}, nil
}

func (p *expressionParser) parseMembership(left expressionNode, operator expressionToken) (expressionNode, error) {
	if p.current().kind != tokenLeftParen {
		return nil, fmt.Errorf("parse expression at byte %d: in requires a parenthesized list", operator.pos)
	}
	p.advance()
	items := make([]expressionNode, 0, 4)
	if p.current().kind == tokenRightParen {
		p.advance()
		return membershipExpressionNode{value: left, items: items}, nil
	}
	for {
		item, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		if p.current().kind == tokenRightParen {
			p.advance()
			break
		}
		if p.current().kind != tokenComma {
			token := p.current()
			return nil, fmt.Errorf("parse expression at byte %d: expected comma or ) in membership list", token.pos)
		}
		p.advance()
		if p.current().kind == tokenRightParen {
			p.advance()
			break
		}
	}
	return membershipExpressionNode{value: left, items: items}, nil
}

func (p *expressionParser) parseAdditive() (expressionNode, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.current().kind == tokenPlus || p.current().kind == tokenMinus {
		operator := p.advance()
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = binaryExpressionNode{operator: operator.kind, left: left, right: right}
	}
	return left, nil
}

func (p *expressionParser) parseMultiplicative() (expressionNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.current().kind == tokenMultiply || p.current().kind == tokenDivide || p.current().kind == tokenModulo {
		operator := p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = binaryExpressionNode{operator: operator.kind, left: left, right: right}
	}
	return left, nil
}

func (p *expressionParser) parseUnary() (expressionNode, error) {
	operator := p.current()
	if operator.kind != tokenNot && operator.kind != tokenPlus && operator.kind != tokenMinus {
		return p.parsePrimary()
	}
	p.advance()
	operand, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	return unaryExpressionNode{operator: operator.kind, operand: operand}, nil
}

func (p *expressionParser) parsePrimary() (expressionNode, error) {
	token := p.advance()
	switch token.kind {
	case tokenNumber, tokenString, tokenBool, tokenNull:
		return literalExpressionNode{value: token.value}, nil
	case tokenIdentifier:
		return identifierExpressionNode{name: token.text}, nil
	case tokenLeftParen:
		node, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.current().kind != tokenRightParen {
			current := p.current()
			return nil, fmt.Errorf("parse expression at byte %d: expected )", current.pos)
		}
		p.advance()
		return node, nil
	case tokenEOF:
		return nil, fmt.Errorf("parse expression at byte %d: unexpected end of expression", token.pos)
	default:
		return nil, fmt.Errorf("parse expression at byte %d: expected a literal, identifier, or (", token.pos)
	}
}

func (p *expressionParser) current() expressionToken {
	return p.tokens[p.pos]
}

func (p *expressionParser) advance() expressionToken {
	token := p.current()
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return token
}

func isComparisonOperator(kind expressionTokenKind) bool {
	switch kind {
	case tokenEqual, tokenNotEqual, tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		return true
	default:
		return false
	}
}

type expressionNode interface {
	eval(values map[string]any) (any, error)
}

type literalExpressionNode struct {
	value any
}

func (n literalExpressionNode) eval(map[string]any) (any, error) {
	return n.value, nil
}

type identifierExpressionNode struct {
	name string
}

func (n identifierExpressionNode) eval(values map[string]any) (any, error) {
	value, exists := values[n.name]
	if !exists {
		return nil, fmt.Errorf("unknown identifier %q", n.name)
	}
	return value, nil
}

type unaryExpressionNode struct {
	operator expressionTokenKind
	operand  expressionNode
}

func (n unaryExpressionNode) eval(values map[string]any) (any, error) {
	value, err := n.operand.eval(values)
	if err != nil {
		return nil, err
	}
	switch n.operator {
	case tokenNot:
		boolean, ok := Bool(value)
		if !ok {
			return nil, fmt.Errorf("logical not requires bool, got %T", value)
		}
		return !boolean, nil
	case tokenPlus, tokenMinus:
		if IsNull(value) {
			return nil, nil
		}
		number, ok := Float(value)
		if !ok {
			return nil, fmt.Errorf("unary numeric operator requires number, got %T", value)
		}
		if n.operator == tokenMinus {
			return -number, nil
		}
		return number, nil
	default:
		return nil, fmt.Errorf("unsupported unary operator")
	}
}

type binaryExpressionNode struct {
	operator    expressionTokenKind
	left, right expressionNode
}

func (n binaryExpressionNode) eval(values map[string]any) (any, error) {
	left, err := n.left.eval(values)
	if err != nil {
		return nil, err
	}
	if n.operator == tokenAnd || n.operator == tokenOr {
		leftBool, ok := Bool(left)
		if !ok {
			return nil, fmt.Errorf("logical operator requires bool, got %T", left)
		}
		if n.operator == tokenAnd && !leftBool {
			return false, nil
		}
		if n.operator == tokenOr && leftBool {
			return true, nil
		}
		right, err := n.right.eval(values)
		if err != nil {
			return nil, err
		}
		rightBool, ok := Bool(right)
		if !ok {
			return nil, fmt.Errorf("logical operator requires bool, got %T", right)
		}
		if n.operator == tokenAnd {
			return leftBool && rightBool, nil
		}
		return leftBool || rightBool, nil
	}

	right, err := n.right.eval(values)
	if err != nil {
		return nil, err
	}
	switch n.operator {
	case tokenEqual:
		return expressionValuesEqual(left, right), nil
	case tokenNotEqual:
		return !expressionValuesEqual(left, right), nil
	case tokenLess:
		if IsNull(left) || IsNull(right) {
			return false, nil
		}
		comparison, compareErr := expressionCompare(left, right)
		return comparison < 0, compareErr
	case tokenLessEqual:
		if IsNull(left) || IsNull(right) {
			return false, nil
		}
		comparison, compareErr := expressionCompare(left, right)
		return comparison <= 0, compareErr
	case tokenGreater:
		if IsNull(left) || IsNull(right) {
			return false, nil
		}
		comparison, compareErr := expressionCompare(left, right)
		return comparison > 0, compareErr
	case tokenGreaterEqual:
		if IsNull(left) || IsNull(right) {
			return false, nil
		}
		comparison, compareErr := expressionCompare(left, right)
		return comparison >= 0, compareErr
	case tokenPlus:
		if leftString, leftOK := left.(string); leftOK {
			if rightString, rightOK := right.(string); rightOK {
				return leftString + rightString, nil
			}
		}
		return evalNumericBinary(n.operator, left, right)
	case tokenMinus, tokenMultiply, tokenDivide, tokenModulo:
		return evalNumericBinary(n.operator, left, right)
	default:
		return nil, fmt.Errorf("unsupported binary operator")
	}
}

func evalNumericBinary(operator expressionTokenKind, left, right any) (any, error) {
	if IsNull(left) || IsNull(right) {
		return nil, nil
	}
	leftNumber, leftOK := Float(left)
	rightNumber, rightOK := Float(right)
	if !leftOK || !rightOK {
		return nil, fmt.Errorf("numeric operator requires numbers, got %T and %T", left, right)
	}
	switch operator {
	case tokenPlus:
		return leftNumber + rightNumber, nil
	case tokenMinus:
		return leftNumber - rightNumber, nil
	case tokenMultiply:
		return leftNumber * rightNumber, nil
	case tokenDivide:
		if rightNumber == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return leftNumber / rightNumber, nil
	case tokenModulo:
		if rightNumber == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return math.Mod(leftNumber, rightNumber), nil
	default:
		return nil, fmt.Errorf("unsupported numeric operator")
	}
}

type membershipExpressionNode struct {
	value expressionNode
	items []expressionNode
}

func (n membershipExpressionNode) eval(values map[string]any) (any, error) {
	value, err := n.value.eval(values)
	if err != nil {
		return nil, err
	}
	for _, itemNode := range n.items {
		item, err := itemNode.eval(values)
		if err != nil {
			return nil, err
		}
		if expressionValuesEqual(value, item) {
			return true, nil
		}
	}
	return false, nil
}

func expressionValuesEqual(left, right any) bool {
	if IsNull(left) || IsNull(right) {
		return IsNull(left) && IsNull(right)
	}
	if leftNumber, leftOK := Float(left); leftOK {
		if rightNumber, rightOK := Float(right); rightOK {
			return leftNumber == rightNumber
		}
		return false
	}
	switch typed := left.(type) {
	case string:
		rightValue, ok := right.(string)
		return ok && typed == rightValue
	case bool:
		rightValue, ok := right.(bool)
		return ok && typed == rightValue
	default:
		return fmt.Sprintf("%T", left) == fmt.Sprintf("%T", right) && ValueKey(left) == ValueKey(right)
	}
}

func expressionCompare(left, right any) (int, error) {
	if leftNumber, leftOK := Float(left); leftOK {
		if rightNumber, rightOK := Float(right); rightOK {
			switch {
			case leftNumber < rightNumber:
				return -1, nil
			case leftNumber > rightNumber:
				return 1, nil
			default:
				return 0, nil
			}
		}
	}
	if leftText, leftOK := left.(string); leftOK {
		if rightText, rightOK := right.(string); rightOK {
			return strings.Compare(leftText, rightText), nil
		}
	}
	return 0, fmt.Errorf("cannot order %T and %T", left, right)
}
