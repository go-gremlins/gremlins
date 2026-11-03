"""Behavioral tests for the repository's lint baseline gate."""

import collections
import contextlib
import io
import json
from pathlib import Path
import subprocess
import shutil
import sys
import tempfile
import unittest
from unittest import mock

import lint


class BaselineTests(unittest.TestCase):
    def setUp(self):
        self.a = ("revive", "cmd/a.go", "a" * 64)
        self.b = ("revive", "cmd/a.go", "b" * 64)

    def test_accepts_existing_debt_and_shrink(self):
        lint.compare({self.a: 2}, {self.a: 2}, {self.a: 3})
        lint.compare({}, {}, {self.a: 3})

    def test_rejects_new_growth_replacement_and_stale_debt(self):
        cases = [
            ({self.a: 1}, {}, {}),
            ({self.a: 2}, {self.a: 2}, {self.a: 1}),
            ({self.b: 1}, {self.b: 1}, {self.a: 1}),
            ({}, {self.a: 1}, {self.a: 1}),
        ]
        for actual, candidate, target in cases:
            with self.subTest(actual=actual, candidate=candidate, target=target):
                with self.assertRaises(lint.LintError):
                    lint.compare(actual, candidate, target)

    def test_baseline_round_trip(self):
        findings = collections.Counter({self.a: 2})
        self.assertEqual(lint.parse_baseline(lint.serialize(findings)), findings)

    def test_rejects_invalid_baseline_data(self):
        valid = json.loads(lint.serialize({self.a: 1}))
        entry = valid["findings"][0]
        cases = [
            b'{"version":1,"version":1,"findings":[]}',
            b'{"version":true,"findings":[]}',
            b'{"version":1,"findings":null}',
            b'{"version":1,"findings":[],"extra":1}',
            json.dumps({"version": 1, "findings": [entry, entry]}).encode(),
        ]
        for field, value in [("count", 0), ("count", True), ("count", 1.5),
                             ("file", "../a.go"), ("file", "/a.go"),
                             ("fingerprint", "x"), ("linter", "typecheck")]:
            cases.append(json.dumps({"version": 1, "findings": [{**entry, field: value}]}).encode())
        for data in cases:
            with self.subTest(data=data):
                with self.assertRaises(lint.LintError):
                    lint.parse_baseline(data)


class ReportTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parent.parent / ".git")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "a.go").write_text("package a\n\nfunc f() {\n    bad()\n}\n", encoding="utf-8")

    def report(self, line=4, text="rule: bad call", name="a.go", linter="revive"):
        return {"Issues": [{"FromLinter": linter, "Text": text,
                            "Pos": {"Filename": name, "Line": line, "Column": 1}}],
                "Report": {"Warnings": [], "Error": ""}}

    def test_line_movement_and_indentation_do_not_add_debt(self):
        before = lint.parse_report(json.dumps(self.report()).encode(), self.root)
        (self.root / "a.go").write_text("package a\n\n\nfunc f() {\n\tbad()\n}\n", encoding="utf-8")
        after = lint.parse_report(json.dumps(self.report(line=5)).encode(), self.root)
        self.assertEqual(before, after)

    def test_different_rule_and_context_cannot_replace_old_finding(self):
        before = lint.parse_report(json.dumps(self.report()).encode(), self.root)
        for report in [self.report(text="other: new error"), self.report(linter="gosec")]:
            after = lint.parse_report(json.dumps(report).encode(), self.root)
            with self.assertRaises(lint.LintError):
                lint.compare(after, after, before)
        (self.root / "a.go").write_text("package a\n\nfunc g() {\n    bad()\n}\n", encoding="utf-8")
        after = lint.parse_report(json.dumps(self.report()).encode(), self.root)
        with self.assertRaises(lint.LintError):
            lint.compare(after, after, before)

    def test_occurrences_are_counted_without_duplicating_platforms(self):
        report = self.report()
        report["Issues"] *= 2
        result = lint.parse_report(json.dumps(report).encode(), self.root)
        self.assertEqual(list(result.values()), [2])
        self.assertEqual(list((result | result).values()), [2])

    def test_identical_context_in_another_declaration_cannot_replace_debt(self):
        for first, second in [("func first()", "func second()"),
                              ("func (a *A[T]) Same()", "func (b *B[T]) Same()")]:
            with self.subTest(first=first, second=second):
                block = "{\n a()\n a()\n a()\n CALL\n a()\n a()\n a()\n}\n"
                before_source = "package a\n" + first + block.replace("CALL", "bad()") + second + block.replace("CALL", "good()")
                after_source = "package a\n" + first + block.replace("CALL", "good()") + second + block.replace("CALL", "bad()")
                (self.root / "a.go").write_text(before_source, encoding="utf-8")
                before = lint.parse_report(json.dumps(self.report(line=before_source.splitlines().index(" bad()") + 1)).encode(), self.root)
                (self.root / "a.go").write_text(after_source, encoding="utf-8")
                after = lint.parse_report(json.dumps(self.report(line=after_source.splitlines().index(" bad()") + 1)).encode(), self.root)
                with self.assertRaises(lint.LintError):
                    lint.compare(after, before, before)


    def test_invalid_or_incomplete_reports_fail(self):
        cases = [{}, {"Issues": []}, {"Issues": [], "Report": None},
                 self.report(line=100), self.report(line=True),
                 self.report(name="../a.go"), self.report(name="/a.go"),
                 self.report(linter="typecheck")]
        for field, value in [("Warnings", ["analysis warning"]), ("Error", "failure")]:
            report = self.report()
            report["Report"][field] = value
            cases.append(report)
        for report in cases:
            with self.subTest(report=report):
                data = json.dumps(report).encode()
                with self.assertRaises(lint.LintError):
                    lint.parse_report(data, self.root)

    def test_symlink_input_is_rejected(self):
        (self.root / "linked.go").symlink_to(self.root / "a.go")
        data = json.dumps(self.report(name="linked.go")).encode()
        with self.assertRaises(lint.LintError):
            lint.parse_report(data, self.root)

    def test_checkout_scan_rejects_linked_source_before_analysis(self):
        (self.root / "linked.go").symlink_to(self.root / "a.go")
        with self.assertRaises(lint.LintError):
            lint.validate_checkout(self.root)

    def test_build_environment_rejects_ambient_overrides(self):
        env = lint.analysis_env({"PATH": "/bin", "GOFLAGS": "-toolexec=bad",
                                 "GOWORK": "/outside/go.work", "GOTOOLCHAIN": "auto",
                                 "GOENV": "/outside/env", "GOPROXY": "https://outside"})
        self.assertEqual(env["GOFLAGS"], "")
        self.assertEqual(env["GOWORK"], "off")
        self.assertEqual(env["GOTOOLCHAIN"], "local")
        self.assertEqual(env["GOENV"], "off")
        self.assertEqual(env["GOPROXY"], "off")
        self.assertEqual(env["CGO_ENABLED"], "0")

    def test_parser_uses_the_host_and_an_empty_standalone_build_path(self):
        def inspect_parser(root, args, env, **kwargs):
            self.assertNotIn("GOARCH", tuple(env))
            self.assertNotIn("GOOS", tuple(env))
            self.assertEqual(env["GO111MODULE"], "off")
            self.assertEqual(list(Path(env["GOPATH"]).iterdir()), [])
            self.assertEqual(Path(args[-1]), lint.REPOSITORY / lint.CONTEXT_TOOL / "main.go")
            return b'{"a.go":[]}'
        with mock.patch.object(lint, "command", side_effect=inspect_parser):
            self.assertEqual(lint.source_declarations({"a.go": "package a\n"}), {"a.go": []})

    def test_analysis_commands_do_not_inherit_credentials(self):
        env = lint.analysis_env({"PATH": "/bin", "HOME": str(self.root),
                                 "EXAMPLE_API_TOKEN": "synthetic-credential", "GOCACHE": str(self.root / "cache")})
        self.assertNotIn("EXAMPLE_API_TOKEN", tuple(env))
        self.assertEqual(env["PATH"], "/bin")
        self.assertEqual(env["GOCACHE"], str(self.root / "cache"))

    def test_windows_tool_and_cache_paths_are_preserved(self):
        paths = {"LOCALAPPDATA": r"C:\Users\dev\AppData\Local", "LocalAppData": r"C:\Users\dev\AppData\Local",
                 "APPDATA": r"C:\Users\dev\AppData\Roaming", "USERPROFILE": r"C:\Users\dev",
                 "HOMEDRIVE": "C:", "HOMEPATH": r"\Users\dev"}
        env = lint.analysis_env(paths)
        for name, path in paths.items():
            self.assertEqual(env.get(name), path)

    def test_parser_cannot_resolve_or_run_candidate_application_initializers(self):
        tool = self.root / lint.CONTEXT_TOOL
        tool.mkdir(parents=True)
        source = (lint.REPOSITORY / lint.CONTEXT_TOOL / "main.go").read_text()
        source = source.replace('import (', 'import (\n_ "github.com/OpenRAE/go-rae-gremlins/internal/unsafe"', 1)
        (tool / "main.go").write_text(source, encoding="utf-8")
        (self.root / "go.mod").write_text("module github.com/OpenRAE/go-rae-gremlins\n\ngo 1.25.0\n", encoding="utf-8")
        unsafe = self.root / "internal/unsafe"
        unsafe.mkdir(parents=True)
        marker = self.root / "initializer-ran"
        (unsafe / "unsafe.go").write_text('package unsafe\nimport "os"\nfunc init() { _ = os.WriteFile('
                                          + json.dumps(str(marker)) + ', []byte("ran"), 0600) }\n', encoding="utf-8")
        # This stub reproduces the original application's Logger dependency without external modules.
        logger = self.root / "internal/log"
        logger.mkdir(parents=True)
        (logger / "log.go").write_text('package log\nimport "io"\nfunc Init(io.Writer, io.Writer) {}\nfunc Errorln(any) {}\n', encoding="utf-8")
        with mock.patch.object(lint, "REPOSITORY", self.root):
            with self.assertRaises(lint.LintError):
                lint.source_declarations({"a.go": "package a\n"})
        self.assertFalse(marker.exists(), "candidate initializer ran during static lint")

    def test_command_failure_timeout_and_large_output_fail_closed(self):
        for failure in [FileNotFoundError(), subprocess.TimeoutExpired("lint", 1)]:
            with mock.patch.object(lint.subprocess, "run", side_effect=failure):
                with self.assertRaises(lint.LintError):
                    lint.command(self.root, ["golangci-lint"], {})
        failed = subprocess.CompletedProcess([], 3)
        with mock.patch.object(lint.subprocess, "run", return_value=failed):
            with self.assertRaises(lint.LintError):
                lint.command(self.root, ["golangci-lint"], {})
        def oversized(*args, **kwargs):
            kwargs["stdout"].write(b"x" * 65)
            return subprocess.CompletedProcess([], 0)
        with mock.patch.object(lint, "LIMIT", 64), mock.patch.object(lint.subprocess, "run", side_effect=oversized):
            with self.assertRaisesRegex(lint.LintError, "exceeds"):
                lint.command(self.root, ["golangci-lint"], {})


class GitGateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir=Path(__file__).resolve().parent.parent / ".git")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "--quiet")
        (self.root / "a.go").write_text("package a\n", encoding="utf-8")
        self.commit()
        self.a = ("revive", "a.go", "a" * 64)
        self.b = ("gosec", "a.go", "b" * 64)

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), "-c", "core.hooksPath=/dev/null",
                                        "-c", "user.name=Test", "-c", "user.email=test@example.invalid",
                                        *args], stderr=subprocess.DEVNULL)

    def commit(self):
        self.git("add", ".")
        self.git("commit", "--quiet", "--no-gpg-sign", "-m", "test fixture")

    def baseline(self, findings):
        (self.root / ".lint").mkdir(exist_ok=True)
        (self.root / lint.BASELINE).write_bytes(lint.serialize(findings))

    def run_gate(self, actual, base="HEAD", shrink=False, adoption_base=""):
        with mock.patch.object(lint, "analyze", return_value=collections.Counter(actual)):
            return lint.run_gate(self.root, base, shrink=shrink, adoption_base=adoption_base)

    def test_first_adoption_requires_unchanged_go_inputs(self):
        self.baseline({self.a: 1})
        self.run_gate({self.a: 1})
        (self.root / "a.go").write_text("package changed\n", encoding="utf-8")
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1})

    def test_first_adoption_rejects_untracked_go_inputs_and_shrink(self):
        self.baseline({self.a: 1})
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1}, shrink=True)
        (self.root / "new.go").write_text("package a\n", encoding="utf-8")
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1})

    def test_adoption_allows_only_new_debt_free_context_tool(self):
        folder = self.root / lint.CONTEXT_TOOL
        folder.mkdir(parents=True)
        (folder / "main.go").write_text("package main\nfunc main() {}\n", encoding="utf-8")
        self.baseline({self.a: 1})
        self.run_gate({self.a: 1})
        tool_finding = ("revive", lint.CONTEXT_TOOL + "/main.go", "c" * 64)
        self.baseline({self.a: 1, tool_finding: 1})
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1, tool_finding: 1})

    def test_rejects_candidate_growth_even_when_it_covers_new_finding(self):
        self.baseline({self.a: 1})
        self.commit()
        self.baseline({self.a: 1, self.b: 1})
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1, self.b: 1})

    def test_shrink_is_explicit_and_cannot_add_debt(self):
        self.baseline({self.a: 2})
        self.commit()
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1})
        self.run_gate({self.a: 1}, shrink=True)
        self.assertEqual(lint.parse_baseline((self.root / lint.BASELINE).read_bytes()), {self.a: 1})
        with self.assertRaises(lint.LintError):
            self.run_gate({self.b: 1}, shrink=True)

    def test_missing_history_and_deleted_established_baselines_fail(self):
        self.baseline({self.a: 1})
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1}, base="not-a-ref")
        self.commit()
        self.git("rm", "--quiet", lint.BASELINE)
        self.commit()
        self.baseline({self.a: 1})
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1})

    def test_comparison_references_reject_options_and_revision_expressions(self):
        for ref in ["-c", "--output=somewhere", "HEAD;touch", "HEAD\nmain", "HEAD~1", "main..dev"]:
            with self.subTest(ref=ref), mock.patch.object(lint, "git_read") as git:
                with self.assertRaises(lint.LintError):
                    lint.resolve_revision(self.root, ref)
                git.assert_not_called()

    def test_ref_queries_are_stdin_data_and_not_command_arguments(self):
        commit = "a" * 40
        with mock.patch.object(lint, "git_read", return_value=(commit + " commit\n").encode()) as git:
            self.assertEqual(lint.resolve_revision(self.root, "origin/dev"), commit)
            self.assertNotIn("origin/dev", git.call_args.args)
            self.assertEqual(git.call_args.kwargs["input_data"], b"origin/dev^{commit}\n")

    def test_linked_candidate_and_target_baselines_fail(self):
        self.baseline({self.a: 1})
        path = self.root / lint.BASELINE
        path.rename(self.root / "other.json")
        path.symlink_to("../other.json")
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1})
        self.commit()
        path.unlink()
        path.write_bytes(lint.serialize({self.a: 1}))
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1})

    def test_first_promotion_uses_the_reviewed_integration_baseline(self):
        initial = self.git("rev-parse", "HEAD").decode().strip()
        (self.root / "a.go").write_text("package newer\n", encoding="utf-8")
        self.baseline({self.a: 1})
        self.commit()
        self.git("update-ref", "refs/remotes/origin/dev", "HEAD")
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1}, base=initial)
        self.run_gate({self.a: 1}, base=initial, adoption_base="origin/dev")
        self.baseline({self.a: 1, self.b: 1})
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1, self.b: 1}, base=initial, adoption_base="origin/dev")

    def test_promotion_cannot_bypass_an_existing_target_baseline(self):
        self.baseline({self.a: 1})
        self.commit()
        initial = self.git("rev-parse", "HEAD").decode().strip()
        self.baseline({self.a: 1, self.b: 1})
        self.commit()
        self.git("update-ref", "refs/remotes/origin/dev", "HEAD")
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1, self.b: 1}, base=initial, adoption_base="origin/dev")

    def test_promotion_requires_an_existing_baseline_and_ancestor(self):
        initial = self.git("rev-parse", "HEAD").decode().strip()
        self.baseline({self.a: 1})
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1}, adoption_base="HEAD")
        self.commit()
        self.git("update-ref", "refs/remotes/origin/dev", "HEAD")
        self.git("checkout", "--quiet", "--detach", initial)
        self.baseline({self.a: 1})
        with self.assertRaises(lint.LintError):
            self.run_gate({self.a: 1}, adoption_base="origin/dev")


class CLIExitTests(unittest.TestCase):
    def test_failed_gate_returns_a_nonzero_exit_status(self):
        failure = lint.LintError("new finding")
        with mock.patch.object(sys, "argv", ["lint.py", "--base", "HEAD"]), \
                mock.patch.object(lint, "run_gate", side_effect=failure), contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(lint.main(), 1)

    def test_successful_gate_returns_zero(self):
        with mock.patch.object(sys, "argv", ["lint.py", "--base", "HEAD"]), \
                mock.patch.object(lint, "run_gate", return_value=None):
            self.assertEqual(lint.main(), 0)


class CoverageAdmissionTests(unittest.TestCase):
    def test_reports_are_admitted_and_invalid_inputs_are_rejected(self):
        script = lint.REPOSITORY / "scripts/check-coverage.sh"
        for scenario in ("valid", "missing", "empty", "linked", "linked-directory", "extra", "doctype", "entity"):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory(dir=lint.REPOSITORY / ".git") as directory:
                root = Path(directory)
                reports = root / "coverage"
                reports.mkdir()
                (reports / "coverage.out").write_text("mode: atomic\n", encoding="utf-8")
                xml = reports / "python-coverage.xml"
                xml.write_text('<coverage/>\n', encoding="utf-8")
                if scenario == "missing":
                    xml.unlink()
                elif scenario == "empty":
                    xml.write_text("", encoding="utf-8")
                elif scenario == "linked":
                    xml.rename(root / "linked.xml")
                    xml.symlink_to(root / "linked.xml")
                elif scenario == "linked-directory":
                    reports.rename(root / "other")
                    reports.symlink_to(root / "other", target_is_directory=True)
                elif scenario == "extra":
                    (reports / "extra").write_text("unexpected", encoding="utf-8")
                elif scenario == "doctype":
                    xml.write_text('<!DOCTYPE coverage><coverage/>', encoding="utf-8")
                elif scenario == "entity":
                    xml.write_text('<!ENTITY example "unexpected"><coverage/>', encoding="utf-8")
                result = subprocess.run(["bash", str(script)], cwd=root, env=lint.analysis_env(),
                                        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
                self.assertEqual(result.returncode == 0, scenario == "valid")


class PinnedLinterTests(unittest.TestCase):
    def test_configured_google_predicates_and_permitted_exceptions(self):
        root_dir = Path(__file__).resolve().parent.parent
        with tempfile.TemporaryDirectory(dir=root_dir / ".git") as directory:
            root = Path(directory)
            shutil.copyfile(root_dir / ".golangci.yml", root / ".golangci.yml")
            (root / "go.mod").write_text("module example.invalid/style\n\ngo 1.25.0\n", encoding="utf-8")
            modules = {"testify": ("github.com/stretchr/testify", "assert"),
                       "gomega": ("github.com/onsi/gomega", ""),
                       "goconvey": ("github.com/smartystreets/goconvey", "convey"),
                       "is": ("github.com/matryer/is", ""),
                       "assertions": ("example.org/assertions", ""),
                       "prefix": ("github.com/google/go-cmp-unapproved", "cmp"),
                       "comparison": ("github.com/google/go-cmp", "cmp")}
            with (root / "go.mod").open("a", encoding="utf-8") as mod:
                for name, (module, package) in modules.items():
                    folder = root / "testdata" / name
                    (folder / package).mkdir(parents=True)
                    (folder / "go.mod").write_text(f"module {module}\n\ngo 1.25.0\n", encoding="utf-8")
                    (folder / package / "stub.go").write_text(
                        f"package {package or name}\nfunc Check() {{}}\n", encoding="utf-8")
                    if name == "comparison":
                        (folder / package / "stub.go").write_text("package cmp\nfunc Diff(x, y any) string { return \"\" }\n", encoding="utf-8")
                    mod.write(f"\nrequire {module} v0.0.0\nreplace {module} => ./testdata/{name}\n")
            fixtures = {
                "names.go": '''package Style_Bad
type Thing struct{}
func (self Thing) First() {}
func (t Thing) Second() {}
func (t *Thing) Third() {}
func Bad_name() { var user_id int; _ = user_id }
type hidden struct{}
func (h hidden) Exported() {}
type Service interface { Start() }
''',
                "comments.go": '''package Style_Bad
// Documented does a thing
func Documented() {}
// Wrong starts with the wrong name.
func Right() {}
''',
                "discarded.go": '''package Style_Bad
import "os"
func Discarded() { _ = os.Chdir(".") }
''',
                "typeassert.go": '''package Style_Bad
func Asserted(v any) { _, _ = v.(string) }
''',
                "imports.go": '''package Style_Bad
import fmt "fmt"
import . "math"
import _ "net/http/pprof"
func imports() string { return fmt.Sprint(Abs(-1)) }
''',
                "alias.go": '''package Style_Bad
import FMT "fmt"
func alias() string { return FMT.Sprint(1) }
''',
                "errors.go": '''package Style_Bad
import ("errors"; "fmt"; "os")
var badFailure = errors.New("failure")
func BadError() (error, int) { return nil, 0 }
func BadMessage() error { return errors.New("Failure.") }
func Formatting(v int) error { return errors.New(fmt.Sprintf("value %d", v)) }
func Ignored() { os.Chdir("."); _ = os.Chdir("."); _, _ = any(1).(string) }
func Quotes(v string) string { return fmt.Sprintf("value \\\"%s\\\"", v) }
''',
                "contexts.go": '''package Style_Bad
import ("context"; "net/http")
type Stored struct { ctx context.Context }
func WrongContext(n int, ctx context.Context) { _ = n; _ = ctx }
func Key() context.Context { return context.WithValue(context.Background(), "key", 1) }
func Request() { _, _ = http.Get("http://example.invalid") }
func downstream(ctx context.Context) { _ = ctx }
func Dropped(ctx context.Context) { downstream(context.Background()) }
''',
                "values.go": '''package Style_Bad
import ("io"; "sync"; "example.invalid/style/external")
type Guard struct { mu sync.Mutex }
func Copy(g Guard) Guard { return g }
func Pointer(r *io.Reader) { _ = r }
func Unkeyed() external.Pair { return external.Pair{1, 2} }
func Any() interface{} { return nil }
func Slice() []int { return []int{} }
func Simplify(v bool) bool { if v == true { return true }; return false }
func Switch(n int) { switch n { case 1: break } }
func Group(group sync.WaitGroup) { group.Wait() }
''',
                "idioms.go": '''package Style_Bad
import ("fmt"; "time"; "os")
var timeoutSecs time.Duration
type private struct{}
func Private() private { return private{} }
func ReturnErr() error { if err := os.Chdir("."); err != nil { return err }; return nil }
func ErrorFlow(err error) bool { if err != nil { return false } else { return true } }
func Increment(n int) int { n += 1; return n }
func Zero() int { var n int = 0; return n }
func Walk(xs []int) { for _ = range xs {} }
func Unused(n int) {}
func Empty(n int) { if n > 0 {} }
func Builtin() { len := 1; _ = len }
func Format() string { return fmt.Sprintf("hello") }
func Unnecessary(n int) bool { if n > 0 { return true } else { return false } }
func Else(xs []int) { for _, v := range xs { if v < 0 { continue } else { fmt.Println(v) } } }
''',
                "assertions_test.go": '''package Style_Bad
import ("testing"; "github.com/stretchr/testify/assert"; "github.com/onsi/gomega"; "github.com/smartystreets/goconvey/convey")
func TestAssertions(t *testing.T) { assert.Check(); gomega.Check(); convey.Check() }
''',
                "unknown_assertions_test.go": '''package Style_Bad
import ("testing"; "github.com/matryer/is"; "example.org/assertions"; othercmp "github.com/google/go-cmp-unapproved/cmp")
func TestUnknownAssertions(t *testing.T) { is.Check(); assertions.Check(); othercmp.Check() }
''',
                "suppressions.go": '''package Style_Bad
import "os"
func MissingReason() {
//nolint:errcheck
os.Chdir(".")
}
func MissingChecker() {
//nolint // An example of a directive without a checker.
os.Chdir(".")
}
''',
                "platform_windows.go": '''package Style_Bad
func Windows_name() {}
''',
                "external/pair.go": '''// Package external supplies an external struct.
package external
// Pair holds two coordinates.
type Pair struct { X, Y int }
''',
                "returns.go": 'package Style_Bad\nfunc LongReturn() (n int) {\n'
                              + '\n'.join('_ = n' for _ in range(32)) + '\nreturn\n}\n',
                "allowed.go": '''package Style_Bad
import "fmt"
// Results returns a value with a useful name.
func Results() (value int) { return 1 }
// Deferred changes a result in a deferred closure.
func Deferred() (value int) { defer func() { value++ }(); return 1 }
// Local is a local pair.
type Local struct { x, y int }
// LocalLiteral uses a permitted package-local unkeyed literal.
func LocalLiteral() Local { return Local{1, 2} }
// NilSlice uses the preferred nil initialization.
func NilSlice() []int { var values []int; return values }
// Quoted uses the quoting verb.
func Quoted(v string) string { return fmt.Sprintf("value %q", v) }
// ShortReturn has a permitted small naked return.
func ShortReturn() (n int) { return }
''',
                "allowed_test.go": '''package Style_Bad
import ("testing"; "github.com/google/go-cmp/cmp")
func goodHelper(t *testing.T) { t.Helper(); t.Log("useful context") }
func TestGoodHelper(t *testing.T) { t.Setenv("KEY", "value"); goodHelper(t) }
func TestComparison(t *testing.T) { if diff := cmp.Diff(1, 2); diff != "" { t.Errorf("comparison diff (-want +got): %s", diff) } }
''',
                "helpers_test.go": '''package Style_Bad
import ("os"; "testing"; "fmt")
func helper(t *testing.T) { t.Error("failure") }
func TestEnvironment(t *testing.T) { os.Setenv("KEY", "value"); helper(t) }
func Example() { fmt.Println("example") }
''',
            }
            for name, source in fixtures.items():
                path = root / name
                path.parent.mkdir(exist_ok=True)
                path.write_text(source, encoding="utf-8")
            lint.command(root, lint.LINTER + ["fmt", "--config", ".golangci.yml", "./..."], lint.analysis_env())
            diagnostics = {}
            lint.analyze(root, diagnostics)
            messages = list(diagnostics.values())
            expectations = [
                ("", "revive", "package-naming"),
                ("", "revive", "package-comments"),
                ("names.go", "revive", "var-naming"),
                ("names.go", "revive", "receiver-naming"),
                ("names.go", "revive", "exported"),
                ("names.go", "revive", "hidden.Exported"),
                ("names.go", "revive", "Start"),
                ("comments.go", "godot", "period"),
                ("comments.go", "staticcheck", "ST1020"),
                ("imports.go", "revive", "dot-imports"),
                ("imports.go", "revive", "blank-imports"),
                ("imports.go", "revive", "redundant-import-alias"),
                ("alias.go", "revive", "import-alias-naming"),
                ("errors.go", "revive", "error-return"),
                ("errors.go", "revive", "error-strings"),
                ("errors.go", "revive", "errorf"),
                ("errors.go", "revive", "error-naming"),
                ("errors.go", "errcheck", "not checked"),
                ("discarded.go", "errcheck", "not checked"),
                ("typeassert.go", "errcheck", "not checked"),
                ("errors.go", "gocritic", "sprintfQuotedString"),
                ("contexts.go", "containedctx", "context.Context"),
                ("contexts.go", "revive", "context-as-argument"),
                ("contexts.go", "revive", "context-keys-type"),
                ("contexts.go", "noctx", "context"),
                ("contexts.go", "bodyclose", "body"),
                ("contexts.go", "contextcheck", "context"),
                ("values.go", "govet", "copylocks"),
                ("values.go", "govet", "composites"),
                ("values.go", "gocritic", "ptrToRefParam"),
                ("values.go", "revive", "use-any"),
                ("values.go", "revive", "enforce-slice-style"),
                ("values.go", "staticcheck", "S1002"),
                ("values.go", "revive", "useless-break"),
                ("values.go", "revive", "waitgroup-by-value"),
                ("idioms.go", "revive", "unexported-return"),
                ("idioms.go", "revive", "if-return"),
                ("idioms.go", "revive", "increment-decrement"),
                ("idioms.go", "revive", "var-declaration"),
                ("idioms.go", "revive", "unused-parameter"),
                ("idioms.go", "revive", "empty-block"),
                ("idioms.go", "revive", "redefines-builtin-id"),
                ("idioms.go", "revive", "unnecessary-format"),
                ("idioms.go", "revive", "unnecessary-if"),
                ("idioms.go", "revive", "superfluous-else"),
                ("idioms.go", "revive", "indent-error-flow"),
                ("idioms.go", "revive", "time-naming"),
                ("names.go", "recvcheck", "receiver"),
                ("assertions_test.go", "revive", "github.com/stretchr/testify/assert"),
                ("assertions_test.go", "revive", "github.com/onsi/gomega"),
                ("assertions_test.go", "revive", "github.com/smartystreets/goconvey/convey"),
                ("unknown_assertions_test.go", "depguard", "github.com/matryer/is"),
                ("unknown_assertions_test.go", "depguard", "example.org/assertions"),
                ("unknown_assertions_test.go", "depguard", "github.com/google/go-cmp-unapproved/cmp"),
                ("suppressions.go", "nolintlint", "explanation"),
                ("suppressions.go", "nolintlint", "specific"),
                ("platform_windows.go", "revive", "var-naming"),
                ("", "staticcheck", "ST1000"),
                ("", "staticcheck", "ST1003"),
                ("returns.go", "nakedret", "naked return"),
                ("helpers_test.go", "thelper", "Helper"),
                ("helpers_test.go", "usetesting", "Setenv"),
                ("helpers_test.go", "testableexamples", "example"),
                ("helpers_test.go", "errcheck", "fmt.Println"),
            ]
            for name, checker, text in expectations:
                with self.subTest(file=name, checker=checker, predicate=text):
                    self.assertTrue(any(message.startswith(checker + " ") and name in message and text in message
                                        for message in messages), "\n".join(messages))
            self.assertFalse(any("allowed.go" in message and message.startswith(("nakedret ", "govet ", "gocritic "))
                                 for message in messages))
            self.assertFalse(any("allowed_test.go" in message and message.startswith(("thelper ", "usetesting ", "depguard "))
                                 for message in messages))
            self.assertIn("for range xs", (root / "idioms.go").read_text())

    def test_actual_linter_rejects_new_or_baselined_violation_and_analysis_errors(self):
        root_dir = Path(__file__).resolve().parent.parent
        with tempfile.TemporaryDirectory(dir=root_dir / ".git") as directory:
            root = Path(directory)
            shutil.copyfile(root_dir / ".golangci.yml", root / ".golangci.yml")
            (root / "go.mod").write_text("module example.invalid/fixture\n\ngo 1.25.0\n", encoding="utf-8")
            source = root / "fixture.go"
            source.write_text('// Package fixture exercises lint policy.\npackage fixture\n\n'
                              'import "os"\n\n// ChangeDirectory changes the directory.\n'
                              'func ChangeDirectory() {\n\tos.Chdir(".")\n}\n', encoding="utf-8")
            findings = lint.analyze(root)
            self.assertTrue(any(key[0] == "errcheck" for key in findings))
            for candidate in ({}, findings):
                with self.subTest(candidate=candidate):
                    with self.assertRaises(lint.LintError):
                        lint.compare(findings, candidate, {})
            source.write_text(source.read_text().replace('func ChangeDirectory() {\n\tos.Chdir',
                                                        'func ChangeDirectory() error {\n\treturn os.Chdir'),
                              encoding="utf-8")
            self.assertEqual(lint.analyze(root), {})
            source.write_text(source.read_text().replace('return os.Chdir', 'return   os.Chdir'), encoding="utf-8")
            with self.assertRaisesRegex(lint.LintError, "formatting differs"):
                lint.analyze(root)
            source.write_text(source.read_text().replace('return   os.Chdir', 'return os.Missing'), encoding="utf-8")
            with self.assertRaises(lint.LintError):
                lint.analyze(root)


if __name__ == "__main__":
    unittest.main()
