package zone

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Record はゾーンファイルの1レコードを表す。
type Record struct {
	Name  string
	TTL   uint32
	Class string
	Type  string
	RData string
}

// Parse はゾーンファイルのバイト列をレコード群にパースする。
// 形式: NAME TTL CLASS TYPE RDATA
// コメント行 (; で始まる) と空行はスキップする。
// 行頭が空白/タブの場合、前のレコードの名前を継承する。
func Parse(data []byte) ([]Record, error) {
	var records []Record
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	lineNum := 0
	var lastName string

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			continue
		}

		r, err := parseLine(line, lastName)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		lastName = r.Name
		records = append(records, r)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan zone data: %w", err)
	}

	return records, nil
}

func parseLine(line string, lastName string) (Record, error) {
	nameInherited := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')

	fields := strings.Fields(line)
	if len(fields) < 4 {
		return Record{}, fmt.Errorf("too few fields: %q", line)
	}

	var name string
	var rest []string

	if nameInherited {
		name = lastName
		rest = fields
	} else {
		name = fields[0]
		rest = fields[1:]
	}

	if len(rest) < 4 {
		return Record{}, fmt.Errorf("too few fields after name: %q", line)
	}

	ttl, err := strconv.ParseUint(rest[0], 10, 32)
	if err != nil {
		return Record{}, fmt.Errorf("invalid TTL %q: %w", rest[0], err)
	}

	return Record{
		Name:  name,
		TTL:   uint32(ttl),
		Class: rest[1],
		Type:  rest[2],
		RData: strings.Join(rest[3:], " "),
	}, nil
}
