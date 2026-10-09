// 部分实现改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.）
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

var docScriptDecisionType = reflect.TypeOf(docScriptDecision{})

// docScriptShellJSONError is an intermediate recovery-parser error. The
// command boundary keeps the original typed strict-JSON error when recovery
// cannot be completed without guessing.
type docScriptShellJSONError struct {
	message string
}

func (parseError *docScriptShellJSONError) Error() string { return parseError.message }

func newDocScriptShellJSONError(format string, args ...any) error {
	return &docScriptShellJSONError{message: fmt.Sprintf(format, args...)}
}

// recoverDocScriptDecisionJSON rebuilds only the JSON syntax that
// legacy native-command argument passing can remove. The Go type remains the
// source of truth for object fields and scalar types; ambiguous bare strings
// containing JSON delimiters are rejected instead of guessed.
func recoverDocScriptDecisionJSON(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", newDocScriptShellJSONError("Presentation Decision 不是合法的 UTF-8")
	}
	parser := docScriptShellJSONParser{raw: raw}
	normalized, err := parser.parseValue(docScriptDecisionType)
	if err != nil {
		return "", err
	}
	parser.skipSpace()
	if parser.offset != len(parser.raw) {
		return "", newDocScriptShellJSONError("第 %d 字节后有多余内容", parser.offset)
	}
	return normalized, nil
}

type docScriptShellJSONParser struct {
	raw    string
	offset int
}

func (parser *docScriptShellJSONParser) parseValue(valueType reflect.Type) (string, error) {
	parser.skipSpace()
	if valueType.Kind() == reflect.Pointer {
		if parser.consumeLiteral("null") {
			return "null", nil
		}
		return parser.parseValue(valueType.Elem())
	}

	switch valueType.Kind() {
	case reflect.Struct:
		if parser.consumeLiteral("null") {
			return "null", nil
		}
		return parser.parseStruct(valueType)
	case reflect.Slice:
		if parser.consumeLiteral("null") {
			return "null", nil
		}
		return parser.parseSlice(valueType.Elem())
	case reflect.String:
		return parser.parseString()
	case reflect.Int:
		return parser.parseInt(valueType.Bits())
	default:
		return "", newDocScriptShellJSONError("不支持的 Presentation Decision 字段类型 %s", valueType)
	}
}

func (parser *docScriptShellJSONParser) parseStruct(structType reflect.Type) (string, error) {
	if err := parser.expectByte('{'); err != nil {
		return "", err
	}
	fields := docScriptShellJSONStructFields(structType)
	seenFields := make(map[string]struct{}, len(fields))
	var normalized strings.Builder
	normalized.WriteByte('{')

	for fieldIndex := 0; ; fieldIndex++ {
		parser.skipSpace()
		if parser.consumeByte('}') {
			normalized.WriteByte('}')
			return normalized.String(), nil
		}
		if fieldIndex > 0 {
			if err := parser.expectByte(','); err != nil {
				return "", err
			}
			parser.skipSpace()
		}

		fieldName, err := parser.parseFieldName()
		if err != nil {
			return "", err
		}
		fieldType, ok := fields[fieldName]
		if !ok {
			return "", newDocScriptShellJSONError("字段 %q 不属于 %s", fieldName, structType.Name())
		}
		if _, duplicated := seenFields[fieldName]; duplicated {
			return "", newDocScriptShellJSONError("字段 %q 重复", fieldName)
		}
		seenFields[fieldName] = struct{}{}
		if err := parser.expectByte(':'); err != nil {
			return "", err
		}
		fieldValue, err := parser.parseValue(fieldType)
		if err != nil {
			return "", newDocScriptShellJSONError("字段 %s: %v", fieldName, err)
		}

		if fieldIndex > 0 {
			normalized.WriteByte(',')
		}
		encodedFieldName, _ := json.Marshal(fieldName)
		normalized.Write(encodedFieldName)
		normalized.WriteByte(':')
		normalized.WriteString(fieldValue)
	}
}

func (parser *docScriptShellJSONParser) parseSlice(elementType reflect.Type) (string, error) {
	if err := parser.expectByte('['); err != nil {
		return "", err
	}
	var normalized strings.Builder
	normalized.WriteByte('[')

	for elementIndex := 0; ; elementIndex++ {
		parser.skipSpace()
		if parser.consumeByte(']') {
			normalized.WriteByte(']')
			return normalized.String(), nil
		}
		if elementIndex > 0 {
			if err := parser.expectByte(','); err != nil {
				return "", err
			}
		}
		element, err := parser.parseValue(elementType)
		if err != nil {
			return "", newDocScriptShellJSONError("第 %d 个元素: %v", elementIndex, err)
		}
		if elementIndex > 0 {
			normalized.WriteByte(',')
		}
		normalized.WriteString(element)
	}
}

func (parser *docScriptShellJSONParser) parseFieldName() (string, error) {
	parser.skipSpace()
	if parser.peekByte() == '"' {
		return parser.parseJSONString()
	}
	start := parser.offset
	for parser.offset < len(parser.raw) && docScriptShellJSONFieldByte(parser.raw[parser.offset]) {
		parser.offset++
	}
	fieldName := parser.raw[start:parser.offset]
	if fieldName == "" {
		return "", newDocScriptShellJSONError("第 %d 字节处应为对象字段名", start)
	}
	parser.skipSpace()
	return fieldName, nil
}

func (parser *docScriptShellJSONParser) parseString() (string, error) {
	parser.skipSpace()
	if parser.peekByte() == '"' {
		value, err := parser.parseJSONString()
		if err != nil {
			return "", err
		}
		encoded, _ := json.Marshal(value)
		return string(encoded), nil
	}

	start := parser.offset
	for parser.offset < len(parser.raw) {
		switch parser.raw[parser.offset] {
		case ',', '}', ']':
			value := strings.TrimSpace(parser.raw[start:parser.offset])
			if value == "" {
				return "", newDocScriptShellJSONError("第 %d 字节处应为字符串", start)
			}
			if strings.ContainsAny(value, `"\{[`) {
				return "", newDocScriptShellJSONError("无引号字符串包含有歧义的 JSON 语法")
			}
			encoded, _ := json.Marshal(value)
			return string(encoded), nil
		default:
			parser.offset++
		}
	}
	return "", newDocScriptShellJSONError("第 %d 字节处的无引号字符串未结束", start)
}

func (parser *docScriptShellJSONParser) parseJSONString() (string, error) {
	start := parser.offset
	if err := parser.expectByte('"'); err != nil {
		return "", err
	}
	escaped := false
	for parser.offset < len(parser.raw) {
		current := parser.raw[parser.offset]
		parser.offset++
		if escaped {
			escaped = false
			continue
		}
		if current == '\\' {
			escaped = true
			continue
		}
		if current == '"' {
			var value string
			if err := json.Unmarshal([]byte(parser.raw[start:parser.offset]), &value); err != nil {
				return "", newDocScriptShellJSONError("第 %d 字节处的 JSON 字符串非法: %v", start, err)
			}
			return value, nil
		}
	}
	return "", newDocScriptShellJSONError("第 %d 字节处的 JSON 字符串未结束", start)
}

func (parser *docScriptShellJSONParser) parseInt(bits int) (string, error) {
	parser.skipSpace()
	start := parser.offset
	for parser.offset < len(parser.raw) {
		switch parser.raw[parser.offset] {
		case ',', '}', ']':
			value := strings.TrimSpace(parser.raw[start:parser.offset])
			parsed, err := strconv.ParseInt(value, 10, bits)
			if err != nil {
				return "", newDocScriptShellJSONError("非法整数 %q", value)
			}
			return strconv.FormatInt(parsed, 10), nil
		default:
			parser.offset++
		}
	}
	return "", newDocScriptShellJSONError("第 %d 字节处的整数未结束", start)
}

func (parser *docScriptShellJSONParser) expectByte(expected byte) error {
	parser.skipSpace()
	if !parser.consumeByte(expected) {
		return newDocScriptShellJSONError("第 %[2]d 字节处应为 %[1]q", expected, parser.offset)
	}
	return nil
}

func (parser *docScriptShellJSONParser) consumeByte(expected byte) bool {
	if parser.offset >= len(parser.raw) || parser.raw[parser.offset] != expected {
		return false
	}
	parser.offset++
	return true
}

func (parser *docScriptShellJSONParser) consumeLiteral(literal string) bool {
	parser.skipSpace()
	if !strings.HasPrefix(parser.raw[parser.offset:], literal) {
		return false
	}
	end := parser.offset + len(literal)
	if end < len(parser.raw) && !docScriptShellJSONDelimiterByte(parser.raw[end]) {
		return false
	}
	parser.offset = end
	return true
}

func (parser *docScriptShellJSONParser) peekByte() byte {
	if parser.offset >= len(parser.raw) {
		return 0
	}
	return parser.raw[parser.offset]
}

func (parser *docScriptShellJSONParser) skipSpace() {
	for parser.offset < len(parser.raw) {
		switch parser.raw[parser.offset] {
		case ' ', '\t', '\r', '\n':
			parser.offset++
		default:
			return
		}
	}
}

func docScriptShellJSONStructFields(structType reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, structType.NumField())
	for index := 0; index < structType.NumField(); index++ {
		field := structType.Field(index)
		fieldName := strings.Split(field.Tag.Get("json"), ",")[0]
		if fieldName != "" && fieldName != "-" {
			fields[fieldName] = field.Type
		}
	}
	return fields
}

func docScriptShellJSONFieldByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_' || value == '-'
}

func docScriptShellJSONDelimiterByte(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', ',', '}', ']':
		return true
	default:
		return false
	}
}
