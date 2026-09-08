package schema

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var devices = map[string]bool{"android": true, "ios": true, "windows": true, "macos": true, "web": true}
var dictionaries = map[string]map[string]bool{
	"device":       devices,
	"member_level": {"basic": true, "silver": true, "gold": true, "diamond": true},
	"channel":      {"organic": true, "paid": true, "referral": true},
}
var decimal = regexp.MustCompile(`^\d+(\.\d+)?$`)
var integer = regexp.MustCompile(`^\d+$`)

func IsNumeric(field string) bool { return field == "score" || field == "age" }
func NumericValue(field, value string) (float64, bool) {
	if field == "score" {
		return Score(value)
	}
	if field != "age" {
		return 0, false
	}
	value = strings.TrimSpace(value)
	number, err := strconv.ParseFloat(value, 64)
	return number, integer.MatchString(value) && err == nil && number >= 0 && number <= 120
}

func Score(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	number, err := strconv.ParseFloat(value, 64)
	return number, decimal.MatchString(value) && err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) && number >= 0 && number <= 100
}

func ValidateFields(fields map[string]string) error {
	for field, value := range fields {
		if dictionary := dictionaries[field]; dictionary != nil && !dictionary[value] {
			return fmt.Errorf("字段 %s 的值不在字典中", field)
		}
		if IsNumeric(field) || dictionaries[field] != nil {
			if err := ValidateCondition(field, "eq", value); err != nil {
				return err
			}
		}
	}
	return nil
}

// Unknown/legacy fields remain readable strings, not implicitly numeric.
func ValidateCondition(field, op, value string) error {
	if field == "" || field != strings.TrimSpace(field) {
		return fmt.Errorf("画像字段名无效")
	}
	if op != "eq" && op != "in" && op != "gte" && op != "lte" {
		return fmt.Errorf("比较方式无效")
	}
	if !IsNumeric(field) && (op == "gte" || op == "lte") {
		return fmt.Errorf("字段 %s 是文本或枚举，只支持等于或属于", field)
	}
	values := []string{value}
	if op == "in" {
		values = strings.Split(value, ",")
	}
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" {
			return fmt.Errorf("比较值不能为空")
		}
		if dictionary := dictionaries[field]; dictionary != nil && !dictionary[item] {
			return fmt.Errorf("字段 %s 的值 %s 不在字典中", field, item)
		}
		if IsNumeric(field) {
			if _, ok := NumericValue(field, item); !ok {
				if field == "age" {
					return fmt.Errorf("年龄必须为 0～120 的整数")
				}
				return fmt.Errorf("活跃分数必须为 0～100 的数字")
			}
		}
	}
	return nil
}
