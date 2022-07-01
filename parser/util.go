package parser

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/constant"
	"github.com/llir/llvm/ir/types"
	"github.com/llir/llvm/ir/value"
)

var Module = ir.NewModule()
var namedValues = map[*ir.Func]map[string]value.Value{
	// Global vals
	nil: {},
}

var opPrecedence = map[rune]int{
	'=': 0,
	'!': 0,
	'<': 10,
	'>': 10,
	'+': 20,
	'-': 20,
	'*': 40,
}

func getFunc(module *ir.Module, name string) *ir.Func {
	for _, f := range module.Funcs {
		if f.Name() == name {
			return f
		}
	}
	return nil
}

func retrieveVar(block *ir.Block, name string) (value.Value, error) {
	// STEP 0: Top level var = retrieve const
	if block == nil {
		return namedValues[nil][name], nil
	}

	// STEP 1: Check local block
	if namedVar, ok := namedValues[block.Parent][name]; ok {
		return load(block, namedVar), nil
	}

	// STEP 2: Check const
	if val, ok := namedValues[nil][name]; ok {
		return val, nil
	}

	return nil, errors.New("could not identify var: " + name)
}

func load(block *ir.Block, namedVar value.Value) value.Value {

	return block.NewLoad(getIRType(getType(namedVar)), namedVar)
}

func setVar(block *ir.Block, name string, val value.Value) error {
	// STEP 0: Top level var = create global
	if block == nil {
		namedValues[nil][name] = val

		// If expression isn't constant
		if _, ok := val.(constant.Constant); !ok {
			return errors.New(name + " is not equal to constant expression")
		}

		return nil
	}

	// STEP 1: Check if local var exists
	if block != nil {
		if namedVar, ok := namedValues[block.Parent][name]; ok {
			err := store(block, name, val, namedVar)
			if err != nil {
				return err
			}
			return nil
		}
	}

	// STEP 2: Check if global exists
	if _, ok := namedValues[nil][name]; ok {
		return errors.New("cannot write to constant variable: " + name)
	}

	// STEP 3: Create new local var
	newVar := block.NewAlloca(val.Type())
	namedValues[block.Parent][name] = newVar
	return store(block, name, val, newVar)
}

func store(block *ir.Block, name string, val value.Value, namedVar value.Value) error {
	if _, ok := namedVar.Type().(*types.PointerType); !ok {
		return errors.New("cannot write to variable " + name)
	}
	if !val.Type().Equal(namedVar.Type().(*types.PointerType).ElemType) {
		return errors.New("cannot store incompatible type for: " + name)
	}
	block.NewStore(val, namedVar)
	return nil
}

func IsOperator(chr int) bool {
	_, ok := opPrecedence[rune(chr)]
	return ok
}

func getMD5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

func newBlock(block *ir.Block, name string) *ir.Block {
	hash := getMD5Hash(block.LocalName + name)[0:8]
	newName := name + "_" + hash
	return block.Parent.NewBlock(newName)
}

func genStatements(block *ir.Block, stmts []*StatementAST) (*ir.Block, error) {
	for _, stmt := range stmts {
		gen, err := stmt.CodeGen(block)
		if err != nil {
			return nil, err
		}

		if retBlock, ok := gen.(*ir.Block); ok {
			block = retBlock
		}
	}
	return block, nil
}

func getIRType(typ Type) types.Type {
	switch typ {
	case Double:
		return types.Double
	case String:
		return types.NewPointer(types.I8)
	case Void:
		return types.Void
	}
	return nil
}

func getType(val value.Value) Type {
	t := val.Type()
	if arrType, ok := t.(*types.ArrayType); ok {
		if arrType.ElemType == types.I8 {
			return String
		}
	} else if _, ok := t.(*types.FloatType); ok {
		return Double
	} else if ptrType, ok := t.(*types.PointerType); ok {
		if _, ok := ptrType.ElemType.(*types.FloatType); ok {
			return Double
		}
		if ptr2, ok := ptrType.ElemType.(*types.PointerType); ok {
			if ptr2.ElemType == types.I8 {
				return String
			}
		}
	}
	return Invalid
}
