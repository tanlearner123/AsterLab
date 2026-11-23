// Package scimath is AsterLab's standalone, zero-UI scientific unit conversion
// and deterministic formula evaluation engine.
//
// All physical dimensional analysis, cross-unit conversion, MW-bridged
// mass-concentration <-> molarity conversion, and LaTeX/arithmetic evaluation
// live here as a single pure-Go source of truth consumed by asterdoc, astercore,
// and cmd/asterlab.
package scimath

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Dimension identifies a canonical SI-compatible physical dimension family.
type Dimension string

const (
	DimUnknown   Dimension = ""
	DimVolume    Dimension = "volume"    // base: uL
	DimMass      Dimension = "mass"      // base: ug
	DimMassConc  Dimension = "mass_conc" // base: ng/mL
	DimMolarConc Dimension = "molar"     // base: nM
	DimTime      Dimension = "time"      // base: s
)

// NormalizeUnit standardizes micro-sign variants (µ, μ -> u) and trims whitespace.
func NormalizeUnit(unit string) string {
	u := strings.TrimSpace(unit)
	u = strings.ReplaceAll(u, "µ", "u")
	u = strings.ReplaceAll(u, "μ", "u")
	return u
}

// DimensionOf classifies a scientific unit into its physical dimension family and
// linear scale factor relative to that family's canonical base unit.
func DimensionOf(unit string) (dim Dimension, scale float64, known bool) {
	switch NormalizeUnit(unit) {
	// Volume (base: uL)
	case "nL", "nl":
		return DimVolume, 1e-3, true
	case "uL", "ul":
		return DimVolume, 1, true
	case "mL", "ml":
		return DimVolume, 1e3, true
	case "L", "l":
		return DimVolume, 1e6, true

	// Mass (base: ug)
	case "pg":
		return DimMass, 1e-6, true
	case "ng":
		return DimMass, 1e-3, true
	case "ug":
		return DimMass, 1, true
	case "mg":
		return DimMass, 1e3, true
	case "g":
		return DimMass, 1e6, true
	case "kg":
		return DimMass, 1e9, true

	// Mass concentration (base: ng/mL)
	case "pg/mL", "pg/ml":
		return DimMassConc, 1e-3, true
	case "ng/mL", "ng/ml", "pg/uL", "pg/ul":
		return DimMassConc, 1, true
	case "ug/mL", "ug/ml", "ng/uL", "ng/ul":
		return DimMassConc, 1e3, true
	case "mg/mL", "mg/ml", "ug/uL", "ug/ul", "g/L", "g/l":
		return DimMassConc, 1e6, true

	// Molar concentration (base: nM)
	case "pM":
		return DimMolarConc, 1e-3, true
	case "nM":
		return DimMolarConc, 1, true
	case "uM":
		return DimMolarConc, 1e3, true
	case "mM":
		return DimMolarConc, 1e6, true
	case "M":
		return DimMolarConc, 1e9, true

	// Time (base: s)
	case "s", "sec":
		return DimTime, 1, true
	case "min":
		return DimTime, 60, true
	case "h", "hr", "hrs":
		return DimTime, 3600, true

	default:
		return DimUnknown, 1, false
	}
}

// Convert converts val from fromUnit to toUnit when both belong to the same
// physical dimension family.
//   - Empty or identical units -> (val, true, true)
//   - Same SI dimension -> (val * sFrom / sTo, true, true)
//   - Incompatible dimensions (e.g. mg -> mL, or 孔 -> uL) -> (0, false, false)
func Convert(val float64, fromUnit, toUnit string) (out float64, ok bool, sameDim bool) {
	fu := NormalizeUnit(fromUnit)
	tu := NormalizeUnit(toUnit)
	if fu == "" || tu == "" || strings.EqualFold(fu, tu) {
		return val, true, true
	}
	d1, s1, k1 := DimensionOf(fu)
	d2, s2, k2 := DimensionOf(tu)
	if k1 && k2 {
		if d1 != d2 {
			return 0, false, false
		}
		return val * s1 / s2, true, true
	}
	if k1 != k2 && (fu == "孔" || tu == "孔" || fu == "rpm" || tu == "rpm" || fu == "×g" || tu == "×g") {
		return 0, false, false
	}
	return val, true, true
}

// ConcFactorToBase returns the base-unit scale and dimension family ("mass_conc" or "molar")
// for concentration units.
func ConcFactorToBase(unit string) (scale float64, family string) {
	d, s, ok := DimensionOf(unit)
	if !ok {
		return 0, ""
	}
	if d == DimMassConc || d == DimMolarConc {
		return s, string(d)
	}
	return 0, ""
}

// ConvertConcWithMW converts a concentration from fromUnit to toUnit, bridging
// mass concentration <-> molar concentration via molecular weight mw (g/mol = Da).
// Returns an error when crossing dimensions without mw > 0.
func ConvertConcWithMW(val float64, fromUnit, toUnit string, mw float64) (float64, error) {
	if out, ok, same := Convert(val, fromUnit, toUnit); ok && same {
		d1, _, k1 := DimensionOf(fromUnit)
		d2, _, k2 := DimensionOf(toUnit)
		if k1 && k2 && d1 == d2 {
			return out, nil
		}
		if NormalizeUnit(fromUnit) == "" || NormalizeUnit(toUnit) == "" || strings.EqualFold(NormalizeUnit(fromUnit), NormalizeUnit(toUnit)) {
			return val, nil
		}
	}
	d1, s1, k1 := DimensionOf(fromUnit)
	d2, s2, k2 := DimensionOf(toUnit)
	if !k1 || !k2 {
		return 0, fmt.Errorf("无法识别的浓度单位：%s -> %s", fromUnit, toUnit)
	}
	if d1 == d2 {
		return val * s1 / s2, nil
	}
	if (d1 == DimMassConc && d2 == DimMolarConc) || (d1 == DimMolarConc && d2 == DimMassConc) {
		if mw <= 0 {
			return 0, fmt.Errorf("跨量纲浓度换算（%s -> %s）需要提供分子量 MW (g/mol)", fromUnit, toUnit)
		}
		if d1 == DimMassConc {
			// base mass_conc = ng/mL, base molar = nM => nM = (ng/mL) * 1000 / MW
			nM := (val * s1) * 1000.0 / mw
			return nM / s2, nil
		}
		// base molar = nM => ng/mL = nM * MW / 1000
		ngPerML := (val * s1) * mw / 1000.0
		return ngPerML / s2, nil
	}
	return 0, fmt.Errorf("量纲不兼容：%s (%s) 无法换算为 %s (%s)", fromUnit, d1, toUnit, d2)
}

// DilutionC1V1 solves C1*V1 = C2*V2 deterministically in microliters (uL), enforcing
// physical dimensional compatibility, MW conversion when crossing mass/molar, and C1 > C2.
func DilutionC1V1(c1 float64, c1Unit string, c2 float64, c2Unit string, v2 float64, v2Unit string, mw float64) (v1uL, vDilUL float64, err error) {
	if c1 <= 0 || c2 <= 0 || v2 <= 0 {
		return 0, 0, fmt.Errorf("稀释参数 C1/C2/V2 必须大于 0")
	}
	v2Conv, ok, _ := Convert(v2, v2Unit, "uL")
	if !ok {
		return 0, 0, fmt.Errorf("终体积单位无效：%s", v2Unit)
	}
	c1InC2, err := ConvertConcWithMW(c1, c1Unit, c2Unit, mw)
	if err != nil {
		return 0, 0, err
	}
	if c1InC2 <= c2 {
		return 0, 0, fmt.Errorf("母液浓度 (%s %s) 必须大于目标浓度 (%s %s)", FormatNumber(c1), c1Unit, FormatNumber(c2), c2Unit)
	}
	v1uL = Round4(v2Conv * c2 / c1InC2)
	vDilUL = Round4(v2Conv - v1uL)
	return v1uL, vDilUL, nil
}

// Round4 rounds a floating-point value to 4 decimal places.
func Round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

// FormatNumber formats a number cleanly without trailing zeros.
func FormatNumber(v float64) string {
	r := Round4(v)
	if math.Abs(r-math.Round(r)) < 1e-6 {
		return strconv.FormatInt(int64(math.Round(r)), 10)
	}
	s := strconv.FormatFloat(r, 'f', 4, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}
