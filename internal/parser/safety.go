package parser

import "fmt"

var blockedOperators = map[string]bool{
	"$where": true, "$function": true, "$accumulator": true,
	"$out": true, "$merge": true,
}

func checkOperatorSafety(v interface{}, depth int) error {
	if depth > 50 {
		return fmt.Errorf("query nesting too deep (limit: 50 levels)")
	}
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			if blockedOperators[k] {
				return fmt.Errorf("%s is not allowed in queries", k)
			}
			if err := checkOperatorSafety(child, depth+1); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, elem := range val {
			if err := checkOperatorSafety(elem, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkPipelineSafety(pipeline []interface{}) error {
	for i, elem := range pipeline {
		if _, ok := elem.(map[string]interface{}); !ok {
			return fmt.Errorf("pipeline stage at index %d must be an object, got %T", i, elem)
		}
		if err := checkOperatorSafety(elem, 0); err != nil {
			return err
		}
	}
	return nil
}

func checkFilterSafety(filter interface{}) error {
	return checkOperatorSafety(filter, 0)
}
