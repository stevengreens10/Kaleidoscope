package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/constant"
	"github.com/llir/llvm/ir/enum"
	"github.com/llir/llvm/ir/types"
	"github.com/llir/llvm/ir/value"
)

type AST interface {
	fmt.Stringer
	CodeGen(block *ir.Block) (interface{}, error)
}

type ASTNode struct {
}

type FuncAST interface {
	AST
}

type ExprAST interface {
	AST
	IsExpr() bool
}

type Expr struct {
	ASTNode
}

func (e Expr) IsExpr() bool {
	return true
}

type Operator struct {
	Op rune `json:""`
}

func (op Operator) String() string {
	return string(op.Op)
}

func (op Operator) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(op.Op))
}

func (op Operator) GetPrecedence() int {
	return opPrecedence[op.Op]
}

type AssignmentAST struct {
	ASTNode
	VarName string
	Expr    ExprAST
}

func (a AssignmentAST) String() string {
	return a.VarName + " = " + a.Expr.String()
}

func (a AssignmentAST) CodeGen(block *ir.Block) (interface{}, error) {
	gen, err := a.Expr.CodeGen(block)
	if err != nil {
		return nil, err
	}
	err = setVar(block, a.VarName, gen.(value.Value))
	if err != nil {
		return nil, err
	}
	return nil, nil
}

type ReturnAST struct {
	ASTNode
	Expr ExprAST
}

func (r ReturnAST) String() string {
	return "return " + r.Expr.String()
}

func (r ReturnAST) CodeGen(block *ir.Block) (interface{}, error) {
	gen, err := r.Expr.CodeGen(block)
	if err != nil {
		return nil, err
	}
	return block.NewRet(gen.(value.Value)), nil
}

type StatementAST struct {
	ASTNode
	AST AST
}

func (s StatementAST) String() string {
	return s.AST.String() + ";"
}

func (s StatementAST) CodeGen(block *ir.Block) (interface{}, error) {
	return s.AST.CodeGen(block)
}

type Param struct {
	Name string
	Type Type
	Len  int
}

func (p Param) String() string {
	return p.Name
}

type PrototypeAST struct {
	ASTNode
	FuncName   string
	Params     []*Param
	ReturnType Type
}

func (p PrototypeAST) CodeGen(*ir.Block) (interface{}, error) {
	irParams := make([]*ir.Param, len(p.Params))
	for i, param := range p.Params {
		irParams[i] = ir.NewParam(param.Name, getIRType(param.Type))
	}
	return Module.NewFunc(p.FuncName, getIRType(p.ReturnType), irParams...), nil
}

func (p PrototypeAST) String() string {
	s := p.FuncName + "("
	for i, param := range p.Params {
		if i < len(p.Params)-1 {
			s = fmt.Sprintf("%s%s,", s, param)
		} else {
			s = fmt.Sprintf("%s%s)", s, param)
		}
	}
	return s
}

type FunctionAST struct {
	ASTNode
	Prototype *PrototypeAST
	Body      []*StatementAST
}

func (f FunctionAST) CodeGen(*ir.Block) (interface{}, error) {
	theFunc := getFunc(Module, f.Prototype.FuncName)
	if theFunc == nil {
		gen, err := f.Prototype.CodeGen(nil)
		if err != nil {
			return nil, err
		}
		theFunc = gen.(*ir.Func)
	}
	entry := theFunc.NewBlock("entry")

	namedValues[theFunc] = map[string]value.Value{}
	for _, param := range theFunc.Params {
		err := setVar(entry, param.Name(), param)
		if err != nil {
			return nil, err
		}
	}

	currentBlock, err := genStatements(entry, f.Body)
	if err != nil {
		return nil, err
	}

	if currentBlock.Term == nil {
		if f.Prototype.ReturnType != Void {
			return nil, errors.New("non-void function: " + f.Prototype.FuncName + " needs return")
		}
		currentBlock.NewRet(nil)
	}

	return theFunc, nil
}

func (f FunctionAST) String() string {

	return "def " + f.Prototype.String() + "{ " + "..." + " }"
}

type IfAST struct {
	ASTNode
	Cond     ExprAST
	IfBody   []*StatementAST
	ElseBody []*StatementAST
}

func (i IfAST) String() string {
	s := "if " + i.Cond.String() + " {...}"
	if i.ElseBody != nil {
		s = s + " else {...}"
	}
	return s
}

func (i IfAST) CodeGen(block *ir.Block) (interface{}, error) {
	ifBlock := newBlock(block, "if-true-block")
	afterBlock := newBlock(block, "if-after-block")
	gen, err := i.Cond.CodeGen(block)
	if err != nil {
		return nil, err
	}
	condVal := gen.(value.Value)

	condVal = block.NewFCmp(enum.FPredOGT, condVal, constant.NewFloat(types.Double, 0.0))

	ifCurrentBlock, err := genStatements(ifBlock, i.IfBody)
	if err != nil {
		return nil, err
	}

	if ifCurrentBlock.Term == nil {
		ifCurrentBlock.NewBr(afterBlock)
	}

	if i.ElseBody != nil {
		elseBlock := newBlock(block, "if-false-block")
		elseCurrentBlock, err := genStatements(elseBlock, i.ElseBody)
		if err != nil {
			return nil, err
		}

		if elseCurrentBlock.Term == nil {
			elseCurrentBlock.NewBr(afterBlock)
		}
		block.NewCondBr(condVal, ifBlock, elseBlock)
	} else {
		// No else
		block.NewCondBr(condVal, ifBlock, afterBlock)
	}

	return afterBlock, nil
}

type WhileAST struct {
	ASTNode
	Cond ExprAST
	Body []*StatementAST
}

func (w WhileAST) String() string {
	return "while " + w.Cond.String() + " {...};"
}

func (w WhileAST) CodeGen(block *ir.Block) (interface{}, error) {
	testBlock := newBlock(block, "while-test")
	loopBlock := newBlock(block, "while-loop")
	afterBlock := newBlock(block, "while-after")

	gen, err := w.Cond.CodeGen(testBlock)
	if err != nil {
		return nil, err
	}
	condVal := gen.(value.Value)
	condVal = testBlock.NewFCmp(enum.FPredOGT, condVal, constant.NewFloat(types.Double, 0.0))
	testBlock.NewCondBr(condVal, loopBlock, afterBlock)

	block.NewBr(testBlock)

	loopCurrentBlock, err := genStatements(loopBlock, w.Body)
	if err != nil {
		return nil, err
	}

	if loopCurrentBlock.Term == nil {
		loopCurrentBlock.NewBr(testBlock)
	}

	return afterBlock, nil
}

type CallExprAST struct {
	Expr
	FuncName string
	Args     []ExprAST
}

func (c CallExprAST) CodeGen(block *ir.Block) (interface{}, error) {
	theFunc := getFunc(Module, c.FuncName)
	if theFunc == nil {
		return nil, errors.New("could not find function: " + c.FuncName)
	}
	var args []value.Value
	for _, arg := range c.Args {
		gen, err := arg.CodeGen(block)
		if err != nil {
			return nil, err
		}

		arg := gen.(value.Value)

		args = append(args, arg)

	}
	return block.NewCall(theFunc, args...), nil
}

func (c CallExprAST) String() string {
	s := fmt.Sprintf("%s(", c.FuncName)
	for i, arg := range c.Args {
		if i < len(c.Args)-1 {
			s = fmt.Sprintf("%s%s,", s, arg)
		} else {
			s = fmt.Sprintf("%s%s)", s, arg)
		}
	}
	return s
}

type BinaryExprAST struct {
	Expr
	Lhs      ExprAST
	Operator *Operator
	Rhs      ExprAST
}

func (b BinaryExprAST) CodeGen(block *ir.Block) (interface{}, error) {
	if block == nil {
		return nil, errors.New("can not use binary expression at top level")
	}
	gen, err := b.Lhs.CodeGen(block)
	if err != nil {
		return nil, err
	}
	leftValue := gen.(value.Value)

	gen, err = b.Rhs.CodeGen(block)
	if err != nil {
		return nil, err
	}
	rightValue := gen.(value.Value)

	if getType(leftValue) != getType(rightValue) {
		return nil, errors.New("types in binary expression must match")
	}

	var val value.Value

	switch getType(leftValue) {
	case Double:
		val, err = b.handleDoubleOps(block, leftValue, rightValue)
		break
	case String:
		val, err = b.handleStringOps(block, leftValue, rightValue)
		break
	default:
		val = nil
		err = errors.New("unexpected type in binary expression")
	}

	if err != nil {
		return nil, err
	}

	return val, nil

}

func (b BinaryExprAST) handleStringOps(block *ir.Block, leftValue value.Value, rightValue value.Value) (value.Value, error) {
	var val value.Value
	var err error

	switch b.Operator.Op {
	default:
		val = nil
		err = errors.New("unsupported operator for double: " + string(b.Operator.Op))
	}
	return val, err
}

func (b BinaryExprAST) handleDoubleOps(block *ir.Block, leftValue value.Value, rightValue value.Value) (value.Value, error) {
	switch b.Operator.Op {

	case '*':
		return block.NewFMul(leftValue, rightValue), nil
	case '+':
		return block.NewFAdd(leftValue, rightValue), nil
	case '-':
		return block.NewFSub(leftValue, rightValue), nil
	case '<':
		cmp := block.NewFCmp(enum.FPredOLT, leftValue, rightValue)
		return block.NewUIToFP(cmp, types.Double), nil
	case '>':
		cmp := block.NewFCmp(enum.FPredOGT, leftValue, rightValue)
		return block.NewUIToFP(cmp, types.Double), nil
	case '=':
		cmp := block.NewFCmp(enum.FPredOEQ, leftValue, rightValue)
		return block.NewUIToFP(cmp, types.Double), nil
	case '!':
		cmp := block.NewFCmp(enum.FPredONE, leftValue, rightValue)
		return block.NewUIToFP(cmp, types.Double), nil
	}
	return nil, errors.New("unsupported operator for double: " + string(b.Operator.Op))
}

func (b BinaryExprAST) String() string {
	return "(" + b.Lhs.String() + string(b.Operator.Op) + b.Rhs.String() + ")"
}

type NumberExprAST struct {
	Expr
	Val float64
}

func (n NumberExprAST) CodeGen(*ir.Block) (interface{}, error) {
	return constant.NewFloat(types.Double, n.Val), nil
}

func (n NumberExprAST) String() string {
	return fmt.Sprintf("%f", n.Val)
}

type StringExprAST struct {
	Expr
	Val string
}

func (s StringExprAST) CodeGen(block *ir.Block) (interface{}, error) {
	charArray := constant.NewCharArrayFromString(s.Val + string(rune(0)))
	x := block.NewAlloca(charArray.Type())
	block.NewStore(charArray, x)
	val := block.NewBitCast(x, types.I8Ptr)
	return val, nil
}

func (s StringExprAST) String() string {
	return fmt.Sprintf("\"%s\"", s.Val)
}

type VariableExprAST struct {
	Expr
	Name string
}

func (v VariableExprAST) CodeGen(block *ir.Block) (interface{}, error) {
	return retrieveVar(block, v.Name)
}

func (v VariableExprAST) String() string {
	return v.Name
}
