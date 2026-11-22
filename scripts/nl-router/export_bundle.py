"""Package a distilled ONNX semantic router checkpoint into a versioned, SHA256-verified
AsterLab Model Bundle (`manifest.json`, `router.int8.onnx`, `vocab.txt`, `schemas.json`).

Usage (upgrade after any future retraining):
  python scripts/nl-router/export_bundle.py \
      --onnx path/to/asterlab_router_v1_int8.onnx \
      --meta path/to/asterlab_router_v1_schemas.npz \
      --backbone BAAI/bge-small-zh-v1.5 \
      --version 1.1.0 \
      --out prototypes/mygo-run/models/nl-router
"""
import argparse
import hashlib
import json
import pathlib
import shutil

import numpy as np
from transformers import AutoTokenizer

REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC_PATH = REPO_ROOT / "prototypes" / "mygo-run" / "internal" / "asterdoc" / "spec" / "nl_schemas.json"

# Must match Go `CanonicalSchemaDescriptor` in nl_router_onnx.go so startup hash matches 100%!
ZH_HINTS = {
    "none": "普通湿实验文字记录或未支持操作，无需插入交互组件：常规PBS清洗、移液吹打混匀、吸弃上清、-20°C/-80°C冰箱保存、细胞代次汇合度观察、历史实验复盘、文献综述、SOP规范、耗材清单、仪器参数检查或未注册专业工具。",
    "timer.cycle": "循环计时器：多阶段交替定时重复多轮操作，例如水浴与冰上交替3轮、反复冻融循环、热激与冰浴交替、超声开停循环。",
    "timer.countdown": "倒计时器：单次固定时长的主动操作定时，包括孵育、离心(如300g 5min)、胰酶消化、PFA固定、封闭、透膜、裂解、酶切、摇床脱色或光交联倒计时。",
    "timer.stopwatch": "正计时秒表：开放时长操作，手工开启秒表计时直至肉眼或物理终点（如完全固化、细胞变圆脱落、显色变蓝、团块散开）并记录实际耗时秒数。",
    "timer.window": "时间锚点等待窗：最小静置/浸泡平衡/细胞贴壁时间窗（至少2小时、不少于4小时、4°C过夜孵育≥12h）或以t0为锚点的后续多时间点随访排程（如6h/12h/24h/48h）。",
    "calculator.dilution": "溶液稀释计算器(C1V1=C2V2)：将已知母液浓度稀释至目标工作浓度和配制终体积（含100x稀释为1x及缺参稀释请求），计算需取母液体积和补液量。",
    "calculator.weighing": "称量配液计算器：根据目标质量体积浓度(% w/v)或摩尔浓度(mM与分子量MW)及配制体积，计算需称取的固体干粉质量(mg/g)。",
    "calculator.mastermix": "孔板预混液计算器：按单孔试剂用量放大至多孔/多反应体系，并包含富余比例(如+10%/+15%余量或额外多算几孔)计算预混总液量。",
    "calculator.seeding": "细胞接种计算器：根据实测细胞悬液密度(cells/mL)、每孔目标细胞数、接种孔数与每孔体积，计算需取细胞悬液和补加培养基体积。",
    "plate.layout": "孔板与实验分组布局：定义6/12/24/48/96孔板规格、各实验处理组/对照组/空白组及每组技术复孔排布。",
    "attachment.grid": "孔板采图网格：定义多孔板系统性显微拍照或荧光成像方案，指定采图孔位、每孔非重叠视野数(FOV)或荧光通道槽位。",
}


def sha256_file(path: pathlib.Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def hash_desc(desc: str) -> str:
    return hashlib.sha256(desc.strip().encode("utf-8")).hexdigest()[:16]


def build_descriptor(reg: dict, tid: str) -> str:
    if tid == "none":
        return f"无需组件 (none) | {ZH_HINTS['none']} | {reg['global_negative_boundary']}"
    by_id = {t["id"]: t for t in reg["targets"]}
    sc = by_id[tid]
    zh = ZH_HINTS.get(tid, sc["title"])
    return f"{sc['title']} ({tid}) | {zh} | {sc['when_to_suggest']} | Not: {sc['when_not_to_suggest']}"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--onnx", required=True, help="Path to INT8 ONNX model file")
    ap.add_argument("--meta", required=True, help="Path to schemas .npz file from distillation")
    ap.add_argument("--backbone", default="BAAI/bge-small-zh-v1.5")
    ap.add_argument("--cache-dir", default=None)
    ap.add_argument("--version", default="1.1.0")
    ap.add_argument("--pos-gate", type=float, default=0.50)
    ap.add_argument("--out", required=True, help="Output bundle directory")
    args = ap.parse_args()

    out_dir = pathlib.Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)

    # 1. Copy ONNX & compute SHA256
    onnx_dst = out_dir / "router.int8.onnx"
    shutil.copy2(args.onnx, onnx_dst)
    onnx_sha = sha256_file(onnx_dst)

    # 2. Export vocab.txt from HuggingFace tokenizer
    tok_kwargs = {"cache_dir": args.cache_dir} if args.cache_dir else {}
    tok = AutoTokenizer.from_pretrained(args.backbone, **tok_kwargs)
    saved = tok.save_vocabulary(str(out_dir))
    vocab_src = pathlib.Path(saved[0])
    vocab_dst = out_dir / "vocab.txt"
    if vocab_src.resolve() != vocab_dst.resolve():
        shutil.move(str(vocab_src), str(vocab_dst))

    # 3. Export portable schemas.json with descriptor hashes + precomputed unit vectors
    reg = json.loads(SPEC_PATH.read_text(encoding="utf-8"))
    meta = np.load(args.meta)
    target_ids = [str(x) for x in meta["target_ids"]]
    desc_embs = meta["desc_embs"].astype(np.float32)
    scale = float(meta["scale"][0])
    noul_bias = float(meta["noul_bias"][0])
    embed_dim = int(desc_embs.shape[1])

    entries = []
    for tid, vec in zip(target_ids, desc_embs):
        desc = build_descriptor(reg, tid)
        entries.append(
            {
                "id": tid,
                "desc_hash": hash_desc(desc),
                "embedding": [round(float(x), 6) for x in vec.tolist()],
            }
        )
    if "meta_ids" in meta and "meta_embs" in meta:
        meta_ids = [str(x) for x in meta["meta_ids"]]
        meta_descs = [str(x) for x in meta["meta_descs"]] if "meta_descs" in meta else meta_ids
        meta_embs = meta["meta_embs"].astype(np.float32)
        for mid, mdesc, mvec in zip(meta_ids, meta_descs, meta_embs):
            entries.append(
                {
                    "id": mid,
                    "desc_hash": hash_desc(mdesc),
                    "embedding": [round(float(x), 6) for x in mvec.tolist()],
                }
            )
    (out_dir / "schemas.json").write_text(
        json.dumps(
            {"model_version": args.version, "embed_dim": embed_dim, "entries": entries},
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )

    # 4. Write self-describing manifest.json (with task_prefixes reserved for future multi-task upgrades!)
    manifest = {
        "schema_version": 1,
        "model_version": args.version,
        "backbone": args.backbone,
        "onnx_file": "router.int8.onnx",
        "onnx_sha256": onnx_sha,
        "vocab_file": "vocab.txt",
        "schemas_file": "schemas.json",
        "tokenizer_type": "wordpiece_bert",
        "do_lower_case": True,
        "max_text_len": 96,
        "max_desc_len": 128,
        "embed_dim": embed_dim,
        "input_names": ["input_ids", "attention_mask"],
        "output_name": "embedding",
        "scale": round(scale, 6),
        "noul_bias": round(noul_bias, 6),
        "pos_gate": args.pos_gate,
        "task_prefixes": {
            "route": "",
            "route_desc": "",
            "retrieve": "",
        },
    }
    (out_dir / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8"
    )

    print(f"Exported AsterLab NL-Router Bundle v{args.version} -> {out_dir}")
    print(f"  router.int8.onnx : {onnx_dst.stat().st_size / 1048576:.2f} MB (sha256={onnx_sha[:16]}...)")
    print(f"  vocab.txt        : {vocab_dst.stat().st_size / 1024:.1f} KB")
    print(f"  schemas.json     : {len(entries)} targets (dim={embed_dim}, scale={scale:.2f}, noul_bias={noul_bias:.3f})")


if __name__ == "__main__":
    main()
