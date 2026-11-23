package scimath

import (
	"math"
	"strings"
	"testing"
)

func TestConvertAndDimensionalGuards(t *testing.T) {
	// Volume
	v, ok, same := Convert(5400, "uL", "mL")
	if !ok || !same || math.Abs(v-5.4) > 1e-6 {
		t.Fatalf("expected 5400 uL -> 5.4 mL, got (%v, %v, %v)", v, ok, same)
	}
	// Mass concentration equivalents
	c, ok, same := Convert(1, "mg/mL", "ug/mL")
	if !ok || !same || math.Abs(c-1000) > 1e-6 {
		t.Fatalf("expected 1 mg/mL -> 1000 ug/mL, got (%v, %v, %v)", c, ok, same)
	}
	// Incompatible dimensions blocked
	if _, ok, same := Convert(270, "mg", "mL"); ok || same {
		t.Fatalf("expected mg -> mL to be rejected")
	}
	if _, ok, same := Convert(12, "孔", "uL"); ok || same {
		t.Fatalf("expected 孔 -> uL to be rejected")
	}
}

func TestConvertConcWithMWAndDilutionC1V1(t *testing.T) {
	// Cross-dimension without MW fails
	if _, err := ConvertConcWithMW(1, "mg/mL", "uM", 0); err == nil || !strings.Contains(err.Error(), "MW") {
		t.Fatalf("expected MW required error, got %v", err)
	}
	// 1 mg/mL with MW=500 g/mol == 2000 uM
	uM, err := ConvertConcWithMW(1, "mg/mL", "uM", 500)
	if err != nil || math.Abs(uM-2000) > 1e-4 {
		t.Fatalf("expected 2000 uM, got %v (err=%v)", uM, err)
	}
	// Full dilution C1V1=C2V2 across mass->molar via MW
	v1, vDil, err := DilutionC1V1(1, "mg/mL", 10, "uM", 10, "mL", 500)
	if err != nil || math.Abs(v1-50) > 1e-4 || math.Abs(vDil-9950) > 1e-4 {
		t.Fatalf("expected V1=50 uL, Vdil=9950 uL, got (%v, %v, err=%v)", v1, vDil, err)
	}
	// C1 <= C2 guard
	if _, _, err := DilutionC1V1(10, "ng/mL", 50, "ng/mL", 5, "mL", 0); err == nil {
		t.Fatalf("expected C1 <= C2 error")
	}
}

func TestEvalAndExtractVars(t *testing.T) {
	val, err := Eval(`\frac{C_{2} \times V_{2}}{C_{1}}`, map[string]float64{"C_1": 100, "C_2": 0.05, "V_2": 5000})
	if err != nil || math.Abs(val-2.5) > 1e-6 {
		t.Fatalf("expected 2.5, got %v (err=%v)", val, err)
	}
	vars := ExtractVars(`N \times V_{well} \times (1 + r)`)
	if strings.Join(vars, ",") != "N,V_well,r" {
		t.Fatalf("unexpected vars: %v", vars)
	}
	if _, err := Eval(`10 / 0`, nil); err == nil {
		t.Fatalf("expected division by zero error")
	}
	if _, err := Eval(`\sqrt{-4}`, nil); err == nil {
		t.Fatalf("expected negative sqrt error")
	}
}
