#!/usr/bin/env python3
"""按候选提交核对远端验证；显式发布时才创建不可变版本 tag。"""

import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def api(path):
    return json.loads(command("gh", "api", path))


def source(candidate, path):
    return command("git", "show", candidate + ":" + path)


def tag_commit(repo, tag):
    obj = api(f"repos/{repo}/git/ref/tags/{tag}")["object"]
    while obj["type"] == "tag":
        obj = api(f"repos/{repo}/git/tags/{obj['sha']}")["object"]
    if obj["type"] != "commit":
        raise SystemExit("Release tag does not resolve to a commit")
    return obj["sha"]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("version")
    parser.add_argument("--candidate", default="HEAD")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--prepare", action="store_true", help="Start missing checks or rerun failed jobs")
    mode.add_argument("--publish", action="store_true", help="Create and push the tag after verification")
    args = parser.parse_args()
    if not re.fullmatch(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)", args.version):
        raise SystemExit("A stable semantic version tag is required")
    root = Path(__file__).resolve().parent.parent
    import os
    os.chdir(root)
    candidate = command("git", "rev-parse", args.candidate + "^{commit}")
    repo = json.loads(command("gh", "repo", "view", "--json", "nameWithOwner"))["nameWithOwner"]
    comparison = api(f"repos/{repo}/compare/{candidate}...main")
    if comparison["status"] not in ("ahead", "identical"):
        raise SystemExit("Candidate is not in main history")
    main_sha = api(f"repos/{repo}/branches/main")["commit"]["sha"]
    refs = api(f"repos/{repo}/git/matching-refs/tags/")
    tags = [item["ref"].removeprefix("refs/tags/") for item in refs]
    stable = [tag for tag in tags if re.fullmatch(r"v\d+\.\d+\.\d+", tag) and tag != args.version]
    base = max(stable, key=lambda tag: tuple(map(int, tag[1:].split("."))))
    if tuple(map(int, args.version[1:].split("."))) <= tuple(map(int, base[1:].split("."))):
        raise SystemExit("Release version must be newer than the latest stable version")
    base_sha = tag_commit(repo, base)
    consumers = json.loads(source(candidate, ".github/consumer-baselines.json"))
    workflow = source(candidate, ".github/workflows/api-compat.yml")
    baseline_path = re.search(r'API_BREAKING_BASELINE:\s*"([^"]+)"', workflow)[1]
    baseline = subprocess.check_output(["git", "show", candidate + ":" + baseline_path])
    baseline_digest = hashlib.sha256(baseline).hexdigest()
    gorelease = re.search(r"GORELEASE_VERSION:\s*(\S+)", workflow)[1]
    pending = []
    receipts = []
    for filename in ("ci.yml", "api-compat.yml", "downstream.yml", "sandbox-code-exec.yml"):
        runs = api(f"repos/{repo}/actions/workflows/{filename}/runs?head_sha={candidate}&per_page=100")["workflow_runs"]
        runs = [run for run in runs if run["head_branch"] == "main"
                and run["event"] in ("push", "workflow_dispatch")
                and run["head_repository"]["full_name"] == repo]
        run = max(runs, key=lambda item: item["id"]) if runs else None
        if run is None:
            if args.prepare and candidate == main_sha:
                dispatch = ["gh", "workflow", "run", filename, "--ref", "main", "--repo", repo]
                if filename == "sandbox-code-exec.yml":
                    dispatch.extend(["-f", "hexclaw_ref=" + consumers["hexclaw"], "-f", "run_live_network=false"])
                command(*dispatch)
            pending.append(filename + ": missing exact-SHA verification")
            continue
        if run["status"] != "completed" or run["conclusion"] != "success":
            if args.prepare and run["status"] == "completed":
                command("gh", "run", "rerun", str(run["id"]), "--failed", "--repo", repo)
            pending.append(filename + ": " + (run["conclusion"] or run["status"]) + " " + run["html_url"])
            continue
        with tempfile.TemporaryDirectory(prefix="toolkit-release-") as directory:
            command("gh", "run", "download", str(run["id"]), "--repo", repo,
                    "--pattern", "verification-inputs-*", "--dir", directory)
            records = [json.loads(path.read_text()) for path in Path(directory).rglob("*-inputs.json")]
        selected = {}
        for record in records:
            if (record.get("candidate_sha") != candidate or record.get("event_sha") != candidate
                    or record.get("run_id") != str(run["id"]) or record.get("event") != run["event"]):
                raise SystemExit("Verification receipt does not match its exact-SHA run: " + filename)
            platform = record["platform"]
            if platform not in selected or int(record["run_attempt"]) > int(selected[platform]["run_attempt"]):
                selected[platform] = record
        expected = {"linux", "darwin", "windows"} if filename == "sandbox-code-exec.yml" else {"linux"}
        if set(selected) != expected:
            raise SystemExit("Verification receipts are incomplete: " + filename)
        for record in selected.values():
            if filename in ("downstream.yml", "sandbox-code-exec.yml") and record.get("consumers") != consumers:
                raise SystemExit("Consumer verification does not use the candidate baselines")
            if filename == "api-compat.yml" and (
                    record.get("base_tag") != base or record.get("base_sha") != base_sha
                    or record.get("baseline_sha256") != baseline_digest or record.get("gorelease_version") != gorelease):
                if args.prepare:
                    command("gh", "run", "rerun", str(run["id"]), "--repo", repo)
                pending.append("API comparison inputs changed; rerun API verification")
        receipts.append({"workflow": filename, "run_id": run["id"], "url": run["html_url"]})
    if pending:
        raise SystemExit("Release is not ready:\n" + "\n".join(pending))
    # tag 创建前再次核对发布比较基线，避免检查期间发生另一轮发布。
    current_tags = api(f"repos/{repo}/git/matching-refs/tags/")
    if {item["ref"] for item in current_tags} != {item["ref"] for item in refs}:
        raise SystemExit("Published tags changed during verification; check again")
    print(json.dumps({"version": args.version, "candidate": candidate, "base": base, "runs": receipts}, indent=2))
    if args.version in tags:
        if tag_commit(repo, args.version) != candidate:
            raise SystemExit("Existing release tag points to a different commit")
        print("Release tag already points to the verified candidate")
        return
    if args.publish:
        if command("git", "status", "--porcelain"):
            raise SystemExit("A clean worktree is required before publishing")
        local = subprocess.run(["git", "rev-parse", "--verify", "refs/tags/" + args.version], capture_output=True)
        if local.returncode == 0:
            raise SystemExit("Local release tag already exists; inspect it before publishing")
        message = f"发布 {args.version}\n"
        with tempfile.NamedTemporaryFile(mode="w", prefix="toolkit-tag-", delete=True) as stream:
            stream.write(message)
            stream.flush()
            subprocess.run(["git", "tag", "-a", args.version, candidate, "-F", stream.name], check=True)
        subprocess.run(["git", "push", "origin", "refs/tags/" + args.version], check=True)


if __name__ == "__main__":
    main()
