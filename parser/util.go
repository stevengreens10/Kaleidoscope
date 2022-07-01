package parser

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/constant"
	"github.com/llir/llvm/ir/value"
)

var Module = ir.NewModule()
var namedValues = map[*ir.Func]map[string]value.Value{
	// Global vals
	nil: {},
}

var opPrecedence = map[rune]int{
	'=': 0,
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
	// STEP 0: Top level var = retrieve global
	if block == nil {
		return namedValues[nil][name], nil
	}

	// STEP 1: Check local block
	if val, ok := namedValues[block.Parent][name]; ok {
		return val, nil
	}

	// STEP 2: Check global
	if val, ok := namedValues[nil][name]; ok {
		return val, nil
	}

	return nil, errors.New("could not identify var: " + name)
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
		if _, ok := namedValues[block.Parent][name]; ok {
			namedValues[block.Parent][name] = val
			return nil
		}
	}

	// STEP 2: Check if global exists
	if _, ok := namedValues[nil][name]; ok {
		namedValues[nil][name] = val
		return errors.New("cannot write to constant variable: " + name)
	}

	// STEP 3: Create new local var
	namedValues[block.Parent][name] = val
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
