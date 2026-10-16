package feed

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

// parseXML reads every RecordElement in the document. Each record's fields are its
// child elements or attributes, named by the mapping. DTDs and entities beyond the
// five predefined ones are not expanded, so a hostile file cannot make the parser
// fetch anything or expand without bound.
func (m *Mapping) parseXML(data []byte) ([]Record, []RowError, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	dec.Entity = nil
	var recs []Record
	var errs []RowError
	n := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("feed: the document is not well formed: %w", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return nil, nil, fmt.Errorf("feed: the document declares a DTD or entity, which is not accepted")
		case xml.StartElement:
			if t.Name.Local != m.RecordElement {
				continue
			}
			n++
			if n > MaxRows {
				return nil, nil, ErrTooManyRows
			}
			line, _ := dec.InputPos()
			fields := map[string]string{}
			for _, a := range t.Attr {
				fields[a.Name.Local] = a.Value
			}
			if err := readChildren(dec, fields); err != nil {
				return nil, nil, fmt.Errorf("feed: the document is not well formed: %w", err)
			}
			rec, err := m.build(line, func(f string) (string, bool) {
				l, ok := m.Fields[f]
				if !ok {
					return "", false
				}
				v, ok := fields[l.Element]
				return v, ok
			})
			if err != nil {
				errs = append(errs, RowError{Line: line, Reason: fmt.Sprintf("record %d: %s", n, err.Error())})
				continue
			}
			recs = append(recs, rec)
		}
	}
	if n == 0 {
		return nil, nil, fmt.Errorf("feed: the document has no %q records", m.RecordElement)
	}
	return recs, errs, nil
}

// readChildren collects the text of the direct child elements until the record ends.
func readChildren(dec *xml.Decoder, into map[string]string) error {
	depth := 1
	var name string
	var text bytes.Buffer
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 {
				name = t.Name.Local
				text.Reset()
			}
		case xml.CharData:
			if depth == 2 {
				text.Write(t)
			}
		case xml.EndElement:
			if depth == 2 {
				into[name] = text.String()
			}
			depth--
		}
	}
	return nil
}
