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

// layerGetter returns a getter function for writeOrderedMap over a Layers map.
func layerGetter(m map[string]*LayerResult) func(string) any {
	return func(name string) any {
		if lr, ok := m[name]; ok {
			return lr
		}
		return nil
	}
}

// writeOrderedMap writes a JSON object whose keys follow LayerOrder.
// getter returns the value for a layer name, or nil if absent.
// If first is true, the leading comma before the key is omitted.
func writeOrderedMap(buf *bytes.Buffer, first bool, key string, getter func(string) any) error {
	if !first {
		buf.WriteByte(',')
	}
	buf.WriteString(`"`)
	buf.WriteString(key)
	buf.WriteString(`":{`)
	innerFirst := true
	for _, name := range LayerOrder {
		v := getter(name)
		if v == nil {
			continue
		}
		if err := writeField(buf, innerFirst, name, v); err != nil {
			return err
		}
		innerFirst = false
	}
	buf.WriteByte('}')
	return nil
}
