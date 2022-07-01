package lexer

import (
	"bufio"
	"errors"
	"strconv"
	"unicode"
)

type Lexer struct {
	CurrTok    int
	Identifier string
	NumVal     float64
	reader     *bufio.Reader
}

const (
	// Type Tokens
	TokIdentifier int = -1
	TokNumVal     int = -2

	// Keyword Tokens
	TokDef    int = -3
	TokExtern int = -4
	TokSet    int = -5
	TokReturn int = -6
	TokConst  int = -7
	TokIf     int = -8
	TokElse   int = -9
	TokWhile  int = -10

	TokEOF int = -99
)

func NewLexer(reader *bufio.Reader) *Lexer {
	l := Lexer{
		CurrTok:    0,
		Identifier: "",
		NumVal:     0,
		reader:     reader,
	}

	return &l
}

func (l *Lexer) NextToken() {
	l.CurrTok = l.parseToken()
}

func (l *Lexer) parseToken() int {
	chr, err := l.reader.ReadByte()
	if err != nil {
		return TokEOF
	}

	chr, err = l.skipCommentsAndWhitespace(chr, err)
	if err != nil {
		return TokEOF
	}

	// word token
	if l.validFirstIdentChar(chr) {
		str := string(chr)

		peek, _ := l.reader.Peek(1)
		for l.validIdentChar(peek[0]) {
			chr, _ = l.reader.ReadByte()
			str += string(chr)
			peek, _ = l.reader.Peek(1)
		}

		if str == "def" {
			return TokDef
		} else if str == "extern" {
			return TokExtern
		} else if str == "set" {
			return TokSet
		} else if str == "const" {
			return TokConst
		} else if str == "return" {
			return TokReturn
		} else if str == "if" {
			return TokIf
		} else if str == "else" {
			return TokElse
		} else if str == "while" {
			return TokWhile
		}

		l.Identifier = str
		return TokIdentifier

	}

	// Number token
	if unicode.IsDigit(rune(chr)) {
		numStr := string(chr)

		peek, _ := l.reader.Peek(1)
		for unicode.IsDigit(rune(peek[0])) {
			chr, _ = l.reader.ReadByte()
			numStr += string(chr)
			peek, _ = l.reader.Peek(1)
		}

		peek, _ = l.reader.Peek(1)
		if peek[0] == '.' {
			chr, _ = l.reader.ReadByte()
			numStr += "."

			peek, _ = l.reader.Peek(1)
			for unicode.IsDigit(rune(peek[0])) {
				chr, _ = l.reader.ReadByte()
				numStr += string(chr)
				peek, _ = l.reader.Peek(1)
			}
		}

		l.NumVal, _ = strconv.ParseFloat(numStr, 64)
		return TokNumVal
	}

	// Return other tokens as they are
	return int(chr)
}

func (l *Lexer) validIdentChar(chr byte) bool {
	return unicode.IsLetter(rune(chr)) || unicode.IsDigit(rune(chr)) || chr == '_'
}

func (l *Lexer) validFirstIdentChar(chr byte) bool {
	return unicode.IsLetter(rune(chr))
}

func (l *Lexer) skipCommentsAndWhitespace(chr byte, err error) (byte, error) {
	chr, err = l.skipWhitespace(chr, err)
	if err != nil {
		return 0, err
	}

	// Ignore comments
	peek, _ := l.reader.Peek(1)
	if len(peek) < 1 {
		return 0, errors.New("")
	}
	if chr == '/' && peek[0] == '*' {
		// Eat *
		_, err = l.reader.ReadByte()
		if err != nil {
			return 0, err
		}

		peek, _ := l.reader.Peek(2)
		if len(peek) < 2 {
			return 0, errors.New("")
		}
		for peek[0] != '*' || peek[1] != '/' {
			_, err = l.reader.ReadByte()
			if err != nil {
				return 0, err
			}
			peek, _ = l.reader.Peek(2)
			if len(peek) < 2 {
				return 0, errors.New("")
			}
		}

		// Eat */
		_, _ = l.reader.ReadByte()
		_, _ = l.reader.ReadByte()

		chr, err = l.reader.ReadByte()
		if err != nil {
			return 0, err
		}

		chr, err = l.skipWhitespace(chr, err)
		if err != nil {
			return 0, err
		}

	}
	return chr, nil
}

func (l *Lexer) skipWhitespace(chr byte, err error) (byte, error) {
	// Skip whitespace
	for unicode.IsSpace(rune(chr)) {
		chr, err = l.reader.ReadByte()
		if err != nil {
			return 0, err
		}
	}
	return chr, err
}
