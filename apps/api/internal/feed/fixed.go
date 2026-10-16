package feed

import (
	"bytes"
	"fmt"
)

// parseFixed reads one record per line. Every line must be exactly RecordLen bytes
// (a trailing carriage return is ignored), because a line of another length means
// the columns have shifted and any field read from it could be wrong.
func (m *Mapping) parseFixed(data []byte) ([]Record, []RowError, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil, fmt.Errorf("feed: the file is empty")
	}
	lines := bytes.Split(data, []byte("\n"))
	var recs []Record
	var errs []RowError
	for i, raw := range lines {
		n := i + 1
		raw = bytes.TrimSuffix(raw, []byte("\r"))
		if i == 0 && m.SkipHeader {
			continue
		}
		if len(raw) == 0 && i == len(lines)-1 {
			continue // the newline that ends the file
		}
		if n > MaxRows+1 {
			return nil, nil, ErrTooManyRows
		}
		if len(raw) != m.RecordLen {
			errs = append(errs, RowError{Line: n, Reason: fmt.Sprintf("the line is %d bytes and the record is %d", len(raw), m.RecordLen)})
			continue
		}
		rec, err := m.build(n, func(f string) (string, bool) {
			l, ok := m.Fields[f]
			if !ok {
				return "", false
			}
			return string(raw[l.Start : l.Start+l.Len]), true
		})
		if err != nil {
			errs = append(errs, RowError{Line: n, Reason: err.Error()})
			continue
		}
		recs = append(recs, rec)
	}
	return recs, errs, nil
}
