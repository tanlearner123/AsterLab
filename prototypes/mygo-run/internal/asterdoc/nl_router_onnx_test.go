package asterdoc

import (
	"runtime"
	"sort"
	"testing"
	"time"

	"asterlab-mygo-prototype/internal/nlengine"
)

func TestPureGoONNXRouterEndToEndBench24AndZeroShot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows onnxruntime.dll integration test")
	}
	router, err := OpenDefaultNLOnnxRouter()
	if err != nil {
		t.Skipf("nl-router bundle or onnxruntime.dll not available in this environment: %v", err)
	}
	defer router.Close()

	m := router.Engine().Manifest()
	t.Logf("Loaded bundle v%s (%s) from %s | ORT %s",
		m.ModelVersion, m.Backbone, router.Engine().BundleDir(), router.Engine().ORTVersion())

	bench24 := []struct {
		want string
		text string
	}{
		{"timer.cycle", "80°C 水浴 5 min 与冰上 10 min 交替 3 轮至澄清"},
		{"timer.cycle", "先37度孵育15分钟再室温静置5分钟，重复4个循环"},
		{"timer.countdown", "37°C 培养箱孵育 30 min 使其充分聚合成胶"},
		{"timer.countdown", "离心 300g 5 分钟后弃上清"},
		{"timer.stopwatch", "405 nm 蓝光照射直至水凝胶完全固化，记录实际固化秒数"},
		{"timer.window", "4°C 完全培养基浸泡平衡至少 2 小时（可并行处理其他实验）"},
		{"timer.window", "接种后 t0 起分别于 6h / 12h / 24h 进行明场成像"},
		{"calculator.dilution", "将 100 ug/mL VEGF 母液稀释至 50 ng/mL，共配制 5 mL 工作液"},
		{"calculator.dilution", "10 mM 药物母液用 DMEM 稀释到 5 uM，终体积 2 mL"},
		{"calculator.weighing", "配制 5% w/v GelMA 前驱液 2 mL，计算需称取的冻干粉质量"},
		{"calculator.weighing", "配制 10 mM 药物溶液 5 mL（分子量 MW = 543.5 g/mol），需称量多少毫克"},
		{"calculator.mastermix", "24 孔板预混液（多配 15% 余量）：每孔 2x Mix 10 uL、引物 1.6 uL、水 6.4 uL"},
		{"calculator.seeding", "实测 HUVEC 悬液密度 5e5 cells/mL，每孔接种 5e4 细胞（500 uL/孔），共 12 孔加 2 孔余量"},
		{"plate.layout", "12 孔板，3% / 5% / 7% GelMA 各 3 个技术复孔，第 4 列加无菌 PBS 作空白对照"},
		{"plate.layout", "96 孔板排布：对照组、低剂量、中剂量、高剂量共 4 组，每组 6 复孔，四周加 PBS 防蒸发"},
		{"plate.layout", "用 6 孔板做处理组和阴性对照两组，每组 3 孔"},
		{"attachment.grid", "对 A1-C3 共 9 个孔进行 10× 明场采图，每孔随机拍 6 个非重叠视野（共 54 张）"},
		{"attachment.grid", "24 孔板每孔荧光拍照 4 个视野，分别保存 DAPI 和 GFP 通道图"},
		{"none", "用无菌 PBS 轻柔清洗细胞 2 遍后吸弃上清"},
		{"none", "用 1 mL 移液枪轻柔吹打混匀 3 次，注意避免产生气泡"},
		{"none", "剩余 GelMA 冻干粉避光密封保存于 -20°C 冰箱"},
		{"none", "评估 GelMA 浓度梯度对 HUVEC 体外成管的影响"},
		{"none", "本批次细胞传代至 P5，汇合度约 85%，状态良好"},
		{"none", "复核上次实验中 5% GelMA 组管腔长度偏高的原因"},
	}

	// Warmup
	_, _ = router.RouteDetail("37°C 孵育 10 min")

	var lats []float64
	exactOK := 0
	fp := 0
	for _, tc := range bench24 {
		t0 := time.Now()
		res, err := router.RouteDetail(tc.text)
		ms := float64(time.Since(t0).Microseconds()) / 1000.0
		if err != nil {
			t.Fatalf("RouteDetail(%q) failed: %v", tc.text, err)
		}
		lats = append(lats, ms)
		if res.TargetID == tc.want {
			exactOK++
		} else {
			t.Logf("  MISMATCH want=%s got=%s (p_pos=%.2f conf=%.2f) | %s", tc.want, res.TargetID, res.PosProb, res.Confidence, tc.text)
		}
		if tc.want == "none" && res.TargetID != "none" {
			fp++
		}
	}
	sort.Float64s(lats)
	p50 := lats[len(lats)/2]
	p95 := lats[int(float64(len(lats))*0.95)]
	t.Logf("Pure-Go INT8 ONNX Bench24: %d/%d (%.1f%%) | FP=%d/6 | Latency p50=%.2f ms, p95=%.2f ms",
		exactOK, len(bench24), 100*float64(exactOK)/float64(len(bench24)), fp, p50, p95)

	if exactOK < 22 {
		t.Fatalf("expected >= 22/24 exact accuracy in pure Go ONNX runtime, got %d/24", exactOK)
	}
	if fp != 0 {
		t.Fatalf("expected 0/6 hard-negative false positives, got %d/6", fp)
	}

	// Verify general-purpose Encode + CosineSim primitive (reserved for future semantic matching)
	vA, err := router.Engine().Encode("retrieve", "离心 300g 5 分钟后弃上清")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	vB, _ := router.Engine().Encode("retrieve", "Spin cells at 300g for 5 min and discard supernatant")
	vC, _ := router.Engine().Encode("retrieve", "将 100 ug/mL VEGF 母液稀释至 50 ng/mL")
	simSame := nlengine.CosineSim(vA, vB)
	simDiff := nlengine.CosineSim(vA, vC)
	t.Logf("General Encode/CosineSim sanity: sim(centrifuge_zh, centrifuge_en)=%.3f vs sim(centrifuge_zh, dilution_zh)=%.3f", simSame, simDiff)
	if simSame <= simDiff {
		t.Fatalf("expected semantically equivalent sentences to have higher cosine sim (%.3f <= %.3f)", simSame, simDiff)
	}

	// Verify Zero-Shot dynamic schema registration at runtime (no retraining!)
	pcrDesc := "PCR 热循环扩增程序 (pcr.program) | PCR/qPCR扩增温控程序：设定预变性温度时间、变性/退火/延伸温度时长及循环次数。 | Sentence defines a PCR or qPCR thermal cycler program with denaturation, annealing, extension temperatures, durations, and cycle count."
	if err := router.RegisterZeroShotSchema("pcr.program", "pcr", "program", pcrDesc); err != nil {
		t.Fatalf("RegisterZeroShotSchema failed: %v", err)
	}
	zsRes, err := router.RouteDetail("设置 PCR 扩增程序：95°C 预变性 3 min；95°C 15 s、60°C 30 s、72°C 30 s 共 35 个循环")
	if err != nil {
		t.Fatalf("ZeroShot RouteDetail failed: %v", err)
	}
	t.Logf("Pure-Go Zero-Shot unseen schema `pcr.program`: got=%s (p_pos=%.2f, conf=%.2f)", zsRes.TargetID, zsRes.PosProb, zsRes.Confidence)
	if zsRes.TargetID != "pcr.program" {
		t.Fatalf("expected Zero-Shot pcr.program match, got %+v", zsRes)
	}

	// Verify v1.3.0+ semantic single-vs-multi action probe on the exact same 512-d vector (0.001 ms extra)
	if router.HasMultiActionProbe() {
		rMulti, err := router.RouteDetail("室温避光二抗孵育 1 h；随后加 DAPI 染核 5 min")
		if err != nil {
			t.Fatalf("RouteDetail multi failed: %v", err)
		}
		rSingle, err := router.RouteDetail("37°C 水浴 10 min 与冰浴 5 min 交替 3 轮，促使冻干粉完全溶解")
		if err != nil {
			t.Fatalf("RouteDetail single failed: %v", err)
		}
		t.Logf("Pure-Go v1.3.0 Multi-Action Probe: multi=(%v, p=%.2f) vs tricky_single_cycle=(%v, p=%.2f)",
			rMulti.IsMulti, rMulti.MultiProb, rSingle.IsMulti, rSingle.MultiProb)
		if !rMulti.IsMulti {
			t.Fatalf("expected compound sentence to trigger IsMulti=true, got p=%.2f", rMulti.MultiProb)
		}
		if rSingle.IsMulti {
			t.Fatalf("expected single cycle timer NOT to trigger IsMulti, got p=%.2f", rSingle.MultiProb)
		}
	}
}
