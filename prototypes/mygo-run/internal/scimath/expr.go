package scimath

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// MathNode is one AST node parsed from a LaTeX or plain arithmetic expression.
// It powers BOTH deterministic evaluation in scimath and 2D math rendering in asterdoc.
type MathNode interface {
	mathNodeTag()
}

type MathNum struct {
	Raw string
	Val float64
}

type MathIdent struct {
	Name    string // Canonical evaluation key, e.g. "C_1", "V_2", "pi", "mu"
	Display string // Pretty display base, e.g. "C", "V", "π", "µ"
	Sub     string // Optional subscript text, e.g. "1", "2", "stock"
	IsGreek bool
}

type MathUnary struct {
	Op   string // "-" | "+"
	Expr MathNode
}

type MathBinOp struct {
	Op    string // "+", "-", "*", "/", "times", "cdot", "div"
	Left  MathNode
	Right MathNode
}

type MathFrac struct {
	Num MathNode
	Den MathNode
}

type MathSup struct {
	Base MathNode
	Exp  MathNode
}

type MathSub struct {
	Base MathNode
	Sub  MathNode
}

type MathSqrt struct {
	Arg MathNode
}

type MathFunc struct {
	Name string   // "log", "log10", "ln", "exp", "abs", "sin", "cos"
	Sub  MathNode // optional base subscript like log_{10}
	Arg  MathNode
}

type MathParen struct {
	Inner MathNode
}

func (MathNum) mathNodeTag()   {}
func (MathIdent) mathNodeTag() {}
func (MathUnary) mathNodeTag() {}
func (MathBinOp) mathNodeTag() {}
func (MathFrac) mathNodeTag()  {}
func (MathSup) mathNodeTag()   {}
func (MathSub) mathNodeTag()   {}
func (MathSqrt) mathNodeTag()  {}
func (MathFunc) mathNodeTag()  {}
func (MathParen) mathNodeTag() {}

var latexGreekMap = map[string]string{
	"alpha":  "α",
	"beta":   "β",
	"gamma":  "γ",
	"delta":  "δ",
	"Delta":  "Δ",
	"mu":     "µ",
	"pi":     "π",
	"rho":    "ρ",
	"lambda": "λ",
	"sigma":  "σ",
	"theta":  "θ",
	"omega":  "ω",
	"Omega":  "Ω",
}

type ltxTokKind int

const (
	ltxEOF ltxTokKind = iota
	ltxNum
	ltxIdent
	ltxCmd
	ltxPlus
	ltxMinus
	ltxStar
	ltxSlash
	ltxCaret
	ltxUnder
	ltxLBrace
	ltxRBrace
	ltxLParen
	ltxRParen
)

type ltxTok struct {
	kind ltxTokKind
	text string
	num  float64
}

func tokenizeLatex(src string) ([]ltxTok, error) {
	s := strings.TrimSpace(src)
	if s == "" {
		return nil, fmt.Errorf("formula cannot be empty")
	}
	var out []ltxTok
	for i := 0; i < len(s); {
		b := s[i]
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '$' {
			i++
			continue
		}
		switch b {
		case '+':
			out = append(out, ltxTok{kind: ltxPlus, text: "+"})
			i++
		case '-':
			out = append(out, ltxTok{kind: ltxMinus, text: "-"})
			i++
		case '*':
			out = append(out, ltxTok{kind: ltxStar, text: "*"})
			i++
		case '/':
			out = append(out, ltxTok{kind: ltxSlash, text: "/"})
			i++
		case '^':
			out = append(out, ltxTok{kind: ltxCaret, text: "^"})
			i++
		case '_':
			out = append(out, ltxTok{kind: ltxUnder, text: "_"})
			i++
		case '{':
			out = append(out, ltxTok{kind: ltxLBrace, text: "{"})
			i++
		case '}':
			out = append(out, ltxTok{kind: ltxRBrace, text: "}"})
			i++
		case '(':
			out = append(out, ltxTok{kind: ltxLParen, text: "("})
			i++
		case ')':
			out = append(out, ltxTok{kind: ltxRParen, text: ")"})
			i++
		case '\\':
			i++
			if i >= len(s) {
				return nil, fmt.Errorf("未完成的 LaTeX 命令")
			}
			if s[i] == ',' || s[i] == ';' || s[i] == ':' || s[i] == '!' || s[i] == ' ' {
				i++
				continue
			}
			start := i
			for i < len(s) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z')) {
				i++
			}
			cmd := s[start:i]
			if cmd == "" {
				return nil, fmt.Errorf("无法识别的 LaTeX 符号 \\%c", s[start])
			}
			if cmd == "left" || cmd == "right" {
				continue
			}
			out = append(out, ltxTok{kind: ltxCmd, text: cmd})
		default:
			if strings.HasPrefix(s[i:], "×") {
				out = append(out, ltxTok{kind: ltxCmd, text: "times"})
				i += len("×")
				continue
			}
			if strings.HasPrefix(s[i:], "·") {
				out = append(out, ltxTok{kind: ltxCmd, text: "cdot"})
				i += len("·")
				continue
			}
			if strings.HasPrefix(s[i:], "÷") {
				out = append(out, ltxTok{kind: ltxCmd, text: "div"})
				i += len("÷")
				continue
			}
			if strings.HasPrefix(s[i:], "−") {
				out = append(out, ltxTok{kind: ltxMinus, text: "-"})
				i += len("−")
				continue
			}
			if (b >= '0' && b <= '9') || b == '.' {
				start := i
				for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
					i++
				}
				v, err := strconv.ParseFloat(s[start:i], 64)
				if err != nil {
					return nil, fmt.Errorf("数字格式无效：%s", s[start:i])
				}
				out = append(out, ltxTok{kind: ltxNum, text: s[start:i], num: v})
			} else if unicode.IsLetter(rune(b)) {
				start := i
				for i < len(s) && (unicode.IsLetter(rune(s[i])) || unicode.IsDigit(rune(s[i]))) {
					i++
				}
				out = append(out, ltxTok{kind: ltxIdent, text: s[start:i]})
			} else {
				return nil, fmt.Errorf("公式中包含无法识别的字符 %q", string(b))
			}
		}
	}
	return out, nil
}

type latexParser struct {
	toks []ltxTok
	pos  int
}

func (p *latexParser) peek() ltxTok {
	if p.pos >= len(p.toks) {
		return ltxTok{kind: ltxEOF}
	}
	return p.toks[p.pos]
}

func (p *latexParser) next() ltxTok {
	t := p.peek()
	if p.pos < len(p.toks) {
		p.pos++
	}
	return t
}

// Parse parses a LaTeX or plain arithmetic expression into a MathNode AST.
func Parse(src string) (MathNode, error) {
	toks, err := tokenizeLatex(src)
	if err != nil {
		return nil, err
	}
	p := &latexParser{toks: toks}
	node, err := p.parseAddSub()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != ltxEOF {
		return nil, fmt.Errorf("公式末尾存在多余符号 %q", p.peek().text)
	}
	return node, nil
}

// Validate checks syntax of an arithmetic or LaTeX formula expression.
func Validate(expr string) error {
	_, err := Parse(expr)
	return err
}

// Eval evaluates an arithmetic or LaTeX formula against scalar variables.
// Variable lookup matches exact key first, then case-insensitive / underscore-normalized
// keys so `C_{1}`, `C_1`, `c1`, and `C1` resolve identically.
func Eval(expr string, vars map[string]float64) (float64, error) {
	ast, err := Parse(expr)
	if err != nil {
		return 0, err
	}
	val, err := evalMathNode(ast, vars)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, fmt.Errorf("计算结果超出有效范围")
	}
	return val, nil
}

// ExtractVars returns unique variable identifiers referenced by a formula in order
// of appearance, skipping built-in constants like `pi` and `e`.
func ExtractVars(src string) []string {
	ast, err := Parse(src)
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	var walk func(MathNode)
	walk = func(n MathNode) {
		switch x := n.(type) {
		case MathIdent:
			if x.Name != "" && !strings.EqualFold(x.Name, "pi") && x.Name != "e" {
				norm := NormalizeVarLookup(x.Name)
				if !seen[norm] {
					seen[norm] = true
					out = append(out, x.Name)
				}
			}
		case MathUnary:
			walk(x.Expr)
		case MathParen:
			walk(x.Inner)
		case MathBinOp:
			walk(x.Left)
			walk(x.Right)
		case MathFrac:
			walk(x.Num)
			walk(x.Den)
		case MathSup:
			walk(x.Base)
			walk(x.Exp)
		case MathSub:
			name := FlattenMathText(x)
			if name != "" {
				norm := NormalizeVarLookup(name)
				if !seen[norm] {
					seen[norm] = true
					out = append(out, name)
				}
			}
		case MathSqrt:
			walk(x.Arg)
		case MathFunc:
			if x.Sub != nil {
				walk(x.Sub)
			}
			walk(x.Arg)
		}
	}
	walk(ast)
	return out
}

func (p *latexParser) parseAddSub() (MathNode, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind == ltxPlus || t.kind == ltxMinus {
			p.next()
			right, err := p.parseMulDiv()
			if err != nil {
				return nil, err
			}
			left = MathBinOp{Op: t.text, Left: left, Right: right}
			continue
		}
		break
	}
	return left, nil
}

func (p *latexParser) isImplicitMulStart(t ltxTok) bool {
	switch t.kind {
	case ltxIdent, ltxLParen:
		return true
	case ltxCmd:
		switch t.text {
		case "frac", "sqrt", "log", "ln", "exp", "sin", "cos", "abs", "text", "mathrm":
			return true
		default:
			_, isGreek := latexGreekMap[t.text]
			return isGreek
		}
	}
	return false
}

func (p *latexParser) parseMulDiv() (MathNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind == ltxStar || t.kind == ltxSlash {
			p.next()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = MathBinOp{Op: t.text, Left: left, Right: right}
			continue
		}
		if t.kind == ltxCmd && (t.text == "times" || t.text == "cdot" || t.text == "div") {
			p.next()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = MathBinOp{Op: t.text, Left: left, Right: right}
			continue
		}
		if p.isImplicitMulStart(t) {
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = MathBinOp{Op: "cdot", Left: left, Right: right}
			continue
		}
		break
	}
	return left, nil
}

func (p *latexParser) parseUnary() (MathNode, error) {
	t := p.peek()
	if t.kind == ltxPlus || t.kind == ltxMinus {
		p.next()
		sub, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return MathUnary{Op: t.text, Expr: sub}, nil
	}
	return p.parsePostfix()
}

func (p *latexParser) parsePostfix() (MathNode, error) {
	node, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind == ltxUnder {
			p.next()
			subNode, err := p.parseScriptArg()
			if err != nil {
				return nil, err
			}
			if id, ok := node.(MathIdent); ok {
				subStr := FlattenMathText(subNode)
				if subStr != "" {
					fullName := id.Name + "_" + subStr
					displaySub := subStr
					if id.Sub != "" {
						displaySub = id.Sub + "_" + subStr
					}
					node = MathIdent{
						Name:    fullName,
						Display: id.Display,
						Sub:     displaySub,
						IsGreek: id.IsGreek,
					}
					continue
				}
			}
			node = MathSub{Base: node, Sub: subNode}
			continue
		}
		if t.kind == ltxCaret {
			p.next()
			expNode, err := p.parseScriptArg()
			if err != nil {
				return nil, err
			}
			node = MathSup{Base: node, Exp: expNode}
			continue
		}
		break
	}
	return node, nil
}

func (p *latexParser) parseScriptArg() (MathNode, error) {
	if p.peek().kind == ltxLBrace {
		return p.parseBracedGroup()
	}
	return p.parsePrimary()
}

func (p *latexParser) parseBracedGroup() (MathNode, error) {
	if p.peek().kind != ltxLBrace {
		return nil, fmt.Errorf("缺少左花括号 '{'")
	}
	p.next()
	if p.peek().kind == ltxRBrace {
		p.next()
		return MathIdent{Name: "", Display: "□"}, nil
	}
	inner, err := p.parseAddSub()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != ltxRBrace {
		return nil, fmt.Errorf("缺少右花括号 '}'")
	}
	p.next()
	return inner, nil
}

func (p *latexParser) parsePrimary() (MathNode, error) {
	t := p.peek()
	switch t.kind {
	case ltxNum:
		p.next()
		return MathNum{Raw: t.text, Val: t.num}, nil
	case ltxIdent:
		p.next()
		return MakeIdentNode(t.text), nil
	case ltxLParen:
		p.next()
		inner, err := p.parseAddSub()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != ltxRParen {
			return nil, fmt.Errorf("缺少右括号 ')'")
		}
		p.next()
		return MathParen{Inner: inner}, nil
	case ltxLBrace:
		return p.parseBracedGroup()
	case ltxCmd:
		p.next()
		switch t.text {
		case "frac", "dfrac", "tfrac":
			num, err := p.parseBracedGroup()
			if err != nil {
				return nil, fmt.Errorf("分数的分子不完整：%w", err)
			}
			den, err := p.parseBracedGroup()
			if err != nil {
				return nil, fmt.Errorf("分数的分母不完整：%w", err)
			}
			return MathFrac{Num: num, Den: den}, nil
		case "sqrt":
			arg, err := p.parseScriptArg()
			if err != nil {
				return nil, fmt.Errorf("根号内容不完整：%w", err)
			}
			return MathSqrt{Arg: arg}, nil
		case "text", "mathrm":
			arg, err := p.parseBracedGroup()
			if err != nil {
				return nil, err
			}
			txt := FlattenMathText(arg)
			return MathIdent{Name: txt, Display: txt}, nil
		case "log", "ln", "exp", "abs", "sin", "cos":
			var sub MathNode
			if t.text == "log" && p.peek().kind == ltxUnder {
				p.next()
				var err error
				sub, err = p.parseScriptArg()
				if err != nil {
					return nil, err
				}
			}
			arg, err := p.parseUnary()
			if err != nil {
				return nil, fmt.Errorf("\\%s 缺少参数：%w", t.text, err)
			}
			return MathFunc{Name: t.text, Sub: sub, Arg: arg}, nil
		default:
			if sym, ok := latexGreekMap[t.text]; ok {
				return MathIdent{Name: t.text, Display: sym, IsGreek: true}, nil
			}
			return nil, fmt.Errorf("暂不支持的 LaTeX 命令 \\%s", t.text)
		}
	default:
		return nil, fmt.Errorf("公式未完整")
	}
}

// MakeIdentNode splits short trailing digits or `_` subscripts into a pretty MathIdent.
func MakeIdentNode(raw string) MathIdent {
	if idx := strings.IndexByte(raw, '_'); idx > 0 && idx < len(raw)-1 {
		return MathIdent{Name: raw, Display: raw[:idx], Sub: raw[idx+1:]}
	}
	if len(raw) >= 2 && len(raw) <= 3 {
		last := raw[len(raw)-1]
		first := raw[0]
		if unicode.IsLetter(rune(first)) && last >= '0' && last <= '9' {
			splitAt := len(raw) - 1
			for splitAt > 1 && raw[splitAt-1] >= '0' && raw[splitAt-1] <= '9' {
				splitAt--
			}
			return MathIdent{Name: raw, Display: raw[:splitAt], Sub: raw[splitAt:]}
		}
	}
	return MathIdent{Name: raw, Display: raw}
}

// FlattenMathText flattens a subscript/text AST node into a canonical identifier string.
func FlattenMathText(n MathNode) string {
	switch x := n.(type) {
	case MathNum:
		return x.Raw
	case MathIdent:
		return x.Name
	case MathSub:
		return FlattenMathText(x.Base) + "_" + FlattenMathText(x.Sub)
	default:
		return ""
	}
}

func lookupVar(vars map[string]float64, name string) (float64, bool) {
	if name == "" {
		return 0, false
	}
	if v, ok := vars[name]; ok {
		return v, true
	}
	norm := NormalizeVarLookup(name)
	for k, v := range vars {
		if NormalizeVarLookup(k) == norm {
			return v, true
		}
	}
	if strings.EqualFold(name, "pi") {
		return math.Pi, true
	}
	if name == "e" {
		return math.E, true
	}
	return 0, false
}

// NormalizeVarLookup normalizes formula variable keys (`C_{1}`, `C_1`, `c1`) for case/underscore-insensitive lookup.
func NormalizeVarLookup(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "{", "")
	s = strings.ReplaceAll(s, "}", "")
	return s
}

func evalMathNode(n MathNode, vars map[string]float64) (float64, error) {
	switch x := n.(type) {
	case MathNum:
		return x.Val, nil
	case MathIdent:
		if x.Name == "" {
			return 0, fmt.Errorf("公式中存在空白占位符")
		}
		if v, ok := lookupVar(vars, x.Name); ok {
			return v, nil
		}
		return 0, fmt.Errorf("missing variable %q", x.Name)
	case MathUnary:
		v, err := evalMathNode(x.Expr, vars)
		if err != nil {
			return 0, err
		}
		if x.Op == "-" {
			return -v, nil
		}
		return v, nil
	case MathParen:
		return evalMathNode(x.Inner, vars)
	case MathBinOp:
		l, err := evalMathNode(x.Left, vars)
		if err != nil {
			return 0, err
		}
		r, err := evalMathNode(x.Right, vars)
		if err != nil {
			return 0, err
		}
		switch x.Op {
		case "+":
			return l + r, nil
		case "-":
			return l - r, nil
		case "*", "times", "cdot":
			return l * r, nil
		case "/", "div":
			if r == 0 {
				return 0, fmt.Errorf("除数不能为 0")
			}
			return l / r, nil
		default:
			return 0, fmt.Errorf("未知运算符 %q", x.Op)
		}
	case MathFrac:
		num, err := evalMathNode(x.Num, vars)
		if err != nil {
			return 0, err
		}
		den, err := evalMathNode(x.Den, vars)
		if err != nil {
			return 0, err
		}
		if den == 0 {
			return 0, fmt.Errorf("分母不能为 0")
		}
		return num / den, nil
	case MathSup:
		b, err := evalMathNode(x.Base, vars)
		if err != nil {
			return 0, err
		}
		e, err := evalMathNode(x.Exp, vars)
		if err != nil {
			return 0, err
		}
		return math.Pow(b, e), nil
	case MathSub:
		name := FlattenMathText(x)
		if v, ok := lookupVar(vars, name); ok {
			return v, nil
		}
		return 0, fmt.Errorf("missing variable %q", name)
	case MathSqrt:
		v, err := evalMathNode(x.Arg, vars)
		if err != nil {
			return 0, err
		}
		if v < 0 {
			return 0, fmt.Errorf("负数不能开平方根")
		}
		return math.Sqrt(v), nil
	case MathFunc:
		v, err := evalMathNode(x.Arg, vars)
		if err != nil {
			return 0, err
		}
		switch x.Name {
		case "log":
			if v <= 0 {
				return 0, fmt.Errorf("对数输入必须大于 0")
			}
			if x.Sub != nil {
				base, err := evalMathNode(x.Sub, vars)
				if err != nil || base <= 0 || base == 1 {
					return 0, fmt.Errorf("对数底数无效")
				}
				return math.Log(v) / math.Log(base), nil
			}
			return math.Log10(v), nil
		case "ln":
			if v <= 0 {
				return 0, fmt.Errorf("自然对数输入必须大于 0")
			}
			return math.Log(v), nil
		case "exp":
			return math.Exp(v), nil
		case "abs":
			return math.Abs(v), nil
		case "sin":
			return math.Sin(v), nil
		case "cos":
			return math.Cos(v), nil
		default:
			return 0, fmt.Errorf("未知函数 %s", x.Name)
		}
	default:
		return 0, fmt.Errorf("无效表达式")
	}
}
