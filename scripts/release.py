#!/usr/bin/env python3
"""发布前复用同一提交的核心验证；发布后独立核验不可变 tag 和模块。"""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


class ReleaseError(Exception):
    pass


def command(*args, **kwargs):
    try:
        return subprocess.check_output(args, text=True, stderr=subprocess.PIPE, **kwargs).strip()
    except subprocess.CalledProcessError as error:
        detail = (error.stderr or "").strip() or (error.output or "").strip() or str(error)
        raise ReleaseError(detail) from error
    except OSError as error:
        raise ReleaseError(f"Cannot execute {args[0]}: {error}") from error


def api(path):
    return json.loads(command("gh", "api", path))


def source(candidate, path):
    return command("git", "show", candidate + ":" + path)


def version_tuple(version):
    if not re.fullmatch(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)", version):
        raise ReleaseError("A stable semantic version tag is required")
    return tuple(map(int, version[1:].split(".")))


def tag_commit(repo, tag):
    obj = api(f"repos/{repo}/git/ref/tags/{tag}")["object"]
    while obj["type"] == "tag":
        obj = api(f"repos/{repo}/git/tags/{obj['sha']}")["object"]
    if obj["type"] != "commit":
        raise ReleaseError("Release tag does not resolve to a commit")
    return obj["sha"]


def local_tag_commit(version):
    result = subprocess.run(["git", "rev-parse", "--verify", "refs/tags/" + version + "^{commit}"],
                            text=True, capture_output=True)
    return result.stdout.strip() if result.returncode == 0 else None


def select_go_toolchain(go_mod):
    # 用目标模块选择工具链，保持历史核验独立于当前工作区，并尊重 auto/local。
    with tempfile.TemporaryDirectory(prefix="toolkit-toolchain-") as directory:
        Path(directory, "go.mod").write_text(go_mod, encoding="utf-8")
        env = dict(os.environ, GOWORK="off")
        go_root = command("go", "env", "GOROOT", cwd=directory, env=env)
        executable = Path(go_root) / "bin" / ("go.exe" if os.name == "nt" else "go")
        # go env 不一定检查最低版本；只加载主模块即可验证工具链，无须下载依赖。
        command(str(executable), "list", "-m", "-mod=readonly", cwd=directory,
                env=dict(env, GOTOOLCHAIN="local"))
        return executable


def download_module(executable, module, version, directory, env):
    result = subprocess.run([str(executable), "mod", "download", "-json", module + "@" + version],
                            cwd=directory, env=env, text=True, capture_output=True)
    try:
        downloaded = json.loads(result.stdout)
    except json.JSONDecodeError as error:
        detail = result.stderr.strip() or result.stdout.strip() or f"exit status {result.returncode}"
        raise ReleaseError("Go module download did not return valid JSON: " + detail) from error
    if not isinstance(downloaded, dict):
        raise ReleaseError("Go module download did not return a JSON object")
    if downloaded.get("Error"):
        raise ReleaseError("Go module download failed: " + str(downloaded["Error"]))
    if result.returncode:
        detail = result.stderr.strip() or result.stdout.strip() or f"exit status {result.returncode}"
        raise ReleaseError("Go module download failed: " + detail)
    return downloaded


def verify_published(repo, version, candidate):
    # 历史发布核验不读取最新版本、候选 CI 产物或当前工作区的 go.mod。
    document = api(f"repos/{repo}/contents/go.mod?ref={candidate}")
    go_mod = base64.b64decode(document["content"]).decode()
    match = re.search(r'(?m)^module\s+"?([^"\s]+)', go_mod)
    if not match:
        raise ReleaseError("Published source has no module path")
    module = match[1]
    go_executable = select_go_toolchain(go_mod)
    with tempfile.TemporaryDirectory(prefix="toolkit-module-") as directory:
        # 直接读取不可变 Git tag，避免镜像收录延迟；Go 校验和数据库仍验证实际包。
        env = dict(os.environ, GOWORK="off", GOTOOLCHAIN="local", GOMODCACHE=directory + "/modules",
                   GOPROXY="direct", GOPRIVATE="", GONOPROXY="", GONOSUMDB="",
                   GOSUMDB="sum.golang.org")
        downloaded = download_module(go_executable, module, version, directory, env)
        if (downloaded.get("Error") or downloaded.get("Path") != module
                or downloaded.get("Version") != version
                or not downloaded.get("Sum") or not downloaded.get("GoModSum")
                or downloaded.get("Origin", {}).get("Hash") != candidate):
            raise ReleaseError("Downloaded module does not match the published tag and commit")
        if not Path(downloaded["Zip"]).is_file() or Path(downloaded["GoMod"]).read_text(encoding="utf-8") != go_mod:
            raise ReleaseError("Published module archive or go.mod is inconsistent")
    return {"version": version, "candidate": candidate, "module": module,
            "sum": downloaded["Sum"], "go_mod_sum": downloaded["GoModSum"],
            "state": "published-and-verified"}


def validate_version(version, base, changes):
    target, previous = version_tuple(version), version_tuple(base)
    if target <= previous:
        raise ReleaseError("Release version must be newer than the latest stable version")
    if any(type(changes.get(key)) is not bool for key in ("has_breaking", "has_compatible")):
        raise ReleaseError("API change classification is missing; refresh API verification")
    if changes["has_breaking"]:
        # v0 patch 兼容是本项目的承诺，SemVer 本身未对 v0 作此保证。
        required_bump = target[:2] > previous[:2] if previous[0] == 0 else target[0] > previous[0]
        if not required_bump:
            raise ReleaseError("Breaking API changes require a minor bump in v0 or a major bump in v1+")
    elif changes["has_compatible"] and previous[0] >= 1 and target[:2] <= previous[:2]:
        raise ReleaseError("Compatible API additions require a minor or major version bump")


def refresh(repo, filename, candidate, main_sha, release_version=""):
    if candidate != main_sha:
        return "Select the current main commit for fresh verification; historical evidence cannot be substituted"
    args = ["gh", "workflow", "run", filename, "--ref", "main", "--repo", repo]
    if release_version:
        args.extend(["-f", "release_version=" + release_version])
    command(*args)
    return "Fresh exact-SHA verification dispatched; wait for completion and check again"


def read_receipt(repo, run, candidate):
    artifacts = api(f"repos/{repo}/actions/runs/{run['id']}/artifacts?per_page=100")["artifacts"]
    if not any(item["name"].startswith("verification-inputs-") and not item["expired"] for item in artifacts):
        raise ReleaseError("Verification artifacts are missing or expired")
    with tempfile.TemporaryDirectory(prefix="toolkit-release-") as directory:
        try:
            command("gh", "run", "download", str(run["id"]), "--repo", repo,
                    "--pattern", "verification-inputs-*", "--dir", directory)
        except subprocess.CalledProcessError as error:
            raise ReleaseError("Verification artifacts could not be downloaded; check access or refresh verification") from error
        records = [json.loads(path.read_text()) for path in Path(directory).rglob("*-inputs.json")]
    if not records:
        raise ReleaseError("Verification receipts are missing")
    for record in records:
        if (record.get("candidate_sha") != candidate or record.get("event_sha") != candidate
                or record.get("run_id") != str(run["id"]) or record.get("event") != run["event"]
                or record.get("platform") != "linux"):
            raise ReleaseError("Verification receipt does not match its exact-SHA run")
    return max(records, key=lambda item: int(item["run_attempt"]))


def qualify(repo, version, candidate, refs, prepare):
    comparison = api(f"repos/{repo}/compare/{candidate}...main")
    if comparison["status"] not in ("ahead", "identical"):
        raise ReleaseError("Candidate is not in main history")
    main_sha = api(f"repos/{repo}/branches/main")["commit"]["sha"]
    stable = [item["ref"].removeprefix("refs/tags/") for item in refs
              if re.fullmatch(r"refs/tags/v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)", item["ref"])]
    if not stable:
        raise ReleaseError("An initial stable API baseline is required")
    base = max(stable, key=version_tuple)
    if version_tuple(version) <= version_tuple(base):
        raise ReleaseError("Release version must be newer than the latest stable version")
    major = version_tuple(version)[0]
    explicit_version = version if major > version_tuple(base)[0] else ""
    module = re.search(r'(?m)^module\s+"?([^"\s]+)', source(candidate, "go.mod"))[1]
    suffix = re.search(r"/v([0-9]+)$", module)
    if (major >= 2 and (not suffix or int(suffix[1]) != major)
            or major < 2 and suffix is not None):
        raise ReleaseError("Candidate module path does not match the release major version")
    base_sha = tag_commit(repo, base)
    workflow = source(candidate, ".github/workflows/api-compat.yml")
    baseline_path = re.search(r'API_BREAKING_BASELINE:\s*"([^"]+)"', workflow)[1]
    baseline = subprocess.check_output(["git", "show", candidate + ":" + baseline_path])
    expected_api = {"base_tag": base, "base_sha": base_sha,
                    "baseline_sha256": hashlib.sha256(baseline).hexdigest(),
                    "gorelease_version": re.search(r"GORELEASE_VERSION:\s*(\S+)", workflow)[1]}
    pending, receipts = [], []
    for filename in ("ci.yml", "api-compat.yml"):
        runs = api(f"repos/{repo}/actions/workflows/{filename}/runs?head_sha={candidate}&per_page=100")["workflow_runs"]
        runs = [run for run in runs if run["head_branch"] == "main"
                and run["event"] in ("push", "workflow_dispatch")
                and run["head_repository"]["full_name"] == repo]
        run = max(runs, key=lambda item: item["id"]) if runs else None
        reason = None
        if run is None:
            reason = "Missing exact-SHA verification"
        elif run["status"] != "completed":
            pending.append(filename + ": " + run["status"] + " " + run["html_url"])
            continue
        elif run["conclusion"] != "success":
            reason = "Latest verification failed: " + str(run["conclusion"]) + " " + run["html_url"]
        else:
            try:
                record = read_receipt(repo, run, candidate)
                if filename == "api-compat.yml":
                    if any(record.get(key) != value for key, value in expected_api.items()):
                        raise ReleaseError("API comparison inputs changed")
                    checked_version = record.get("release_version", "")
                    if (explicit_version or checked_version) and checked_version != version:
                        raise ReleaseError("API comparison release version does not match the candidate")
                    changes = record.get("api_changes", {})
                    if any(type(changes.get(key)) is not bool for key in ("has_breaking", "has_compatible")):
                        raise ReleaseError("API change classification is missing")
                    validate_version(version, base, changes)
            except ReleaseError as error:
                # 版本与 API 不匹配不能靠重跑解决；证据缺失才提供补验路径。
                if str(error).startswith(("Breaking API", "Compatible API")):
                    raise
                reason = str(error)
        if reason:
            if prepare:
                requested_version = explicit_version if filename == "api-compat.yml" else ""
                reason += "; " + refresh(repo, filename, candidate, main_sha, requested_version)
            pending.append(filename + ": " + reason)
        else:
            receipts.append({"workflow": filename, "run_id": run["id"], "url": run["html_url"]})
    if pending:
        raise ReleaseError("Release is not ready:\n" + "\n".join(pending))
    # 只比较 tag 的实际对象，检查期间发生新发布时必须重新核对 API 基线。
    current = api(f"repos/{repo}/git/matching-refs/tags/")
    if {item["ref"]: item["object"]["sha"] for item in current} != {
            item["ref"]: item["object"]["sha"] for item in refs}:
        raise ReleaseError("Published tags changed during verification; check again")
    return {"version": version, "candidate": candidate, "base": base, "runs": receipts, "state": "ready"}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("version")
    parser.add_argument("--candidate", help="Expected commit; defaults to existing tag or HEAD")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--prepare", action="store_true", help="Dispatch fresh missing or failed main checks")
    mode.add_argument("--publish", action="store_true", help="Create or resume pushing the verified tag")
    mode.add_argument("--verify-published", action="store_true", help="Verify an existing published module")
    args = parser.parse_args()
    version_tuple(args.version)
    os.chdir(Path(__file__).resolve().parent.parent)
    repo = json.loads(command("gh", "repo", "view", "--json", "nameWithOwner"))["nameWithOwner"]
    refs = api(f"repos/{repo}/git/matching-refs/tags/")
    remote_exists = any(item["ref"] == "refs/tags/" + args.version for item in refs)
    local_sha = local_tag_commit(args.version)
    remote_sha = tag_commit(repo, args.version) if remote_exists else None
    candidate = (command("git", "rev-parse", args.candidate + "^{commit}") if args.candidate
                 else remote_sha or local_sha or command("git", "rev-parse", "HEAD"))
    if (remote_sha and remote_sha != candidate) or (local_sha and local_sha != candidate):
        raise ReleaseError("Existing release tag points to a different commit; tags are immutable")
    if remote_sha:
        print(json.dumps(verify_published(repo, args.version, candidate), indent=2))
        return
    if args.verify_published:
        raise ReleaseError("Release tag is not published")
    result = qualify(repo, args.version, candidate, refs, args.prepare)
    if args.publish:
        # 工具链不可用时在创建或续推不可变 tag 之前失败。
        select_go_toolchain(source(candidate, "go.mod"))
    print(json.dumps(result, indent=2))
    if args.publish:
        if not local_sha:
            with tempfile.NamedTemporaryFile(mode="w", prefix="toolkit-tag-") as stream:
                stream.write(f"发布 {args.version}\n")
                stream.flush()
                subprocess.run(["git", "tag", "-a", args.version, candidate, "-F", stream.name], check=True)
        # 上次 push 中断后复用同一 SHA 的本地 tag，不删除、移动或重建。
        subprocess.run(["git", "push", "origin", "refs/tags/" + args.version], check=True)
        if tag_commit(repo, args.version) != candidate:
            raise ReleaseError("Remote tag does not match the verified candidate")
        print(json.dumps(verify_published(repo, args.version, candidate), indent=2))


if __name__ == "__main__":
    try:
        main()
    except (ReleaseError, subprocess.CalledProcessError) as error:
        raise SystemExit(str(error)) from error
