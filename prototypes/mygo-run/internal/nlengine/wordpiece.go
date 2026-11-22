package nlengine

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// WordPieceTokenizer is a pure-Go BERT/BGE WordPiece tokenizer compatible with HuggingFace
// BertTokenizer (Chinese CJK character splitting + ASCII lowercasing + accent stripping + ## subwords).
type WordPieceTokenizer struct {
	vocab       map[string]int64
	clsID       int64
	sepID       int64
	padID       int64
	unkID       int64
	doLowerCase bool
	maxWordLen  int
}

// LoadWordPieceVocab loads a standard line-delimited HuggingFace vocab.txt file.
func LoadWordPieceVocab(path string, doLowerCase bool) (*WordPieceTokenizer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vocab := make(map[string]int64, 24000)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var id int64
	for sc.Scan() {
		tok := strings.TrimRight(sc.Text(), "\r\n")
		vocab[tok] = id
		id++
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(vocab) == 0 {
		return nil, fmt.Errorf("empty vocab file: %s", path)
	}
	lookup := func(tok string, fallback int64) int64 {
		if v, ok := vocab[tok]; ok {
			return v
		}
		return fallback
	}
	return &WordPieceTokenizer{
		vocab:       vocab,
		clsID:       lookup("[CLS]", 101),
		sepID:       lookup("[SEP]", 102),
		padID:       lookup("[PAD]", 0),
		unkID:       lookup("[UNK]", 100),
		doLowerCase: doLowerCase,
		maxWordLen:  100,
	}, nil
}

// Encode converts text into BERT/BGE input_ids and attention_mask truncated to maxLen (including [CLS]/[SEP]).
func (t *WordPieceTokenizer) Encode(text string, maxLen int) (inputIDs []int64, attentionMask []int64) {
	if maxLen < 4 {
		maxLen = 4
	}
	basic := t.basicTokenize(text)
	ids := make([]int64, 0, min(maxLen, len(basic)+4))
	ids = append(ids, t.clsID)

	for _, word := range basic {
		subIDs := t.wordPiece(word)
		for _, sid := range subIDs {
			if len(ids) >= maxLen-1 {
				break
			}
			ids = append(ids, sid)
		}
		if len(ids) >= maxLen-1 {
			break
		}
	}
	ids = append(ids, t.sepID)

	mask := make([]int64, len(ids))
	for i := range mask {
		mask[i] = 1
	}
	return ids, mask
}

func (t *WordPieceTokenizer) basicTokenize(text string) []string {
	// 1. Clean control characters & pad CJK characters with spaces
	var b strings.Builder
	b.Grow(len(text) * 2)
	for _, r := range text {
		if r == 0 || r == 0xfffd || isBertControl(r) {
			continue
		}
		if isBertWhitespace(r) {
			b.WriteByte(' ')
			continue
		}
		if isChineseChar(r) {
			b.WriteByte(' ')
			b.WriteRune(r)
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(r)
	}

	rawFields := strings.Fields(b.String())
	out := make([]string, 0, len(rawFields)*2)
	for _, tok := range rawFields {
		if t.doLowerCase {
			tok = stripAccents(strings.ToLower(tok))
		}
		out = append(out, splitOnPunctuation(tok)...)
	}
	return out
}

func (t *WordPieceTokenizer) wordPiece(word string) []int64 {
	runes := []rune(word)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) > t.maxWordLen {
		return []int64{t.unkID}
	}
	if id, ok := t.vocab[word]; ok {
		return []int64{id}
	}
	var out []int64
	start := 0
	for start < len(runes) {
		end := len(runes)
		found := false
		var curID int64
		for start < end {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if id, ok := t.vocab[sub]; ok {
				curID = id
				found = true
				break
			}
			end--
		}
		if !found {
			return []int64{t.unkID}
		}
		out = append(out, curID)
		start = end
	}
	return out
}

func stripAccents(s string) string {
	// Fast path for pure ASCII
	allASCII := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			allASCII = false
			break
		}
	}
	if allASCII {
		return s
	}
	decomposed := norm.NFD.String(s)
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func splitOnPunctuation(s string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		if isBertPunctuation(r) {
			flush()
			out = append(out, string(r))
		} else {
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func isBertWhitespace(r rune) bool {
	if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

func isBertControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)
}

func isBertPunctuation(r rune) bool {
	if (r >= 33 && r <= 47) || (r >= 58 && r <= 64) || (r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
		return true
	}
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func isChineseChar(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x20000 && r <= 0x2A6DF) ||
		(r >= 0x2A700 && r <= 0x2B73F) ||
		(r >= 0x2B740 && r <= 0x2B81F) ||
		(r >= 0x2B820 && r <= 0x2CEAF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0x2F800 && r <= 0x2FA1F)
}
