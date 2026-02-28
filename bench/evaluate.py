#!/usr/bin/env python3
"""Evaluate stdiag benchmark results against ground truth scenarios."""

import argparse
import json
import sys
from pathlib import Path

# Manual diagnosis keywords per error code
MANUAL_KEYWORDS = {
    "DNS_NXDOMAIN": ["NXDOMAIN", "name not found"],
    "DNS_TIMEOUT": ["timed out", "no servers could be reached", "connection timed out"],
    "REACHABILITY_TIMEOUT": ["100% packet loss", "unreachable", "timed out"],
    "TLS_CERT_EXPIRED": ["certificate has expired", "expired"],
    "TLS_CERT_EXPIRING_SOON": ["certificate will expire", "expiring soon"],
    "TLS_HOSTNAME_MISMATCH": ["hostname mismatch", "does not match"],
    "TLS_UNTRUSTED_CHAIN": ["self-signed", "unable to verify", "unable to get local issuer"],
    "TLS_NO_CERTIFICATES": ["no peer certificate"],
    "TLS_PROTOCOL_ERROR": ["wrong version number", "connection reset", "protocol version"],
    "TCP_REFUSED": ["Connection refused"],
    "TCP_TIMEOUT": ["timed out", "connection timed out"],
    "HTTP_503": ["503"],
    "HTTP_403": ["403"],
    "HTTP_500": ["500"],
    "HTTP_429": ["429"],
}

# Keywords that indicate an error in manual output (for healthy scenarios)
ALL_ERROR_KEYWORDS = []
for kws in MANUAL_KEYWORDS.values():
    ALL_ERROR_KEYWORDS.extend(kws)


def evaluate_stdiag_diagnosis(stdiag: dict, scenario: dict) -> bool:
    """Check if stdiag correctly diagnosed the scenario."""
    ground_truth = scenario["ground_truth"]
    layers = stdiag.get("layers", {})

    if ground_truth is None:
        # Healthy scenario: all layers should be ok, skip, or warn
        return all(
            layer.get("status") in ("ok", "skip", "warn")
            for layer in layers.values()
        )

    # Warning scenario (e.g. TLS_CERT_EXPIRING_SOON): check warn status + code
    if ground_truth in ("TLS_CERT_EXPIRING_SOON",):
        for layer in layers.values():
            if layer.get("status") == "warn":
                error = layer.get("error")
                if error and error.get("code") == ground_truth:
                    return True
        return False

    # Error scenario: find a layer with matching error code
    for layer in layers.values():
        error = layer.get("error")
        if error and error.get("code") == ground_truth:
            return True
    return False


def evaluate_manual_diagnosis(manual_text: str, scenario: dict) -> bool:
    """Check if manual output correctly identifies the issue."""
    ground_truth = scenario["ground_truth"]

    if ground_truth is None:
        # Healthy scenario: no error keywords should be present
        text_lower = manual_text.lower()
        return not any(kw.lower() in text_lower for kw in ALL_ERROR_KEYWORDS)

    # Error scenario: check for matching keywords
    keywords = MANUAL_KEYWORDS.get(ground_truth, [])
    if not keywords:
        return False
    return any(kw.lower() in manual_text.lower() for kw in keywords)


def evaluate_evidence_recall(stdiag: dict, scenario: dict) -> float:
    """Calculate recall of required evidence fields in stdiag output."""
    required = scenario.get("required_evidence", [])
    if not required:
        return 1.0

    layers = stdiag.get("layers", {})
    # Collect all observation keys across all layers
    all_obs_keys = set()
    for layer in layers.values():
        obs = layer.get("observations", {})
        all_obs_keys.update(obs.keys())

    present = sum(1 for field in required if field in all_obs_keys)
    return present / len(required)


def count_tokens_file(path: Path) -> dict:
    """Read token counts from a .tokens.json sidecar file.

    Normalizes keys to canonical model names:
      claude_tokens / claude_tokens_approx -> claude
      gpt4o_tokens -> gpt4o
    """
    tokens_path = path.parent / (path.stem + ".tokens.json")
    if not tokens_path.exists():
        return {}
    with open(tokens_path) as f:
        raw = json.load(f)
    result = {}
    # Claude: prefer exact count, fall back to approximation
    if "claude_tokens" in raw:
        result["claude"] = raw["claude_tokens"]
    elif "claude_tokens_approx" in raw:
        result["claude"] = raw["claude_tokens_approx"]
    if "gpt4o_tokens" in raw:
        result["gpt4o"] = raw["gpt4o_tokens"]
    return result


def evaluate(scenarios_path: Path, results_dir: Path) -> dict:
    """Run full evaluation across all scenarios."""
    with open(scenarios_path) as f:
        scenarios = json.load(f)

    results = []
    total_stdiag_correct = 0
    total_manual_correct = 0
    total_evidence_recall = 0.0
    total_scenarios = len(scenarios)

    # Accumulators for token efficiency
    token_models = set()
    stdiag_tokens_correct = {}
    manual_tokens_correct = {}
    stdiag_tokens_total = {}
    manual_tokens_total = {}

    for scenario in scenarios:
        name = scenario["name"]
        scenario_dir = results_dir / name

        # Load stdiag result
        stdiag_path = scenario_dir / "stdiag.json"
        stdiag = {}
        if stdiag_path.exists():
            with open(stdiag_path) as f:
                stdiag = json.load(f)

        # Load manual result
        manual_path = scenario_dir / "manual.txt"
        manual_text = ""
        if manual_path.exists():
            manual_text = manual_path.read_text()

        # Evaluate
        stdiag_correct = evaluate_stdiag_diagnosis(stdiag, scenario)
        manual_correct = evaluate_manual_diagnosis(manual_text, scenario)
        evidence_recall = evaluate_evidence_recall(stdiag, scenario)

        if stdiag_correct:
            total_stdiag_correct += 1
        if manual_correct:
            total_manual_correct += 1
        total_evidence_recall += evidence_recall

        # Token counts
        stdiag_tokens = count_tokens_file(stdiag_path)
        manual_tokens = count_tokens_file(manual_path)
        token_models.update(stdiag_tokens.keys())
        token_models.update(manual_tokens.keys())

        for model, count in stdiag_tokens.items():
            stdiag_tokens_total[model] = stdiag_tokens_total.get(model, 0) + count
            if stdiag_correct:
                stdiag_tokens_correct[model] = stdiag_tokens_correct.get(model, 0) + count
        for model, count in manual_tokens.items():
            manual_tokens_total[model] = manual_tokens_total.get(model, 0) + count
            if manual_correct:
                manual_tokens_correct[model] = manual_tokens_correct.get(model, 0) + count

        result_entry = {
            "name": name,
            "stdiag_correct": stdiag_correct,
            "manual_correct": manual_correct,
            "evidence_recall": round(evidence_recall, 4),
        }
        if stdiag_tokens:
            result_entry["stdiag_tokens"] = stdiag_tokens
        if manual_tokens:
            result_entry["manual_tokens"] = manual_tokens

        results.append(result_entry)

    # Build summary
    stdiag_accuracy = total_stdiag_correct / total_scenarios if total_scenarios else 0
    manual_accuracy = total_manual_correct / total_scenarios if total_scenarios else 0
    avg_evidence_recall = total_evidence_recall / total_scenarios if total_scenarios else 0

    summary = {
        "stdiag_accuracy": round(stdiag_accuracy, 4),
        "manual_accuracy": round(manual_accuracy, 4),
        "avg_evidence_recall": round(avg_evidence_recall, 4),
    }

    # Token efficiency per model
    for model in sorted(token_models):
        sc = stdiag_tokens_correct.get(model, 0)
        n_sc = total_stdiag_correct or 1
        mc = manual_tokens_correct.get(model, 0)
        n_mc = total_manual_correct or 1

        summary[f"stdiag_tokens_per_correct_{model}"] = round(sc / n_sc, 1)
        summary[f"manual_tokens_per_correct_{model}"] = round(mc / n_mc, 1)

        if mc > 0:
            summary[f"token_efficiency_ratio_{model}"] = round(sc / mc, 4) if mc else 0
        else:
            summary[f"token_efficiency_ratio_{model}"] = None

    return {"scenarios": results, "summary": summary}


def main():
    parser = argparse.ArgumentParser(description="Evaluate stdiag benchmark results")
    parser.add_argument(
        "--scenarios", required=True, type=Path, help="Path to scenarios.json"
    )
    parser.add_argument(
        "--results-dir", required=True, type=Path, help="Path to results directory"
    )
    args = parser.parse_args()

    if not args.scenarios.exists():
        print(f"Error: scenarios file not found: {args.scenarios}", file=sys.stderr)
        sys.exit(1)
    if not args.results_dir.exists():
        print(f"Error: results directory not found: {args.results_dir}", file=sys.stderr)
        sys.exit(1)

    output = evaluate(args.scenarios, args.results_dir)
    json.dump(output, sys.stdout, indent=2)
    print()


if __name__ == "__main__":
    main()
