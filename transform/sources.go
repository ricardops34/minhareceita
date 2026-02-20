package transform

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type sourceKind uint8

const (
	CompanySrc sourceKind = iota
	TaxSrc
	IBGESrc
)

type source struct {
	prefix       string
	filePrefix   string // empty means loaded from URL
	key          string
	sep          rune
	hasHeader    bool
	isCumulative bool
	kind         sourceKind
	counter      atomic.Uint32
}

func (s *source) keyFor(id string) []byte {
	if !s.isCumulative {
		return fmt.Appendf([]byte{}, "%s::%s", id, s.key)
	}
	c := s.counter.Add(1)
	return fmt.Appendf([]byte{}, "%s::%s::%d", id, s.key, c)
}

func (s *source) keyPrefixFor(id string) []byte {
	if !s.isCumulative {
		return s.keyFor(id)
	}
	return fmt.Appendf([]byte{}, "%s::%s", id, s.key)
}

func newCompanySrc(prefix string, sep rune, hasHeader, isCumulative bool) *source {
	key := strings.ToLower(strings.TrimPrefix(prefix, "Lucro ")[0:3])
	return &source{
		prefix:       prefix,
		key:          key,
		sep:          sep,
		hasHeader:    hasHeader,
		isCumulative: isCumulative,
		kind:         CompanySrc,
	}
}

func newTaxSrc(prefix, filePrefix string, sep rune, hasHeader, isCumulative bool) *source {
	key := strings.ToLower(strings.TrimPrefix(prefix, "Lucro ")[0:3])
	return &source{
		prefix:       prefix,
		filePrefix:   filePrefix,
		key:          key,
		sep:          sep,
		hasHeader:    hasHeader,
		isCumulative: isCumulative,
		kind:         TaxSrc,
	}
}

func newIBGESrc(prefix string, sep rune, hasHeader, isCumulative bool) *source {
	key := strings.ToLower(strings.TrimPrefix(prefix, "Lucro ")[0:3])
	return &source{
		prefix:       prefix,
		key:          key,
		sep:          sep,
		hasHeader:    hasHeader,
		isCumulative: isCumulative,
		kind:         IBGESrc,
	}
}
