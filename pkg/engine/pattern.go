package engine

import (
	"math"
	"regexp"
	"strings"

	"github.com/golang/glog"

	"github.com/minio/minio/pkg/wildcard"
)

// Operator is string alias that represents selection operators enum
type Operator string

const (
	// Equal stands for ==
	Equal Operator = ""
	// MoreEqual stands for >=
	MoreEqual Operator = ">="
	// LessEqual stands for <=
	LessEqual Operator = "<="
	// NotEqual stands for !
	NotEqual Operator = "!"
	// More stands for >
	More Operator = ">"
	// Less stands for <
	Less Operator = "<"
)

const relativePrefix Operator = "./"
const referenceSign Operator = "$()"

// ValidateValueWithPattern validates value with operators and wildcards
func ValidateValueWithPattern(value, pattern interface{}) bool {
	switch typedPattern := pattern.(type) {
	case bool:
		typedValue, ok := value.(bool)
		if !ok {
			glog.Warningf("Expected bool, found %T", value)
			return false
		}
		return typedPattern == typedValue
	case int:
		return validateValueWithIntPattern(value, int64(typedPattern))
	case int64:
		return validateValueWithIntPattern(value, typedPattern)
	case float64:
		return validateValueWithFloatPattern(value, typedPattern)
	case string:
		return validateValueWithStringPatterns(value, typedPattern)
	case nil:
		return validateValueWithNilPattern(value)
	case map[string]interface{}, []interface{}:
		glog.Warning("Maps and arrays as patterns are not supported")
		return false
	default:
		glog.Warningf("Unknown type as pattern: %T\n", pattern)
		return false
	}
}

// Handler for int values during validation process
func validateValueWithIntPattern(value interface{}, pattern int64) bool {
	switch typedValue := value.(type) {
	case int:
		return int64(typedValue) == pattern
	case int64:
		return typedValue == pattern
	case float64:
		// check that float has no fraction
		if typedValue == math.Trunc(typedValue) {
			return int64(typedValue) == pattern
		}

		glog.Warningf("Expected int, found float: %f\n", typedValue)
		return false
	default:
		glog.Warningf("Expected int, found: %T\n", value)
		return false
	}
}

// Handler for float values during validation process
func validateValueWithFloatPattern(value interface{}, pattern float64) bool {
	switch typedValue := value.(type) {
	case int:
		// check that float has no fraction
		if pattern == math.Trunc(pattern) {
			return int(pattern) == value
		}

		glog.Warningf("Expected float, found int: %d\n", typedValue)
		return false
	case float64:
		return typedValue == pattern
	default:
		glog.Warningf("Expected float, found: %T\n", value)
		return false
	}
}

// Handler for nil values during validation process
func validateValueWithNilPattern(value interface{}) bool {
	switch typed := value.(type) {
	case float64:
		return typed == 0.0
	case int:
		return typed == 0
	case int64:
		return typed == 0
	case string:
		return typed == ""
	case bool:
		return typed == false
	case nil:
		return true
	case map[string]interface{}, []interface{}:
		glog.Warningf("Maps and arrays could not be checked with nil pattern")
		return false
	default:
		glog.Warningf("Unknown type as value when checking for nil pattern: %T\n", value)
		return false
	}
}

// Handler for pattern values during validation process
func validateValueWithStringPatterns(value interface{}, pattern string) bool {
	statements := strings.Split(pattern, "|")
	for _, statement := range statements {
		statement = strings.Trim(statement, " ")
		if validateValueWithStringPattern(value, statement) {
			return true
		}
	}

	return false
}

// Handler for single pattern value during validation process
// Detects if pattern has a number
func validateValueWithStringPattern(value interface{}, pattern string) bool {
	operator := getOperatorFromStringPattern(pattern)
	pattern = pattern[len(operator):]
	number, str := getNumberAndStringPartsFromPattern(pattern)

	if "" == number {
		return validateString(value, str, operator)
	}

	return validateNumberWithStr(value, number, str, operator)
}

// Handler for string values
func validateString(value interface{}, pattern string, operator Operator) bool {
	if NotEqual == operator || Equal == operator {
		strValue, ok := value.(string)
		if !ok {
			glog.Warningf("Expected string, found %T\n", value)
			return false
		}

		wildcardResult := wildcard.Match(pattern, strValue)

		if NotEqual == operator {
			return !wildcardResult
		}

		return wildcardResult
	}

	glog.Warningf("Operators >, >=, <, <= are not applicable to strings")
	return false
}

// validateNumberWithStr applies wildcard to suffix and operator to numerical part
func validateNumberWithStr(value interface{}, patternNumber, patternStr string, operator Operator) bool {
	// pattern has suffix
	if "" != patternStr {
		typedValue, ok := value.(string)
		if !ok {
			glog.Warningf("Number must have suffix: %s", patternStr)
			return false
		}

		valueNumber, valueStr := getNumberAndStringPartsFromPattern(typedValue)
		if !wildcard.Match(patternStr, valueStr) {
			glog.Warningf("Suffix %s has not passed wildcard check: %s", valueStr, patternStr)
			return false
		}

		return validateNumber(valueNumber, patternNumber, operator)
	}

	return validateNumber(value, patternNumber, operator)
}

// validateNumber compares two numbers with operator
func validateNumber(value, pattern interface{}, operator Operator) bool {
	floatPattern, err := convertToFloat(pattern)
	if err != nil {
		return false
	}

	floatValue, err := convertToFloat(value)
	if err != nil {
		return false
	}

	switch operator {
	case Equal:
		return floatValue == floatPattern
	case NotEqual:
		return floatValue != floatPattern
	case More:
		return floatValue > floatPattern
	case MoreEqual:
		return floatValue >= floatPattern
	case Less:
		return floatValue < floatPattern
	case LessEqual:
		return floatValue <= floatPattern
	}

	return false
}

// getOperatorFromStringPattern parses opeartor from pattern
func getOperatorFromStringPattern(pattern string) Operator {
	if len(pattern) < 2 {
		return Equal
	}

	if pattern[:len(MoreEqual)] == string(MoreEqual) {
		return MoreEqual
	}

	if pattern[:len(LessEqual)] == string(LessEqual) {
		return LessEqual
	}

	if pattern[:len(More)] == string(More) {
		return More
	}

	if pattern[:len(Less)] == string(Less) {
		return Less
	}

	if pattern[:len(NotEqual)] == string(NotEqual) {
		return NotEqual
	}

	return Equal
}

// detects numerical and string parts in pattern and returns them
func getNumberAndStringPartsFromPattern(pattern string) (number, str string) {
	regexpStr := `^(\d*(\.\d+)?)(.*)`
	re := regexp.MustCompile(regexpStr)
	matches := re.FindAllStringSubmatch(pattern, -1)
	match := matches[0]
	return match[1], match[3]
}
