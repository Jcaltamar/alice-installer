package migration

import (
	"strconv"
	"strings"
	"unicode"
)

type valueKind uint8

const (
	valueLiteral valueKind = iota
	valueEnvironment
)

type configValue struct {
	kind     valueKind
	text     string
	number   int
	env      string
	fallback *configValue
}
type configObject map[string]configValue
type configRoot map[string]configObject

type tokenKind uint8

const (
	tokenEOF tokenKind = iota
	tokenIdentifier
	tokenString
	tokenNumber
	tokenPunct
	tokenOr
	tokenNullish
)

type token struct {
	kind tokenKind
	text string
}
type parser struct {
	tokens []token
	at     int
}

func parseConfig(source []byte) (configRoot, error) {
	tokens, err := lex(source)
	if err != nil {
		return nil, configError()
	}
	p := parser{tokens: tokens}
	if !p.takeIdentifier("module") || !p.takePunct(".") || !p.takeIdentifier("exports") || !p.takePunct("=") {
		return nil, configError()
	}
	root, err := p.object()
	if err != nil || !p.takePunct(";") || p.current().kind != tokenEOF || !validConfigShape(root) {
		return nil, configError()
	}
	return root, nil
}
func validConfigShape(root configRoot) bool {
	for environment, fields := range root {
		if environment != string(EnvironmentProduction) && environment != string(EnvironmentDevelopment) && environment != "test" {
			return false
		}
		for name := range fields {
			switch name {
			case "dialect", "database", "username", "password", "host", "port":
			default:
				return false
			}
		}
	}
	return true
}

func (p *parser) object() (configRoot, error) {
	if !p.takePunct("{") {
		return nil, configError()
	}
	result := configRoot{}
	for !p.takePunct("}") {
		key, ok := p.key()
		if !ok || !p.takePunct(":") {
			return nil, configError()
		}
		if _, found := result[key]; found {
			return nil, configError()
		}
		fields, err := p.fields()
		if err != nil {
			return nil, err
		}
		result[key] = fields
		if p.takePunct("}") {
			break
		}
		if !p.takePunct(",") {
			return nil, configError()
		}
	}
	return result, nil
}
func (p *parser) fields() (configObject, error) {
	if !p.takePunct("{") {
		return nil, configError()
	}
	result := configObject{}
	for !p.takePunct("}") {
		key, ok := p.key()
		if !ok || !p.takePunct(":") {
			return nil, configError()
		}
		if _, found := result[key]; found {
			return nil, configError()
		}
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		result[key] = value
		if p.takePunct("}") {
			break
		}
		if !p.takePunct(",") {
			return nil, configError()
		}
	}
	return result, nil
}
func (p *parser) key() (string, bool) {
	current := p.current()
	if current.kind != tokenIdentifier && current.kind != tokenString {
		return "", false
	}
	p.at++
	return current.text, true
}
func (p *parser) value() (configValue, error) {
	current := p.current()
	if current.kind == tokenString {
		p.at++
		return configValue{kind: valueLiteral, text: current.text}, nil
	}
	if current.kind == tokenNumber {
		p.at++
		n, err := strconv.Atoi(current.text)
		if err != nil {
			return configValue{}, configError()
		}
		return configValue{kind: valueLiteral, number: n}, nil
	}
	if !p.takeIdentifier("process") || !p.takePunct(".") || !p.takeIdentifier("env") || !p.takePunct(".") {
		return configValue{}, configError()
	}
	nameToken := p.current()
	if nameToken.kind != tokenIdentifier {
		return configValue{}, configError()
	}
	p.at++
	name := nameToken.text
	operator := p.current()
	if operator.kind != tokenOr && operator.kind != tokenNullish {
		return configValue{}, configError()
	}
	p.at++
	fallback, err := p.valueLiteral()
	if err != nil {
		return configValue{}, err
	}
	return configValue{kind: valueEnvironment, env: name, fallback: &fallback}, nil
}
func (p *parser) valueLiteral() (configValue, error) {
	current := p.current()
	if current.kind == tokenString {
		p.at++
		return configValue{kind: valueLiteral, text: current.text}, nil
	}
	if current.kind == tokenNumber {
		p.at++
		n, err := strconv.Atoi(current.text)
		if err != nil {
			return configValue{}, configError()
		}
		return configValue{kind: valueLiteral, number: n}, nil
	}
	return configValue{}, configError()
}
func (p *parser) current() token {
	if p.at >= len(p.tokens) {
		return token{kind: tokenEOF}
	}
	return p.tokens[p.at]
}
func (p *parser) takeIdentifier(want string) bool {
	current := p.current()
	if current.kind != tokenIdentifier || current.text != want {
		return false
	}
	p.at++
	return true
}
func (p *parser) takePunct(want string) bool {
	current := p.current()
	if current.kind != tokenPunct || current.text != want {
		return false
	}
	p.at++
	return true
}

func lex(source []byte) ([]token, error) {
	var tokens []token
	for i := 0; i < len(source); {
		if unicode.IsSpace(rune(source[i])) {
			i++
			continue
		}
		if source[i] == '/' && i+1 < len(source) && source[i+1] == '/' {
			i += 2
			for i < len(source) && source[i] != '\n' {
				i++
			}
			continue
		}
		if source[i] == '/' && i+1 < len(source) && source[i+1] == '*' {
			end := i + 2
			for end+1 < len(source) && (source[end] != '*' || source[end+1] != '/') {
				end++
			}
			if end+1 >= len(source) {
				return nil, configError()
			}
			i = end + 2
			continue
		}
		if isIdentifierStart(source[i]) {
			start := i
			i++
			for i < len(source) && isIdentifierPart(source[i]) {
				i++
			}
			tokens = append(tokens, token{tokenIdentifier, string(source[start:i])})
			continue
		}
		if source[i] >= '0' && source[i] <= '9' {
			start := i
			i++
			for i < len(source) && source[i] >= '0' && source[i] <= '9' {
				i++
			}
			tokens = append(tokens, token{tokenNumber, string(source[start:i])})
			continue
		}
		if source[i] == '\'' || source[i] == '"' {
			quote := source[i]
			i++
			var out []byte
			closed := false
			for i < len(source) {
				if source[i] == quote {
					i++
					closed = true
					break
				}
				if source[i] == '\\' {
					if i+1 >= len(source) {
						return nil, configError()
					}
					i++
					if source[i] != '\\' && source[i] != quote && source[i] != 'n' && source[i] != 'r' && source[i] != 't' {
						return nil, configError()
					}
					switch source[i] {
					case 'n':
						out = append(out, '\n')
					case 'r':
						out = append(out, '\r')
					case 't':
						out = append(out, '\t')
					default:
						out = append(out, source[i])
					}
					i++
					continue
				}
				out = append(out, source[i])
				i++
			}
			if !closed {
				return nil, configError()
			}
			tokens = append(tokens, token{tokenString, string(out)})
			continue
		}
		if i+1 < len(source) && source[i] == '|' && source[i+1] == '|' {
			tokens = append(tokens, token{tokenOr, "||"})
			i += 2
			continue
		}
		if i+1 < len(source) && source[i] == '?' && source[i+1] == '?' {
			tokens = append(tokens, token{tokenNullish, "??"})
			i += 2
			continue
		}
		if strings.ContainsRune("{}:,.=;", rune(source[i])) {
			tokens = append(tokens, token{tokenPunct, string(source[i])})
			i++
			continue
		}
		return nil, configError()
	}
	return append(tokens, token{kind: tokenEOF}), nil
}
func isIdentifierStart(b byte) bool {
	return b == '_' || b == '$' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
func isIdentifierPart(b byte) bool { return isIdentifierStart(b) || b >= '0' && b <= '9' }
