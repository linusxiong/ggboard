package main

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// LogEntry counts one WARN/ERROR message of the server log.
type LogEntry struct {
	Level   string `json:"level"`
	Message string `json:"msg"`
	Count   int    `json:"count"`
	Dropped int64  `json:"dropped,omitempty"` // sum of dropped=N attributes (subscription access log)
}

var (
	logLine    = regexp.MustCompile(`level=(WARN|ERROR) msg=("(?:[^"\\]|\\.)*"|\S+)`)
	droppedArg = regexp.MustCompile(`\bdropped=(\d+)`)
)

// summarizeLog counts the WARN and ERROR messages of a text (logfmt) server log, most frequent first.
func summarizeLog(r io.Reader) ([]LogEntry, error) {
	byKey := map[string]*LogEntry{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		m := logLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		msg := m[2]
		if u, err := strconv.Unquote(msg); err == nil {
			msg = u
		}
		k := m[1] + " " + msg
		e := byKey[k]
		if e == nil {
			e = &LogEntry{Level: m[1], Message: msg}
			byKey[k] = e
		}
		e.Count++
		if d := droppedArg.FindStringSubmatch(sc.Text()); d != nil {
			n, _ := strconv.ParseInt(d[1], 10, 64)
			e.Dropped += n
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]LogEntry, 0, len(byKey))
	for _, e := range byKey {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.Compare(out[i].Message, out[j].Message) < 0
	})
	return out, nil
}

func summarizeLogFile(path string) ([]LogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return summarizeLog(f)
}
