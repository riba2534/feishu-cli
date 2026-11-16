package client

import (
	"encoding/binary"
	"fmt"
)

type nativeField struct {
	wire   uint64
	number uint64
	data   []byte
}

type nativeFields map[uint64][]nativeField

func nativeNumber(data []byte, field, value uint64) []byte {
	data = binary.AppendUvarint(data, field<<3)
	return binary.AppendUvarint(data, value)
}

func nativeBytes(data []byte, field uint64, value []byte) []byte {
	data = binary.AppendUvarint(data, field<<3|2)
	data = binary.AppendUvarint(data, uint64(len(value)))
	return append(data, value...)
}

func nativeString(data []byte, field uint64, value string) []byte {
	return nativeBytes(data, field, []byte(value))
}

func parseNative(data []byte) (nativeFields, error) {
	fields := nativeFields{}
	for len(data) > 0 {
		tag, size := binary.Uvarint(data)
		if size <= 0 || tag>>3 == 0 {
			return nil, fmt.Errorf("网页协议字段无效")
		}
		data = data[size:]
		field := nativeField{wire: tag & 7}
		switch field.wire {
		case 0:
			field.number, size = binary.Uvarint(data)
			if size <= 0 {
				return nil, fmt.Errorf("网页协议整数截断或溢出")
			}
		case 1:
			size = 8
		case 2:
			length, prefix := binary.Uvarint(data)
			if prefix <= 0 || length > uint64(len(data)-prefix) {
				return nil, fmt.Errorf("网页协议内容截断")
			}
			data = data[prefix:]
			size = int(length)
			field.data = data[:size]
		case 5:
			size = 4
		default:
			return nil, fmt.Errorf("网页协议不支持的 wire type: %d", field.wire)
		}
		if size > len(data) {
			return nil, fmt.Errorf("网页协议字段截断")
		}
		fields[tag>>3] = append(fields[tag>>3], field)
		data = data[size:]
	}
	return fields, nil
}

func (fields nativeFields) bytes(field uint64) []byte {
	if values := fields[field]; len(values) > 0 && values[0].wire == 2 {
		return values[0].data
	}
	return nil
}

func (fields nativeFields) text(field uint64) string {
	return string(fields.bytes(field))
}

func (fields nativeFields) number(field uint64) uint64 {
	if values := fields[field]; len(values) > 0 && values[0].wire == 0 {
		return values[0].number
	}
	return 0
}

func nativeMapValues(fields nativeFields, field uint64) ([]nativeFields, error) {
	var values []nativeFields
	for _, entry := range fields[field] {
		pair, err := parseNative(entry.data)
		if err != nil {
			return nil, err
		}
		value, err := parseNative(pair.bytes(2))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
