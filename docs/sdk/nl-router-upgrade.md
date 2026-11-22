# AsterLab 端侧语义路由引擎（NL-Router）升级与扩展指南

本文档规定 AsterLab 端侧离线 ONNX 语义编码器（`internal/nlengine`）与组件路由器（`asterdoc.NLOnnxRouter`）的模型包契约、热插拔升级路径，以及未来扩展到新组件或新语义任务的标准流程。

---

## 1. 架构分层

1. **底层通用编码引擎**（`prototypes/mygo-run/internal/nlengine/`）：
   - 纯 Go 零 CGO 调用 `onnxruntime.dll`（使用 ABI 稳定的 `OrtGetApiBase()->GetApi(17)`，兼容 ORT 1.17–1.25+ 及后续版本）；
   - 纯 Go BERT/BGE WordPiece 分词器（`wordpiece.go`）；
   - 统一暴露 `Encode(task, text) -> []float32` 与 `CosineSim(a, b)`，在 `manifest.json` 中预留 `task_prefixes`（如 `"route"` / `"retrieve"` / `"k2_align"`），未来可扩展到历史步骤语义检索、偏离相似坑点推荐和 K2 操作对齐而无需重复加载 DLL。
2. **上层组件路由层**（`prototypes/mygo-run/internal/asterdoc/nl_router_onnx.go`）：
   - 消费底层 512 维单位向量与 `spec/nl_schemas.json` 组件描述做余弦打分 + 层次门控（`1 - P(none) >= pos_gate`）；
   - 启动时比对每个 Schema 的 `desc_hash`：若 `spec/nl_schemas.json` 新增或修改了组件描述，Go 自动在启动期用 ONNX 编码新描述，实现 **Zero-Shot 即插即用**；
   - 若机器上未安装模型包或 `onnxruntime.dll`（例如 Linux CI），自动安全降级为 `<1 ms` 确定性规则路由 `localRouteSentence`。

---

## 2. 版本化模型包规范（Bundle Contract v1）

每个可热替换的模型包是一个独立目录（`models/nl-router/`）：

```text
models/nl-router/
  manifest.json      # 自描述契约（版本、分词器、SHA256、scale/noul_bias/pos_gate、task_prefixes）
  router.int8.onnx   # INT8 动态量化编码器（输入 input_ids / attention_mask，输出 L2 归一化 embedding）
  vocab.txt          # 标准 HuggingFace WordPiece 词表
  schemas.json       # 预计算的组件描述向量 + desc_hash（可选；缺失或哈希变化时 Go 自动重算）
```

> 大体积二进制（`*.onnx` 与 `prototypes/mygo-run/models/nl-router/`）已在 `.gitignore` 忽略，不直接提交进 Git 仓库。

---

## 3. 模型包加载优先级与零改代码升级路径

`nlengine.ResolveBundleDir()` 按以下优先级查找模型包（命中第一个包含合法 `manifest.json` 的目录即加载，并校验 `onnx_sha256`）：

1. **环境变量覆盖**（开发 / A-B 对比）：`ASTERLAB_NL_ROUTER_DIR=/path/to/new-bundle`
2. **用户热升级目录**（线上静默升级推荐位置）：`%LOCALAPPDATA%\AsterLab\models\nl-router\`
3. **随安装包内置目录**：`<AsterLab.exe 同级>\models\nl-router\`
4. **本地开发目录**：`prototypes/mygo-run/models/nl-router/`

### 升级步骤（无需改 Go 代码、无需重编译）

1. 在本地 GPU 或 Colab 重训/蒸馏出新 `.onnx` + `.npz`；
2. 运行一行打包命令生成带 SHA256 校验的新模型包：
   ```bash
   python scripts/nl-router/export_bundle.py \
     --onnx path/to/new_router_int8.onnx \
     --meta path/to/new_schemas.npz \
     --backbone BAAI/bge-small-zh-v1.5 \
     --version 1.2.0 \
     --out "%LOCALAPPDATA%\AsterLab\models\nl-router"
   ```
3. 重启客户端（或在运行时调用 `asterdoc.ReloadDefaultNLRouter()`）立即生效；若需回滚，替换回旧目录或删除用户热升级目录即可自动回退到安装包内置版本。
4. 本地验证（纯 Go 直接跑 ONNX Runtime 盲测与延迟）：
   ```powershell
   cd prototypes/mygo-run
   go test ./internal/asterdoc -run TestPureGoONNXRouterEndToEndBench24AndZeroShot -v
   ```

---

## 4. 新增组件的两种路径

| 场景 | 操作方式 | 是否需要重训 |
|---|---|---|
| **快速上线新组件（Zero-Shot）** | 在 `spec/nl_schemas.json` 添加新条目（按 `docs/sdk/nl-extraction-schema.md` 4 条铁律写清 `when_to_suggest` / `when_not_to_suggest` 与 5 个原子槽位）+ Go 展开器 | **否**（启动时 Go 自动编码新描述向量） |
| **高精校准 / 扩展多语言或新任务** | 补充 50–100 条正/负样本过教师蒸馏，重跑 `export_bundle.py` 产出新版模型包（如 `v1.2.0`）并更新 `task_prefixes` | 是（RTX 5060 约 1 分钟，零改 Go 代码） |
