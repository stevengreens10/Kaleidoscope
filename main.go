package main

import (
	"Kaleidoscope/lexer"
	"Kaleidoscope/parser"
	"bufio"
	"log"
	"os"
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	if len(os.Args) == 2 {
		file, err := os.Open(os.Args[1])
		if err != nil {
			log.Fatalln(err.Error())
		}
		reader = bufio.NewReader(file)
	}

	lex := lexer.NewLexer(reader)
	parse := parser.NewParser(lex)

	parse.Shell()

}
