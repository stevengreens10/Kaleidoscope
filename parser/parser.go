package parser

import (
	"Kaleidoscope/lexer"
	"errors"
	"fmt"
	"log"
	"reflect"
)

type Parser struct {
	lexer *lexer.Lexer
}

func NewParser(lexer *lexer.Lexer) *Parser {
	return &Parser{lexer: lexer}
}

func (p *Parser) Shell() {
	p.lexer.NextToken()
	for true {
		//fmt.Printf("> ")
		var result fmt.Stringer
		var err error
		switch p.lexer.CurrTok {
		case lexer.TokEOF:
			//fmt.Println("Received EOF")
			fmt.Println(Module)
			return
		case lexer.TokDef:
			result, err = p.parseFuncDef()
			break
		case lexer.TokExtern:
			result, err = p.parseExternFunc()
			break
		case lexer.TokConst:
			result, err = p.parseAssignment()

		case ';':
			p.lexer.NextToken()
			break
		default:
			result = nil
			err = errors.New("unknown token when parsing top level: " + string(rune(p.lexer.CurrTok)))
			break
		}

		if err != nil {
			log.Fatalf("Error during parse: %s\n", err.Error())
		}

		if !isNil(result) {
			if funcAST, ok := result.(AST); ok {
				_, err = funcAST.CodeGen(nil)
				if err != nil {
					log.Fatalf("Error during code gen: %s\n", err.Error())
				}
			}
		}
	}
}

func (p *Parser) ParseTopLevelExpr() (*FunctionAST, error) {
	stmt, err := p.parseStatement()
	if err != nil {
		return nil, err
	}

	return &FunctionAST{
		Prototype: &PrototypeAST{
			FuncName: "",
			Params:   []string{},
		},
		Body: []*StatementAST{stmt},
	}, nil
}

func (p *Parser) ParsePrimary() (ExprAST, error) {

	switch p.lexer.CurrTok {
	case lexer.TokIdentifier:
		return p.parseIdentifierExpr()
	case lexer.TokNumVal:
		return p.parseNumberExpr()
	case '(':
		return p.parseParenExpr()
	default:
		return nil, errors.New("unknown token when parsing primary: " + string(rune(p.lexer.CurrTok)))
	}
}

func (p *Parser) parseStatement() (*StatementAST, error) {
	var ast AST
	var err error
	switch p.lexer.CurrTok {
	case lexer.TokSet:
		ast, err = p.parseAssignment()
		break
	case lexer.TokReturn:
		ast, err = p.parseReturn()
		break
	case lexer.TokIf:
		ast, err = p.parseIf()
		break
	default:
		ast, err = p.parseExpression()
	}

	if err != nil {
		return nil, err
	}

	if p.lexer.CurrTok != ';' {
		return nil, errors.New("expected ; at end of statement")
	}

	// Eat ;
	p.lexer.NextToken()

	return &StatementAST{
		AST: ast,
	}, nil
}

func (p *Parser) parseIf() (AST, error) {
	// Eat "if"
	p.lexer.NextToken()

	cond, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	ifBody, err := p.parseStatementBlock()
	if err != nil {
		return nil, err
	}

	var elseBody []*StatementAST
	if p.lexer.CurrTok == lexer.TokElse {
		// Eat "else"
		p.lexer.NextToken()
		elseBody, err = p.parseStatementBlock()
		if err != nil {
			return nil, err
		}
	}

	return &IfAST{
		Cond:     cond,
		IfBody:   ifBody,
		ElseBody: elseBody,
	}, nil
}

func (p *Parser) parseAssignment() (AST, error) {
	// Eat "set" or "const"
	p.lexer.NextToken()

	if p.lexer.CurrTok != lexer.TokIdentifier {
		return nil, errors.New("expected identifier after set")
	}

	ident := p.lexer.Identifier
	p.lexer.NextToken()

	if p.lexer.CurrTok != '=' {
		return nil, errors.New("expected = in set statement")
	}
	// Eat =
	p.lexer.NextToken()

	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	return &AssignmentAST{
		VarName: ident,
		Expr:    expr,
	}, nil
}

func (p *Parser) parseReturn() (AST, error) {
	// Eat "return"
	p.lexer.NextToken()

	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	return &ReturnAST{
		Expr: expr,
	}, nil
}

func (p *Parser) parseExpression() (ExprAST, error) {
	lhsExpr, err := p.ParsePrimary()
	if err != nil {
		return nil, err
	}

	return p.parseBinaryExprRHS(0, lhsExpr)
}

func (p *Parser) parseBinaryExprRHS(exprPrecedence int, lhsExpr ExprAST) (ExprAST, error) {

	for true {

		tokPrecedence := -1
		if IsOperator(p.lexer.CurrTok) {
			// parse with consume
			op, _ := p.parseOperator(false)
			tokPrecedence = op.GetPrecedence()
		}

		if tokPrecedence < exprPrecedence {
			return lhsExpr, nil
		}

		op, _ := p.parseOperator(true)
		rhsExpr, err := p.ParsePrimary()
		if err != nil {
			return nil, err
		}

		nextPrecedence := -1
		if IsOperator(p.lexer.CurrTok) {
			// Parse without consume
			nextOp, _ := p.parseOperator(false)
			nextPrecedence = nextOp.GetPrecedence()
		}

		if tokPrecedence < nextPrecedence {
			rhsExpr, err = p.parseBinaryExprRHS(tokPrecedence+1, rhsExpr)
			if rhsExpr == nil {
				return nil, err
			}
		}

		lhsExpr = &BinaryExprAST{
			Lhs:      lhsExpr,
			Operator: op,
			Rhs:      rhsExpr,
		}
	}

	return lhsExpr, nil
}

func (p *Parser) parseExternFunc() (*PrototypeAST, error) {
	// Eat 'extern'
	p.lexer.NextToken()
	prototype, err := p.parseFuncPrototype()
	if p.lexer.CurrTok != ';' {
		return nil, errors.New("expected ; after extern statement")
	}

	// Eat ;
	p.lexer.NextToken()

	return prototype, err
}

func (p *Parser) parseFuncPrototype() (*PrototypeAST, error) {
	if p.lexer.CurrTok != lexer.TokIdentifier {
		return nil, errors.New("invalid identifier for function definition")
	}
	funcName := p.lexer.Identifier
	p.lexer.NextToken()

	if p.lexer.CurrTok != '(' {
		return nil, errors.New("expected ( for function definition")
	}

	// Eat (
	p.lexer.NextToken()

	var params []string
	if p.lexer.CurrTok != ')' {
		for true {
			if p.lexer.CurrTok != lexer.TokIdentifier {
				return nil, errors.New("invalid identifier for function parameter")
			}
			param := p.lexer.Identifier
			p.lexer.NextToken()
			params = append(params, param)

			if p.lexer.CurrTok != ',' && p.lexer.CurrTok != ')' {
				return nil, errors.New("expected , or ) in function definition")
			}

			currTok := p.lexer.CurrTok
			// Eat , or )
			p.lexer.NextToken()

			if currTok == ')' {
				break
			}
		}
	} else {
		// Eat )
		p.lexer.NextToken()
	}

	protoype := &PrototypeAST{
		FuncName: funcName,
		Params:   params,
	}

	return protoype, nil
}

func (p *Parser) parseFuncDef() (*FunctionAST, error) {
	// Eat 'def'
	p.lexer.NextToken()

	prototype, err := p.parseFuncPrototype()
	if err != nil {
		return nil, err
	}

	body, err := p.parseStatementBlock()
	if err != nil {
		return nil, err
	}

	functionAST := &FunctionAST{
		Prototype: prototype,
		Body:      body,
	}

	return functionAST, nil
}

func (p *Parser) parseStatementBlock() ([]*StatementAST, error) {
	if p.lexer.CurrTok != '{' {
		return nil, errors.New("expected { for statement block")
	}
	// Eat {
	p.lexer.NextToken()

	var body []*StatementAST

	// Parse statements
	for true {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}

		body = append(body, stmt)

		if p.lexer.CurrTok == '}' {
			break
		}
	}

	// Eat }
	p.lexer.NextToken()
	return body, nil
}

func (p *Parser) parseIdentifierExpr() (ExprAST, error) {
	id := p.lexer.Identifier
	p.lexer.NextToken()

	if p.lexer.CurrTok != '(' {
		varAST := &VariableExprAST{
			Name: id,
		}
		return varAST, nil
	}

	// Eat (
	p.lexer.NextToken()
	var args []ExprAST
	if p.lexer.CurrTok != ')' {
		for true {
			arg, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)

			if p.lexer.CurrTok != ',' && p.lexer.CurrTok != ')' {
				return nil, errors.New("expected , or ) in function call")
			}

			currTok := p.lexer.CurrTok
			// Eat , or )
			p.lexer.NextToken()

			if currTok == ')' {
				break
			}
		}
	} else {
		// Eat )
		p.lexer.NextToken()
	}

	callExpr := CallExprAST{
		FuncName: id,
		Args:     args,
	}

	return callExpr, nil

}

func (p *Parser) parseNumberExpr() (ExprAST, error) {
	numAST := NumberExprAST{
		Val: p.lexer.NumVal,
	}
	p.lexer.NextToken()
	return &numAST, nil
}

func (p *Parser) parseParenExpr() (ExprAST, error) {
	// Consume '('
	p.lexer.NextToken()

	expression, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	if p.lexer.CurrTok != ')' {
		return nil, errors.New("expected closing ) for expression")
	}
	p.lexer.NextToken()

	return expression, err
}

func (p *Parser) parseOperator(consume bool) (*Operator, error) {
	if !IsOperator(p.lexer.CurrTok) {
		return nil, errors.New("invalid operator between expressions")
	}
	operator := &Operator{Op: rune(p.lexer.CurrTok)}

	if consume {
		p.lexer.NextToken()
	}
	return operator, nil
}

func isNil(i interface{}) bool {
	return i == nil || reflect.ValueOf(i).IsNil()
}
