package asterdoc

import (
	"math"
	"testing"
)

func TestParseAndEvalLatexMath(t *testing.T) {
	vars := map[string]float64{
		"c1":  10,
		"c2":  0.5,
		"v2":  1000,
		"x":   3,
		"y":   4,
		"mw":  543.5,
		"vol": 5,
	}

	tests := []struct {
		expr string
		want float64
	}{
		{`\frac{C_{2} \times V_{2}}{C_{1}}`, 50},
		{`\sqrt{x^{2} + y^{2}}`, 5},
		{`\log_{10}(1000)`, 3},
		{`2 \times 10^{3}`, 2000},
		{`c2 * v2 / c1`, 50},
		{`\frac{C_{2} \cdot V_{2}}{C_{1}} + 10`, 60},
		{`\ln(\exp(2))`, 2},
	}

	for _, tc := range tests {
		got, err := EvalLatexMath(tc.expr, vars)
		if err != nil {
			t.Fatalf("EvalLatexMath(%q) error: %v", tc.expr, err)
		}
		if math.Abs(got-tc.want) > 1e-6 {
			t.Fatalf("EvalLatexMath(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestExtractLatexVarsAndPrettyFormat(t *testing.T) {
	got := ExtractLatexVars(`\frac{C_{2} \times V_{2}}{C_{1}} + \pi \cdot k`)
	want := []string{"C_2", "V_2", "C_1", "k"}
	if len(got) != len(want) {
		t.Fatalf("ExtractLatexVars = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("var[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if p := FormatVarSymbolPretty("C_2"); p != "C₂" {
		t.Fatalf("FormatVarSymbolPretty(C_2) = %q, want C₂", p)
	}
	if ins := FormatVarLatexInsert("c1"); ins != "c_{1}" {
		t.Fatalf("FormatVarLatexInsert(c1) = %q, want c_{1}", ins)
	}
}
