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

type PrototypeAST struct {
	ASTNode
	FuncName string
	Params   []string
}

func (p PrototypeAST) CodeGen(*ir.Block) (interface{}, error) {
	irParams := make([]*ir.Param, len(p.Params))
	for i, param := range p.Params {
		irParams[i] = ir.NewParam(param, types.Double)
	}
	return Module.NewFunc(p.FuncName, types.Double, irParams...), nil
}

func (p PrototypeAST) String() string {
	s := p.FuncName + "("
	for i, arg := range p.Params {
		if i < len(p.Params)-1 {
			s = fmt.Sprintf("%s%s,", s, arg)
		} else {
			s = fmt.Sprintf("%s%s)", s, arg)
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

	currentBlock := entry

	for _, stmt := range f.Body {
		gen, err := stmt.CodeGen(currentBlock)
		if err != nil {
			return nil, err
		}
		if block, ok := gen.(*ir.Block); ok {
			currentBlock = block
		}
	}

	if currentBlock.Term == nil {
		currentBlock.NewRet(constant.NewFloat(types.Double, 0.0))
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

	ifCurrentBlock := ifBlock

	for _, stmt := range i.IfBody {
		gen, err = stmt.CodeGen(ifCurrentBlock)
		if err != nil {
			return nil, err
		}

		if retBlock, ok := gen.(*ir.Block); ok {
			ifCurrentBlock = retBlock
		}
	}

	var phi *ir.InstPhi

	if ifCurrentBlock.Term != nil {
		if retTerm, ok := ifCurrentBlock.Term.(*ir.TermRet); ok {
			ifVal := retTerm.X

			phi = afterBlock.NewPhi(&ir.Incoming{
				X:    ifVal,
				Pred: ifCurrentBlock,
			})
		}
	}
	ifCurrentBlock.NewBr(afterBlock)

	if i.ElseBody != nil {
		elseBlock := newBlock(block, "if-false-block")
		elseCurrentBlock := elseBlock
		for _, stmt := range i.ElseBody {
			gen, err = stmt.CodeGen(elseCurrentBlock)
			if err != nil {
				return nil, err
			}

			if retBlock, ok := gen.(*ir.Block); ok {
				elseCurrentBlock = retBlock
			}
		}
		if elseCurrentBlock.Term != nil {
			if retTerm, ok := elseCurrentBlock.Term.(*ir.TermRet); ok {
				elseVal := retTerm.X
				elseInc := &ir.Incoming{
					X:    elseVal,
					Pred: elseCurrentBlock,
				}
				if phi == nil {
					// If "if" block returns nothing, set its phi val to this one
					ifInc := &ir.Incoming{
						X:    elseVal,
						Pred: ifCurrentBlock,
					}
					afterBlock.NewPhi(elseInc, ifInc)
				} else {
					phi.Incs = append(phi.Incs, elseInc)
				}
			}
		} else {
			// "Else" returns nothing
			if phi != nil {
				// "if" does, add phi with if block val
				elseInc := &ir.Incoming{
					X:    phi.Incs[0].X,
					Pred: elseCurrentBlock,
				}
				phi.Incs = append(phi.Incs, elseInc)
			}
		}
		elseCurrentBlock.NewBr(afterBlock)
		block.NewCondBr(condVal, ifBlock, elseBlock)
	} else {
		// No else
		block.NewCondBr(condVal, ifBlock, afterBlock)
		if phi == nil {

		}
	}

	if phi != nil {
		afterBlock.NewRet(phi)
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
		args = append(args, gen.(value.Value))

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
	}

	return constant.NewFloat(types.Double, 0), nil
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
