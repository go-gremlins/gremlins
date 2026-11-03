"""Run complete pinned lint analysis and enforce the shrink-only lint baseline."""

import collections
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
import tempfile


BASELINE = ".lint/baseline.json"
GO_MOD = "go.mod"
LINT_CONFIG = ".golangci.yml"
LIMIT = 32 * 1024 * 1024
LINTER = ["mise", "exec", "--", "golangci-lint"]
CONTEXTS = ("linux", "darwin", "windows")
CONTEXT_TOOL = "tools/lintcontext"
REPOSITORY = Path(__file__).resolve().parent.parent


class LintError(Exception):
    """A failed analysis or baseline contract."""


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise LintError("duplicate JSON key")
        result[key] = value
    return result


def decode(data):
    if len(data) > LIMIT:
        raise LintError("input exceeds 32 MiB; analysis cannot be truncated")
    try:
        return json.loads(data, object_pairs_hook=unique_object)
    except ValueError as exc:
        raise LintError("invalid JSON input") from exc


def valid_path(name):
    return (isinstance(name, str) and bool(name) and "\\" not in name
            and not any(ord(c) < 32 for c in name)
            and not PurePosixPath(name).is_absolute()
            and str(PurePosixPath(name)) == name
            and not any(part in (".", "..") for part in name.split("/"))
            and not re.match(r"^[A-Za-z]:", name))


def parse_baseline(data):
    obj = decode(data)
    if (not isinstance(obj, dict) or set(obj) != {"version", "findings"}
            or type(obj["version"]) is not int or obj["version"] != 1
            or not isinstance(obj["findings"], list)):
        raise LintError("baseline needs version 1 and an explicit findings array")
    findings = collections.Counter()
    for entry in obj["findings"]:
        if (not isinstance(entry, dict) or set(entry) != {"linter", "file", "fingerprint", "count"}
                or not isinstance(entry["linter"], str)
                or not re.fullmatch(r"[a-z][a-z0-9_]*", entry["linter"])
                or entry["linter"] == "typecheck"
                or not valid_path(entry["file"]) or not entry["file"].endswith(".go")
                or not isinstance(entry["fingerprint"], str)
                or not re.fullmatch(r"[0-9a-f]{64}", entry["fingerprint"])
                or type(entry["count"]) is not int or entry["count"] < 1):
            raise LintError("invalid baseline identity or count")
        key = (entry["linter"], entry["file"], entry["fingerprint"])
        if key in findings:
            raise LintError("duplicate baseline identity")
        findings[key] = entry["count"]
    return findings


def serialize(findings):
    entries = [{"linter": key[0], "file": key[1], "fingerprint": key[2], "count": count}
               for key, count in sorted(findings.items())]
    return (json.dumps({"version": 1, "findings": entries}, indent=2) + "\n").encode()


def compare(actual, candidate, target, *, shrink=False):
    for key, count in candidate.items():
        if count > target.get(key, 0):
            raise LintError(f"baseline growth for {key[0]} in {key[1]!r}; restore target allowances")
    for key, count in actual.items():
        if count > candidate.get(key, 0):
            raise LintError(f"new finding for {key[0]} in {key[1]!r}; fix the violation")
    if not shrink and actual != candidate:
        raise LintError("stale lint baseline; fix findings, then run make lint-shrink")


def local_file(root, name):
    if not valid_path(name):
        raise LintError("expected a clean checkout-relative file path")
    current = root
    for part in name.split("/"):
        current /= part
        if current.is_symlink():
            raise LintError(f"linked input {name!r}; use a regular checkout file")
    if not current.is_file() or current.stat().st_size > LIMIT:
        raise LintError(f"missing or oversized input {name!r}")
    return current


def check_diagnostic(issue, root, sources):
    if not isinstance(issue, dict):
        raise LintError("invalid diagnostic")
    checker, message, pos = issue.get("FromLinter"), issue.get("Text"), issue.get("Pos")
    if (not isinstance(checker, str) or not re.fullmatch(r"[a-z][a-z0-9_]*", checker)
            or checker == "typecheck" or not isinstance(message, str) or not message
            or not isinstance(pos, dict)):
        raise LintError("invalid or non-baselinable diagnostic")
    name, line = pos.get("Filename"), pos.get("Line")
    source = local_file(root, name)
    if not name.endswith(".go") or type(line) is not int or line < 1:
        raise LintError("invalid diagnostic location")
    if name not in sources:
        sources[name] = source.read_text(encoding="utf-8")
    if line > len(sources[name].splitlines()):
        raise LintError("diagnostic line is outside its source")
    return checker, message, name, line


def declaration_name(scopes, line):
    for scope in scopes:
        if scope["start"] <= line <= scope["end"]:
            return scope["name"]
    return "file"


def parse_report(data, root, diagnostics=None):
    report = decode(data)
    if (not isinstance(report, dict) or "Issues" not in report
            or not isinstance(report.get("Report"), dict)
            or report["Report"].get("Warnings") or report["Report"].get("Error")
            or report["Issues"] is not None and not isinstance(report["Issues"], list)):
        raise LintError("incomplete lint report or analysis warning/error")
    findings = collections.Counter()
    sources = {}
    checked = [check_diagnostic(issue, root, sources) for issue in report["Issues"] or []]
    declarations = source_declarations(sources) if sources else {}
    for checker, message, name, line in checked:
        lines = sources[name].splitlines()
        enclosing = declaration_name(declarations[name], line)
        before = [s.strip() for s in lines[:line - 1] if s.strip()][-3:]
        after = [s.strip() for s in lines[line:] if s.strip()][:3]
        context = [before, lines[line - 1].strip(), after]
        fingerprint = hashlib.sha256(json.dumps([enclosing, message, context], ensure_ascii=True).encode()).hexdigest()
        key = (checker, name, fingerprint)
        findings[key] += 1
        if diagnostics is not None:
            diagnostics[key] = f"{checker} {name!r}:{line}: {message!r}"
    return findings


def source_declarations(sources):
    env = analysis_env()
    env.pop("GOOS", None)
    env.pop("GOARCH", None)
    env["GO111MODULE"] = "off"
    data = json.dumps(sources).encode()
    if len(data) > LIMIT:
        raise LintError("source context exceeds 32 MiB")
    with tempfile.TemporaryDirectory(dir=REPOSITORY) as workspace:
        env["GOPATH"] = workspace
        declarations = decode(command(REPOSITORY, ["go", "run", str(REPOSITORY / CONTEXT_TOOL / "main.go")], env, input_data=data))
    if not isinstance(declarations, dict) or set(declarations) != set(sources):
        raise LintError("incomplete declaration context")
    for name, scopes in declarations.items():
        if not isinstance(scopes, list):
            raise LintError("invalid declaration context")
        for scope in scopes:
            if (not isinstance(scope, dict) or set(scope) != {"start", "end", "name"}
                    or type(scope["start"]) is not int or type(scope["end"]) is not int
                    or not 1 <= scope["start"] <= scope["end"] <= len(sources[name].splitlines())
                    or not isinstance(scope["name"], str) or not scope["name"]):
                raise LintError("invalid declaration context")
    return declarations


def analysis_env(environ=None):
    # Keep only tool/cache locations and basic OS settings, never ambient credentials or build execution overrides.
    allowed = {"PATH", "HOME", "USER", "LOGNAME", "SYSTEMROOT", "SystemRoot", "WINDIR", "TEMP", "TMP", "TMPDIR",
               "LOCALAPPDATA", "LocalAppData", "APPDATA", "USERPROFILE", "HOMEDRIVE", "HOMEPATH",
               "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "MISE_DATA_DIR", "MISE_CONFIG_DIR", "MISE_CACHE_DIR",
               "GOPATH", "GOCACHE", "GOMODCACHE", "LANG", "LC_ALL", "NO_COLOR"}
    env = {key: value for key, value in (os.environ if environ is None else environ).items()
           if key in allowed}
    env.update(GOENV="off", GOWORK="off", GOFLAGS="", GOTOOLCHAIN="local",
               GOPROXY="off", GOSUMDB="off", CGO_ENABLED="0", GOARCH="amd64",
               GOOS="linux", GIT_TERMINAL_PROMPT="0")
    return env


def command(root, args, env, allowed=(0,), input_data=None):
    try:
        with tempfile.TemporaryFile(dir=root) as output, tempfile.TemporaryFile(dir=root) as errors:
            stdin = {"stdin": subprocess.DEVNULL} if input_data is None else {"input": input_data}
            result = subprocess.run(args, cwd=root, env=env, **stdin,
                                    stdout=output, stderr=errors, timeout=600, check=False)
            if result.returncode not in allowed or result.returncode != 0 and output.tell() == 0:
                raise LintError(f"{args[0]!r} failed (exit {result.returncode}); analysis cannot be baselined")
            if output.tell() > LIMIT or errors.tell() > LIMIT:
                raise LintError("command output exceeds 32 MiB; analysis cannot be truncated")
            output.seek(0)
            return output.read()
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise LintError(f"{args[0]!r} unavailable or timed out; provision tools and retry") from exc


def validate_checkout(root):
    for directory, names, files in os.walk(root, followlinks=False):
        names[:] = [name for name in names if name not in (".git", "vendor", "testdata", "dist", "__pycache__")]
        for name in names:
            if (Path(directory) / name).is_symlink():
                raise LintError("linked source directory; analysis must stay inside the checkout")
        for name in files:
            check_source_file(root, Path(directory), name)


def check_source_file(root, directory, name):
    if name.endswith(".go") or name in (GO_MOD, "go.sum"):
        local_file(root, (directory / name).relative_to(root).as_posix())
        if name == GO_MOD and directory != root:
            raise LintError("unexpected nested module outside fixture/vendor/output paths")


def analyze(root, diagnostics=None):
    env = analysis_env()
    validate_checkout(root)
    local_file(root, LINT_CONFIG)
    command(root, LINTER + ["config", "verify", "--config", LINT_CONFIG], env)
    # go mod edit reads module metadata without downloading or executing modules.
    module = decode(command(root, ["go", "mod", "edit", "-json"], env))
    for replacement in module.get("Replace") or []:
        new = replacement["New"]
        if not new.get("Version"):
            destination = (root / new["Path"]).resolve()
            if not destination.is_relative_to(root):
                raise LintError("local module replacement escapes the checkout")
    diff = command(root, LINTER + ["fmt", "--config", LINT_CONFIG, "--diff", "./..."], env, allowed=(0, 1))
    if diff.strip():
        raise LintError("formatting differs; run mise exec -- golangci-lint fmt on the changed files")
    actual = collections.Counter()
    for context in CONTEXTS:
        env["GOOS"] = context
        print(f"Analyzing {context}/amd64 (tests included)", flush=True)
        data = command(root, LINTER + ["run", "--config", LINT_CONFIG, "--issues-exit-code=0",
                       "--show-stats=false", "--output.json.path=stdout", f"--output.text.path={os.devnull}",
                       "--max-issues-per-linter=0", "--max-same-issues=0", "--uniq-by-line=false", "./..."], env)
        actual |= parse_report(data, root, diagnostics)
    return actual


def git_read(root, *args, input_data=None):
    return command(root, ["git", "--no-pager", "-c", "core.fsmonitor=false", *args], analysis_env(), input_data=input_data)


def resolve_revision(root, ref):
    match = re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._/-]{0,199}", ref)
    if match is None or ".." in ref:
        raise LintError("comparison must be a named Git ref or full commit hash, never options or revision expressions")
    query = (match.group(0) + "^{commit}\n").encode()
    result = git_read(root, "cat-file", "--batch-check=%(objectname) %(objecttype)", input_data=query).decode().split()
    if len(result) != 2 or result[1] != "commit" or not re.fullmatch(r"[0-9a-f]{40,64}", result[0]):
        raise LintError("comparison did not resolve to a commit")
    return result[0]


def promotion_findings(root, adoption_base):
    adopted = resolve_revision(root, adoption_base)
    ancestor = git_read(root, "merge-base", "HEAD", adopted).decode().strip()
    if ancestor != adopted:
        raise LintError("promotion head must contain the reviewed adoption commit")
    allowed = target_findings(root, adopted)
    if allowed is None:
        raise LintError("promotion adoption base must already contain a lint baseline")
    return allowed


def target_findings(root, base, adoption_base=""):
    revision = resolve_revision(root, base)
    listing = git_read(root, "ls-tree", revision, "--", BASELINE)
    if listing:
        if not listing.startswith((b"100644 blob ", b"100755 blob ")):
            raise LintError("target baseline must be a regular file")
        return parse_baseline(git_read(root, "show", revision + ":" + BASELINE))
    # Absence is adoption only before this check has ever existed in target history.
    history = git_read(root, "log", "-1", "--format=%H", revision, "--", BASELINE, "scripts/lint.py")
    if history.strip():
        raise LintError("established lint baseline is missing from the target")
    if adoption_base:
        return promotion_findings(root, adoption_base)
    paths = ["*.go", GO_MOD, "go.sum", "**/go.mod", "**/go.sum"]
    changed = git_read(root, "diff", "--no-ext-diff", "--no-textconv", "--name-only", "-z", revision, "--", *paths)
    untracked = git_read(root, "ls-files", "--others", "--exclude-standard", "-z", "--", *paths)
    if changed or untracked:
        prior_tool = git_read(root, "ls-tree", "-r", "--name-only", revision, "--", CONTEXT_TOOL)
        additions = (changed + untracked).decode().strip("\x00").split("\x00")
        if prior_tool or any(not name.startswith(CONTEXT_TOOL + "/") or not name.endswith(".go") for name in additions):
            raise LintError("first adoption requires unchanged existing Go source and dependency inputs")
    return None


def run_gate(root, base, *, shrink=False, adoption_base=""):
    path = local_file(root, BASELINE)
    candidate = parse_baseline(path.read_bytes())
    target = target_findings(root, base, adoption_base)
    if target is None and shrink:
        raise LintError("shrink cannot initialize a baseline")
    if target is None and any(key[1].startswith(CONTEXT_TOOL + "/") for key in candidate):
        raise LintError("the new declaration tool must have no lint baseline debt")
    diagnostics = {}
    actual = analyze(root, diagnostics)
    for key in sorted(diagnostics):
        kind = "new" if actual[key] > candidate.get(key, 0) else "baseline"
        print(f"{kind} x{actual[key]}: {diagnostics[key]}")
    # First adoption is the only permitted initial allowance set. All source inputs were checked above.
    compare(actual, candidate, actual if target is None else target, shrink=shrink)
    if shrink:
        data = serialize(actual)
        temporary = None
        try:
            with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as output:
                temporary = Path(output.name)
                output.write(data)
                output.flush()
                os.fsync(output.fileno())
            local_file(root, BASELINE)
            os.replace(temporary, path)
        finally:
            if temporary is not None:
                temporary.unlink(missing_ok=True)
    print(f"Lint passed: {sum(actual.values())} findings in {len(actual)} identities; baseline only shrinks.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="origin/dev", help="comparison commit or ref (must exist locally)")
    parser.add_argument("--shrink", action="store_true", help="remove only stale baseline allowances")
    parser.add_argument("--adoption-base", default="", help="reviewed integration ref for the first main promotion")
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    try:
        if not args.base:
            raise LintError("an explicit comparison commit or ref is required")
        run_gate(root, args.base, shrink=args.shrink, adoption_base=args.adoption_base)
    except (LintError, OSError, UnicodeError) as exc:
        print(f"Lint failed: {str(exc)!r}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
