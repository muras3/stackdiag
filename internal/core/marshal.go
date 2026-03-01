package core

import "bytes"

// writeField writes `,"key":value` to buf. If first is true, the leading comma is omitted.
func writeField(buf *bytes.Buffer, first bool, key string, val any) error {
	if !first {
		buf.WriteByte(',')
	}
	buf.WriteString(`"`)
	buf.WriteString(key)
	buf.WriteString(`":`)
	b, err := marshalNoEscape(val)
	if err != nil {
		return err
	}
	buf.Write(b)
	return nil
}

// writeOrderedMap writes a JSON object whose keys follow LayerOrder.
// getter returns the value for a layer name, or nil if absent.
func writeOrderedMap(buf *bytes.Buffer, key string, getter func(string) any) error {
	buf.WriteString(`,"`)
	buf.WriteString(key)
	buf.WriteString(`":{`)
	first := true
	for _, name := range LayerOrder {
		v := getter(name)
		if v == nil {
			continue
		}
		if err := writeField(buf, first, name, v); err != nil {
			return err
		}
		first = false
	}
	buf.WriteByte('}')
	return nil
}
