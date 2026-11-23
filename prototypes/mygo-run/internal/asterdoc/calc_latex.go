package asterdoc

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/scimath"
	"asterlab-mygo-prototype/internal/theme"
)

// MathNode and concrete AST nodes are aliased directly from internal/scimath so
// parsing/evaluation live in the standalone math engine while this file focuses
// purely on native 2D mathematical rendering in MyGo.
type (
	MathNode  = scimath.MathNode
	MathNum   = scimath.MathNum
	MathIdent = scimath.MathIdent
	MathUnary = scimath.MathUnary
	MathBinOp = scimath.MathBinOp
	MathFrac  = scimath.MathFrac
	MathSup   = scimath.MathSup
	MathSub   = scimath.MathSub
	MathSqrt  = scimath.MathSqrt
	MathFunc  = scimath.MathFunc
	MathParen = scimath.MathParen
)

// ParseLatexMath parses a LaTeX or arithmetic expression into a MathNode AST.
func ParseLatexMath(src string) (MathNode, error) {
	return scimath.Parse(src)
}

// EvalLatexMath evaluates a LaTeX or plain arithmetic expression against vars.
func EvalLatexMath(src string, vars map[string]float64) (float64, error) {
	return scimath.Eval(src, vars)
}

// ExtractLatexVars returns unique variable identifiers referenced by a formula.
func ExtractLatexVars(src string) []string {
	return scimath.ExtractVars(src)
}

func makeIdentNode(raw string) MathIdent { return scimath.MakeIdentNode(raw) }

func normalizeVarLookup(s string) string { return scimath.NormalizeVarLookup(s) }

// FormatVarSymbolPretty turns keys like `c1` or `C_2` into human math chips like `C₁` / `C₂`.
func FormatVarSymbolPretty(key string) string {
	id := scimath.MakeIdentNode(strings.TrimSpace(key))
	if id.Sub == "" {
		return id.Display
	}
	return id.Display + toUnicodeSubscript(id.Sub)
}

// FormatVarLatexInsert turns a variable key like `c1` or `C_2` into clean LaTeX (`C_{1}` / `V_{2}`).
func FormatVarLatexInsert(key string) string {
	key = strings.TrimSpace(key)
	id := scimath.MakeIdentNode(key)
	if id.Sub != "" {
		return fmt.Sprintf("%s_{%s}", id.Display, id.Sub)
	}
	return key
}

func toUnicodeSubscript(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '0':
			b.WriteRune('₀')
		case '1':
			b.WriteRune('₁')
		case '2':
			b.WriteRune('₂')
		case '3':
			b.WriteRune('₃')
		case '4':
			b.WriteRune('₄')
		case '5':
			b.WriteRune('₅')
		case '6':
			b.WriteRune('₆')
		case '7':
			b.WriteRune('₇')
		case '8':
			b.WriteRune('₈')
		case '9':
			b.WriteRune('₉')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RenderMathNode draws a 2D mathematical expression natively inside MyGo using recursive flex boxes.
func RenderMathNode(c *ui.Context, tok theme.Tokens, node MathNode, baseSize float32, textColor ui.Color) {
	if node == nil {
		return
	}
	if baseSize < 11 {
		baseSize = 11
	}
	switch x := node.(type) {
	case MathNum:
		ui.Text(c, x.Raw).FontSize(baseSize).TextColor(textColor).FontFeatures("tnum").SingleLine()
	case MathIdent:
		if x.Sub != "" {
			ui.Row(c).AlignItems(ui.End).Gap(1).Children(func() {
				ui.Text(c, x.Display).FontSize(baseSize).Italic().Bold().TextColor(tok.Accent).SingleLine()
				ui.Box(c).Padding(0, 0, 0, 0).Children(func() {
					ui.Text(c, x.Sub).FontSize(maxFloat32(10, baseSize*0.66)).Bold().TextColor(tok.Accent.Alpha(0.88)).SingleLine()
				})
			})
		} else {
			col := tok.Accent
			if x.Display == "□" || strings.EqualFold(x.Name, "pi") {
				col = textColor
			}
			txt := ui.Text(c, x.Display).FontSize(baseSize).Bold().TextColor(col).SingleLine()
			if !x.IsGreek && x.Display != "□" {
				txt.Italic()
			}
		}
	case MathUnary:
		ui.Row(c).AlignItems(ui.Center).Gap(1).Children(func() {
			op := x.Op
			if op == "-" {
				op = "−"
			}
			ui.Text(c, op).FontSize(baseSize).TextColor(textColor).SingleLine()
			RenderMathNode(c, tok, x.Expr, baseSize, textColor)
		})
	case MathParen:
		ui.Row(c).AlignItems(ui.Center).Gap(2).Children(func() {
			ui.Text(c, "(").FontSize(baseSize * 1.08).TextColor(tok.TextMuted).SingleLine()
			RenderMathNode(c, tok, x.Inner, baseSize, textColor)
			ui.Text(c, ")").FontSize(baseSize * 1.08).TextColor(tok.TextMuted).SingleLine()
		})
	case MathBinOp:
		opSym := x.Op
		switch x.Op {
		case "*":
			opSym = "×"
		case "times":
			opSym = "×"
		case "cdot":
			opSym = "·"
		case "/", "div":
			opSym = "÷"
		case "-":
			opSym = "−"
		}
		ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4) * 1.2).Children(func() {
			RenderMathNode(c, tok, x.Left, baseSize, textColor)
			ui.Text(c, opSym).FontSize(baseSize).TextColor(tok.TextMuted).SingleLine()
			RenderMathNode(c, tok, x.Right, baseSize, textColor)
		})
	case MathFrac:
		subSz := maxFloat32(11, baseSize*0.86)
		ui.Column(c).AlignItems(ui.Center).Gap(2).Padding(0, float32(tok.Spacing4)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Padding(0, float32(tok.Spacing4)).Children(func() {
				RenderMathNode(c, tok, x.Num, subSz, textColor)
			})
			ui.Box(c).FillWidth().Height(1.5).Background(textColor.Alpha(0.75))
			ui.Row(c).AlignItems(ui.Center).Padding(0, float32(tok.Spacing4)).Children(func() {
				RenderMathNode(c, tok, x.Den, subSz, textColor)
			})
		})
	case MathSup:
		ui.Row(c).AlignItems(ui.Start).Gap(1).Children(func() {
			ui.Box(c).Padding(baseSize*0.20, 0, 0, 0).Children(func() {
				RenderMathNode(c, tok, x.Base, baseSize, textColor)
			})
			RenderMathNode(c, tok, x.Exp, maxFloat32(10, baseSize*0.66), textColor)
		})
	case MathSub:
		ui.Row(c).AlignItems(ui.End).Gap(1).Children(func() {
			RenderMathNode(c, tok, x.Base, baseSize, textColor)
			RenderMathNode(c, tok, x.Sub, maxFloat32(10, baseSize*0.66), textColor)
		})
	case MathSqrt:
		ui.Row(c).AlignItems(ui.Center).Gap(1).Children(func() {
			ui.Text(c, "√").FontSize(baseSize * 1.15).TextColor(textColor).SingleLine()
			ui.Column(c).AlignItems(ui.Start).Gap(1).Children(func() {
				ui.Box(c).FillWidth().Height(1.25).Background(textColor.Alpha(0.75))
				ui.Row(c).Padding(0, float32(tok.Spacing4)*0.5).AlignItems(ui.Center).Children(func() {
					RenderMathNode(c, tok, x.Arg, baseSize*0.95, textColor)
				})
			})
		})
	case MathFunc:
		ui.Row(c).AlignItems(ui.Center).Gap(2).Children(func() {
			if x.Sub != nil {
				ui.Row(c).AlignItems(ui.End).Gap(1).Children(func() {
					ui.Text(c, x.Name).FontSize(baseSize).TextColor(textColor).SingleLine()
					RenderMathNode(c, tok, x.Sub, maxFloat32(10, baseSize*0.64), tok.TextMuted)
				})
			} else {
				ui.Text(c, x.Name).FontSize(baseSize).TextColor(textColor).SingleLine()
			}
			RenderMathNode(c, tok, x.Arg, baseSize, textColor)
		})
	}
}

func maxFloat32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
