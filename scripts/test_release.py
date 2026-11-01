#!/usr/bin/env python3
"""验证发布状态与版本承诺；GitHub、Git 和 Go 的外部写入均隔离。"""

from contextlib import ExitStack, redirect_stdout
import base64
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("toolkit_release", Path(__file__).with_name("release.py"))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)

REPO = "example/toolkit"
CANDIDATE = "a" * 40
BASE_SHA = "b" * 40
OTHER_SHA = "c" * 40
GO_MOD = "module example/toolkit\n\ngo 1.26.10\n"


def tag_ref(version, sha):
    return {"ref": "refs/tags/" + version, "object": {"type": "commit", "sha": sha}}


class ReleaseStateTests(unittest.TestCase):
    def main_boundary(self, version, local=None, remote=None, candidate=None):
        stack = ExitStack()
        self.addCleanup(stack.close)
        stack.enter_context(patch.object(release.os, "chdir"))
        stack.enter_context(redirect_stdout(io.StringIO()))
        refs = [tag_ref("v0.9.0", OTHER_SHA)]
        if remote:
            refs.append(tag_ref(version, remote))
        stack.enter_context(patch.object(release, "api", return_value=refs))
        stack.enter_context(patch.object(release, "local_tag_commit", return_value=local))
        stack.enter_context(patch.object(release, "tag_commit", return_value=remote or candidate))

        def command(*args, **kwargs):
            if args == ("gh", "repo", "view", "--json", "nameWithOwner"):
                return json.dumps({"nameWithOwner": REPO})
            if args[:2] == ("git", "rev-parse"):
                return candidate or CANDIDATE
            raise AssertionError("Unexpected external command: " + repr(args))

        stack.enter_context(patch.object(release, "command", side_effect=command))
        stack.enter_context(patch.object(release, "source", return_value=GO_MOD))
        stack.enter_context(patch.object(release, "select_go_toolchain", return_value=Path("/sdk/go/bin/go")))
        qualify = stack.enter_context(patch.object(release, "qualify", return_value={"state": "ready"}))
        verify = stack.enter_context(patch.object(release, "verify_published", return_value={"state": "published-and-verified"}))
        writes = stack.enter_context(patch.object(release.subprocess, "run"))
        return qualify, verify, writes

    def invoke(self, *args):
        with patch("sys.argv", ["release.py", *args]):
            release.main()

    def test_historical_published_tag_ignores_newer_release_and_ci_artifacts(self):
        qualify, verify, writes = self.main_boundary("v0.2.0", remote=CANDIDATE)
        self.invoke("v0.2.0")
        self.invoke("v0.2.0", "--publish")
        qualify.assert_not_called()
        self.assertEqual(verify.call_count, 2)
        verify.assert_called_with(REPO, "v0.2.0", CANDIDATE)
        writes.assert_not_called()

    def test_same_sha_local_tag_resumes_push_without_recreating_tag(self):
        qualify, verify, writes = self.main_boundary("v0.10.0", local=CANDIDATE, candidate=CANDIDATE)
        self.invoke("v0.10.0", "--publish")
        qualify.assert_called_once()
        writes.assert_called_once_with(["git", "push", "origin", "refs/tags/v0.10.0"], check=True)
        verify.assert_called_once_with(REPO, "v0.10.0", CANDIDATE)

    def test_interrupted_push_can_retry_same_local_tag(self):
        _, verify, writes = self.main_boundary("v0.10.0", local=CANDIDATE, candidate=CANDIDATE)
        writes.side_effect = [subprocess.CalledProcessError(1, "git push"), None]
        with self.assertRaises(subprocess.CalledProcessError):
            self.invoke("v0.10.0", "--publish")
        self.invoke("v0.10.0", "--publish")
        self.assertEqual(writes.call_count, 2)
        self.assertTrue(all(call.args[0][1] == "push" for call in writes.call_args_list))
        verify.assert_called_once()

    def test_different_sha_local_or_remote_tag_refuses_all_writes(self):
        for state in ({"local": OTHER_SHA}, {"remote": OTHER_SHA}):
            with self.subTest(state=state):
                qualify, verify, writes = self.main_boundary("v0.10.0", candidate=CANDIDATE, **state)
                with self.assertRaisesRegex(release.ReleaseError, "tags are immutable"):
                    self.invoke("v0.10.0", "--candidate", CANDIDATE, "--publish")
                qualify.assert_not_called()
                verify.assert_not_called()
                writes.assert_not_called()

    def test_verify_unpublished_version_never_creates_tag(self):
        qualify, verify, writes = self.main_boundary("v0.10.0")
        with self.assertRaisesRegex(release.ReleaseError, "not published"):
            self.invoke("v0.10.0", "--verify-published")
        qualify.assert_not_called()
        verify.assert_not_called()
        writes.assert_not_called()

    def test_toolchain_failure_refuses_new_tag_and_interrupted_push_resume(self):
        for local in (None, CANDIDATE):
            with self.subTest(local_tag=local):
                _, verify, writes = self.main_boundary("v0.10.0", local=local, candidate=CANDIDATE)
                with patch.object(release, "select_go_toolchain", side_effect=release.ReleaseError("Go version is too old")):
                    with self.assertRaisesRegex(release.ReleaseError, "Go version is too old"):
                        self.invoke("v0.10.0", "--publish")
                writes.assert_not_called()
                verify.assert_not_called()


class PublishedModuleTests(unittest.TestCase):
    def test_target_module_selects_toolchain_and_respects_auto_or_local(self):
        target = "module example/historical\n\ngo 1.25.0\n"
        for mode, usable in (("auto", True), ("local", True), ("local", False)):
            with self.subTest(mode=mode, usable=usable), tempfile.TemporaryDirectory() as directory:
                selected_root = Path(directory) / "selected-go"
                observed = []

                def go_process(args, **kwargs):
                    observed.append(args)
                    self.assertEqual(Path(kwargs["cwd"], "go.mod").read_text(encoding="utf-8"), target)
                    self.assertEqual(kwargs["env"]["GOWORK"], "off")
                    if args[1:] == ("env", "GOROOT"):
                        self.assertEqual(kwargs["env"]["GOTOOLCHAIN"], mode)
                        return str(selected_root) + "\n"
                    self.assertEqual(kwargs["env"]["GOTOOLCHAIN"], "local")
                    if not usable:
                        raise subprocess.CalledProcessError(1, args, stderr="go.mod requires go >= 1.25.0")
                    return "example/historical\n"

                with patch.dict(release.os.environ, {"GOTOOLCHAIN": mode, "GOWORK": "unrelated.work"}), \
                        patch.object(release.subprocess, "check_output", side_effect=go_process):
                    if usable:
                        executable = release.select_go_toolchain(target)
                        self.assertEqual(executable.parent.parent, selected_root)
                    else:
                        with self.assertRaisesRegex(release.ReleaseError, "requires go >= 1.25.0"):
                            release.select_go_toolchain(target)
                self.assertEqual(len(observed), 2)

    def test_published_verification_uses_target_toolchain_and_validates_download_metadata(self):
        target = "module example/historical\n\ngo 1.25.0\n// 目标模块\n"
        metadata = {"content": base64.b64encode(target.encode()).decode()}
        with tempfile.TemporaryDirectory() as directory:
            selected_root = Path(directory) / "selected-go"

            def go_process(args, **kwargs):
                self.assertEqual(Path(kwargs["cwd"], "go.mod").read_text(encoding="utf-8"), target)
                if args[1:] == ("env", "GOROOT"):
                    self.assertEqual(kwargs["env"]["GOTOOLCHAIN"], "auto")
                    return str(selected_root)
                return "example/historical"

            def download(args, **kwargs):
                self.assertEqual(Path(args[0]).parent.parent, selected_root)
                self.assertEqual(args[1:], ["mod", "download", "-json", "example/historical@v0.2.0"])
                self.assertEqual(kwargs["env"]["GOTOOLCHAIN"], "local")
                archive = Path(kwargs["cwd"], "module.zip")
                manifest = Path(kwargs["cwd"], "module.mod")
                archive.write_bytes(b"downloaded archive fixture")
                manifest.write_text(target, encoding="utf-8")
                body = {"Path": "example/historical", "Version": "v0.2.0", "Sum": "h1:archive",
                        "GoModSum": "h1:manifest", "Origin": {"Hash": CANDIDATE},
                        "Zip": str(archive), "GoMod": str(manifest)}
                return subprocess.CompletedProcess(args, 0, json.dumps(body), "")

            with patch.dict(release.os.environ, {"GOTOOLCHAIN": "auto"}), \
                    patch.object(release, "api", return_value=metadata) as api, \
                    patch.object(release.subprocess, "check_output", side_effect=go_process), \
                    patch.object(release.subprocess, "run", side_effect=download):
                verified = release.verify_published(REPO, "v0.2.0", CANDIDATE)
            api.assert_called_once_with(f"repos/{REPO}/contents/go.mod?ref={CANDIDATE}")
            self.assertEqual(verified["state"], "published-and-verified")
            self.assertEqual(verified["candidate"], CANDIDATE)

    def test_download_preserves_json_error_and_non_json_stderr(self):
        cases = [
            (1, '{"Error":"requires Go 1.26.10"}', "secondary error", "requires Go 1.26.10"),
            (1, "", "checksum service unavailable", "checksum service unavailable"),
            (1, "{}", "download interrupted", "download interrupted"),
            (0, '{"Path":"example/toolkit","Version":"v0.3.7"}', "", None),
        ]
        for code, stdout, stderr, expected_error in cases:
            with self.subTest(code=code, stdout=stdout, stderr=stderr):
                completed = subprocess.CompletedProcess([], code, stdout, stderr)
                with patch.object(release.subprocess, "run", return_value=completed):
                    if expected_error:
                        with self.assertRaisesRegex(release.ReleaseError, expected_error):
                            release.download_module("go", "example/toolkit", "v0.3.7", ".", {})
                    else:
                        self.assertEqual(release.download_module("go", "example/toolkit", "v0.3.7", ".", {})["Version"], "v0.3.7")


class ReleaseEvidenceTests(unittest.TestCase):
    def test_major_receipt_binds_candidate_version_and_patch_keeps_default_mode(self):
        cases = [
            ("v1.2.3", "v2.0.0", "example/toolkit/v2", "v2.0.0", True),
            ("v1.2.3", "v2.0.0", "example/toolkit/v2", "", False),
            ("v1.2.3", "v2.0.0", "example/toolkit/v2", "v2.1.0", False),
            ("v0.3.6", "v1.0.0", "example/toolkit", "v1.0.0", True),
            ("v0.3.6", "v0.3.7", "example/toolkit", "", True),
            ("v0.3.6", "v0.3.7", "example/toolkit", "v0.3.7", True),
            ("v0.3.6", "v0.3.7", "example/toolkit", "v0.3.8", False),
        ]
        for base, target, module, checked_version, accepted in cases:
            with self.subTest(base=base, target=target, checked_version=checked_version):
                refs = [tag_ref(base, BASE_SHA)]
                dispatched = []
                baseline = b"approved API fixture\n"
                workflow = 'API_BREAKING_BASELINE: "approved.txt"\nGORELEASE_VERSION: pinned\n'
                api_receipt = {"base_tag": base, "base_sha": BASE_SHA,
                               "baseline_sha256": hashlib.sha256(baseline).hexdigest(),
                               "gorelease_version": "pinned", "release_version": checked_version,
                               "api_changes": {"has_breaking": False, "has_compatible": False}}

                def api(path):
                    if "/compare/" in path:
                        return {"status": "identical"}
                    if path.endswith("/branches/main"):
                        return {"commit": {"sha": CANDIDATE}}
                    if "/actions/workflows/" in path:
                        run_id = 101 if "/workflows/ci.yml/" in path else 102
                        return {"workflow_runs": [{"id": run_id, "head_branch": "main", "event": "push",
                                                  "head_repository": {"full_name": REPO},
                                                  "status": "completed", "conclusion": "success",
                                                  "html_url": "https://example.invalid/run/" + str(run_id)}]}
                    if path.endswith("/git/matching-refs/tags/"):
                        return refs
                    raise AssertionError("Unexpected API request: " + path)

                def source(candidate, path):
                    if path == "go.mod":
                        return "module " + module + "\n\ngo 1.26.10\n"
                    return workflow

                def command(*args, **kwargs):
                    self.assertEqual(args[:3], ("gh", "workflow", "run"))
                    dispatched.append(args)
                    return ""

                with patch.object(release, "api", side_effect=api), \
                        patch.object(release, "tag_commit", return_value=BASE_SHA), \
                        patch.object(release, "source", side_effect=source), \
                        patch.object(release.subprocess, "check_output", return_value=baseline), \
                        patch.object(release, "read_receipt", side_effect=lambda repo, run, candidate: api_receipt if run["id"] == 102 else {}), \
                        patch.object(release, "command", side_effect=command):
                    if accepted:
                        result = release.qualify(REPO, target, CANDIDATE, refs, prepare=True)
                        self.assertEqual(result["state"], "ready")
                        self.assertEqual(dispatched, [])
                    else:
                        with self.assertRaisesRegex(release.ReleaseError, "release version does not match"):
                            release.qualify(REPO, target, CANDIDATE, refs, prepare=True)
                        self.assertEqual(len(dispatched), 1)
                        self.assertEqual(dispatched[0][3], "api-compat.yml")
                        if target.startswith("v2."):
                            self.assertEqual(dispatched[0][-2:], ("-f", "release_version=" + target))
                        else:
                            self.assertNotIn("-f", dispatched[0])

    def qualify_with_artifacts(self, artifacts, main_sha, prepare,
                               version="v0.3.7", module="example/toolkit"):
        refs = [tag_ref("v0.3.6", BASE_SHA)]
        dispatched = []

        def api(path):
            if "/compare/" in path:
                return {"status": "ahead" if main_sha != CANDIDATE else "identical"}
            if path.endswith("/branches/main"):
                return {"commit": {"sha": main_sha}}
            if "/actions/workflows/" in path:
                return {"workflow_runs": [{"id": 42, "head_branch": "main", "event": "push",
                                          "head_repository": {"full_name": REPO},
                                          "status": "completed", "conclusion": "success",
                                          "html_url": "https://example.invalid/run/42"}]}
            if "/artifacts?" in path:
                return {"artifacts": artifacts}
            raise AssertionError("Unexpected API request: " + path)

        def command(*args, **kwargs):
            # 产物不可用时只能发起补验，不能尝试下载不存在的产物。
            if args[:3] != ("gh", "workflow", "run"):
                raise AssertionError("Unexpected external command: " + repr(args))
            dispatched.append(args)
            return ""

        workflow = 'API_BREAKING_BASELINE: "approved.txt"\nGORELEASE_VERSION: pinned\n'

        def source(candidate, path):
            if path == "go.mod":
                return "module " + module + "\n\ngo 1.26.10\n"
            if path == ".github/workflows/api-compat.yml":
                return workflow
            raise AssertionError("Unexpected source path: " + path)

        with patch.object(release, "api", side_effect=api), \
                patch.object(release, "tag_commit", return_value=BASE_SHA), \
                patch.object(release, "source", side_effect=source), \
                patch.object(release.subprocess, "check_output", return_value=b"baseline\n"), \
                patch.object(release, "command", side_effect=command):
            with self.assertRaises(release.ReleaseError) as failed:
                release.qualify(REPO, version, CANDIDATE, refs, prepare)
        return str(failed.exception), dispatched

    def test_missing_or_expired_artifacts_dispatch_fresh_main_verification(self):
        for artifacts in ([], [{"name": "verification-inputs-1", "expired": True}]):
            with self.subTest(artifacts=artifacts):
                error, dispatched = self.qualify_with_artifacts(artifacts, CANDIDATE, prepare=True)
                self.assertIn("missing or expired", error)
                self.assertIn("wait for completion", error)
                self.assertEqual({call[3] for call in dispatched}, {"ci.yml", "api-compat.yml"})
                self.assertTrue(all(call[4:6] == ("--ref", "main") for call in dispatched))

    def test_historical_candidate_never_substitutes_new_main_evidence(self):
        error, dispatched = self.qualify_with_artifacts([], OTHER_SHA, prepare=True)
        self.assertIn("historical evidence cannot be substituted", error)
        self.assertEqual(dispatched, [])

    def test_check_only_does_not_dispatch_for_missing_evidence(self):
        error, dispatched = self.qualify_with_artifacts([], CANDIDATE, prepare=False)
        self.assertIn("Release is not ready", error)
        self.assertEqual(dispatched, [])

    def test_module_major_mismatch_is_rejected_before_dispatch(self):
        cases = [
            ("v2.0.0", "example/toolkit", False),
            ("v2.0.0", "example/toolkit/v3", False),
            ("v1.0.0", "example/toolkit/v2", False),
            ("v0.3.7", "example/toolkit", True),
            ("v1.0.0", "example/toolkit", True),
            ("v2.0.0", "example/toolkit/v2", True),
            ("v2.0.0", '"example/toolkit/v2"', True),
        ]
        for version, module, accepted in cases:
            with self.subTest(version=version, module=module):
                error, dispatched = self.qualify_with_artifacts(
                    [], CANDIDATE, prepare=True, version=version, module=module)
                if accepted:
                    self.assertIn("missing or expired", error)
                    self.assertEqual(len(dispatched), 2)
                else:
                    self.assertIn("module path does not match", error)
                    self.assertEqual(dispatched, [])


class ReleaseVersionTests(unittest.TestCase):
    def test_checker_rejects_package_load_errors_before_approving_api_changes(self):
        checker = Path(__file__).resolve().parents[1] / ".github/workflows/check-api-breaking.py"
        breaking = "## incompatible changes\nOld: removed\n"
        approved = "# example/toolkit\n" + breaking
        cases = [
            ("base", breaking, "\n"),
            ("release", breaking, "\r\n"),
            ("base", "", "\r\n"),
            ("release", "", "\n"),
        ]
        for version, changes, newline in cases:
            with self.subTest(version=version, changes=changes, newline=newline), \
                    tempfile.TemporaryDirectory(prefix="toolkit-api-load-errors-") as directory:
                report = Path(directory) / "report.txt"
                baseline = Path(directory) / "baseline.txt"
                output = Path(directory) / "changes.json"
                # 已批准的变更和空变更都不能覆盖包加载失败。
                body = (f"# example/toolkit\n## errors in {version} version:\n"
                        "bad.go:3:15: undefined: MissingType\n\n" + changes +
                        "\n# summary\nv2.0.0 is a valid semantic version for this release.\n")
                report.write_bytes(body.replace("\n", newline).encode())
                baseline.write_text(approved, encoding="utf-8")
                result = subprocess.run(
                    [sys.executable, "-B", str(checker), str(report), str(baseline),
                     "--allow-empty", "--json-output", str(output)],
                    text=True, capture_output=True, check=False)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn(f"errors in {version} version", result.stderr)
                self.assertFalse(output.exists(), "Failed comparison must not produce API classification")

    def test_checker_cli_classifies_api_changes_before_version_validation(self):
        checker = Path(__file__).resolve().parents[1] / ".github/workflows/check-api-breaking.py"
        approved = "# example/toolkit\n## incompatible changes\nOld: removed\n"
        cases = [
            ("unchanged", "# summary\nNo API changes.\n", approved,
             {"has_breaking": False, "has_compatible": False}),
            ("compatible", "# example/toolkit\n## compatible changes\nNew: added\n", approved,
             {"has_breaking": False, "has_compatible": True}),
            ("approved-breaking", approved, approved,
             {"has_breaking": True, "has_compatible": False}),
            ("unapproved-breaking", "# example/toolkit\n## incompatible changes\nOther: removed\n",
             approved, None),
        ]
        for name, report_text, baseline_text, expected in cases:
            with self.subTest(name=name), tempfile.TemporaryDirectory(prefix="toolkit-api-classification-") as directory:
                report = Path(directory) / "report.txt"
                baseline = Path(directory) / "baseline.txt"
                output = Path(directory) / "changes.json"
                report.write_text(report_text, encoding="utf-8")
                baseline.write_text(baseline_text, encoding="utf-8")
                result = subprocess.run(
                    [sys.executable, "-B", str(checker), str(report), str(baseline),
                     "--allow-empty", "--json-output", str(output)],
                    text=True, capture_output=True, check=False)
                if expected is None:
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("Unapproved breaking API changes", result.stderr)
                    self.assertFalse(output.exists())
                else:
                    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertEqual(json.loads(output.read_text()), expected)

    def test_api_classification_controls_candidate_version(self):
        cases = [
            ("v0.3.7", "v0.3.6", True, False, False),
            ("v0.4.0", "v0.3.6", True, False, True),
            ("v0.3.7", "v0.3.6", False, True, True),
            ("v1.2.4", "v1.2.3", False, True, False),
            ("v1.3.0", "v1.2.3", False, True, True),
            ("v1.3.0", "v1.2.3", True, True, False),
            ("v2.0.0", "v1.2.3", True, False, True),
            ("v1.2.4", "v1.2.3", False, False, True),
        ]
        for target, base, breaking, compatible, accepted in cases:
            with self.subTest(target=target, base=base, breaking=breaking, compatible=compatible):
                changes = {"has_breaking": breaking, "has_compatible": compatible}
                if accepted:
                    release.validate_version(target, base, changes)
                else:
                    with self.assertRaises(release.ReleaseError):
                        release.validate_version(target, base, changes)

    def test_missing_classification_and_nonadvancing_versions_are_rejected(self):
        with self.assertRaisesRegex(release.ReleaseError, "classification is missing"):
            release.validate_version("v0.3.7", "v0.3.6", {})
        for target in ("v0.3.6", "v0.3.5"):
            with self.subTest(target=target), self.assertRaisesRegex(release.ReleaseError, "newer"):
                release.validate_version(target, "v0.3.6", {"has_breaking": False, "has_compatible": False})


if __name__ == "__main__":
    unittest.main()
