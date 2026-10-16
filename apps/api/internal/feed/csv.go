package feed

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
)

// Parse reads a file using the mapping. Rows that cannot be read are returned as
// RowErrors and do not stop the others. A file that cannot be read at all, or whose
// header lacks a mapped column, returns an error and no records.
func (m *Mapping) Parse(data []byte) ([]Record, []RowError, error) {
	if err := m.Validate(); err != nil {
		return nil, nil, err
	}
	switch m.Format {
	case CSV:
		return m.parseCSV(data)
	case FixedWidth:
		return m.parseFixed(data)
	case XML:
		return m.parseXML(data)
	}
	return nil, nil, fmt.Errorf("%w: format %s has no parser", ErrMapping, m.Format)
}

func (m *Mapping) parseCSV(data []byte) ([]Record, []RowError, error) {
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	r.FieldsPerRecord = -1
	r.LazyQuotes = false
	if m.Delimiter != "" {
		r.Comma = []rune(m.Delimiter)[0]
	}
	header, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("feed: cannot read the header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	for f, l := range m.Fields {
		if _, ok := col[l.Column]; !ok {
			return nil, nil, fmt.Errorf("feed: the file has no column %q for %s", l.Column, f)
		}
	}
	var recs []Record
	var errs []RowError
	line := 1
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err == nil {
			line, _ = r.FieldPos(0)
		}
		if line > MaxRows+1 {
			return nil, nil, ErrTooManyRows
		}
		if err != nil {
			errs = append(errs, RowError{Line: line, Reason: err.Error()})
			continue
		}
		if len(row) != len(header) {
			errs = append(errs, RowError{Line: line, Reason: fmt.Sprintf("the row has %d columns and the header has %d", len(row), len(header))})
			continue
		}
		rec, err := m.build(line, func(f string) (string, bool) {
			l, ok := m.Fields[f]
			if !ok {
				return "", false
			}
			return row[col[l.Column]], true
		})
		if err != nil {
			errs = append(errs, RowError{Line: line, Reason: err.Error()})
			continue
		}
		recs = append(recs, rec)
	}
	return recs, errs, nil
}
