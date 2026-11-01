#!/usr/bin/env bash
# install-hooks.sh — activate and verify commit-time pre-commit hooks for THIS clone.
#
# Ground Control onboarding treats pre-commit as an active local gate, but three
# things are distinct contracts (ADR-079): the committed .pre-commit-config.yaml,
# CI pre-commit execution, and commit-time hook ACTIVATION in a given clone.
# `pre-commit install` cowardly-refuses whenever git's core.hooksPath is set, so a
# repo onboarded under a global hooks dispatcher (e.g. ~/.git-hooks with a _chain
# script) can hold a valid config, pass CI, and still run zero hooks on commit —
# silently. This script is the repo-native installer + verifier that closes that gap.
#
# What it does, for the current clone only:
#   * writes a Ground-Control-managed pre-commit and pre-push hook into git's
#     resolved hook path (the location a _chain-style dispatcher delegates to, and
#     the location git uses directly when no dispatcher is set). Each managed hook
#     execs `pre-commit hook-impl` — the same entrypoint `pre-commit install` wires.
#   * PROVES activation by asking git to run the hook (`git hook run`) and checking
#     the managed hook is actually reached through git's EFFECTIVE dispatch, so a
#     green config/CI is never mistaken for a wired commit-time hook.
#   * runs `pre-commit run --all-files` and requires it to pass.
#
# It never unsets or rewrites GLOBAL git config, never overwrites an unmanaged hook
# without --force, and fails closed (non-zero, actionable diagnostic) when git's
# core.hooksPath points at a dispatcher that does not reach this clone's hook path.
#
# Usage: scripts/install-hooks.sh [--force] [--dry-run] [--help]
#   --force     replace an existing unmanaged hook instead of failing closed
#   --dry-run   report the actions without writing hooks or running verification
#   --help      print this header and exit
#
# Fresh clones must re-run this (via `make hooks`) because .git/hooks is not
# versioned — that is by design (ADR-079), not a bug.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

MANAGED_MARKER="ground-control-managed-hook"
MANAGED_VERSION="v1"
# Hook types this repo activates. Add a type here to activate a new stage; the
# managed-hook body and verification are parameterised over it (no new framework).
MANAGED_HOOK_TYPES=(pre-commit pre-push)

force=0
dry_run=0
for arg in "$@"; do
  case "$arg" in
    --force) force=1 ;;
    --dry-run) dry_run=1 ;;
    -h|--help)
      sed -n '2,45p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "install-hooks: unknown argument: $arg" >&2
      exit 2
      ;;
  esac
done

log()  { printf 'install-hooks: %s\n' "$1"; }
fail() { printf 'install-hooks: %s\n' "$1" >&2; exit 1; }

cd "$REPO_ROOT"

# --- Preconditions ----------------------------------------------------------

git rev-parse --is-inside-work-tree >/dev/null 2>&1 \
  || fail "not inside a git work tree ($REPO_ROOT)"

[ -f "$REPO_ROOT/.pre-commit-config.yaml" ] \
  || fail "no .pre-commit-config.yaml at repo root — nothing to activate"

PRE_COMMIT_BIN="$(command -v pre-commit || true)"
[ -n "$PRE_COMMIT_BIN" ] \
  || fail "pre-commit not found on PATH — install it (e.g. 'pipx install pre-commit' or 'pip install pre-commit') and re-run"

# Resolve the interpreter pre-commit runs under, mirroring the shim pre-commit
# itself generates, so the managed hook works even when PATH is thin at commit
# time. Falls back to `command -v pre-commit` inside the hook body.
INSTALL_PYTHON="$(sed -n '1s/^#!//p' "$PRE_COMMIT_BIN" 2>/dev/null || true)"
case "$INSTALL_PYTHON" in
  /*) : ;;              # a real interpreter path from the shebang
  *)  INSTALL_PYTHON="" ;;
esac

HOOKS_PATH="$(git config --get core.hooksPath || true)"

# Resolve the hook directory THIS clone actually dispatches to. Treat every
# git-supplied path as data; do not assume `.git/hooks` is a plain directory.
#
# CRITICAL: `git rev-parse --git-path hooks` HONORS core.hooksPath — when a global
# dispatcher is set it returns that host-owned global dir (e.g. ~/.git-hooks), NOT
# this clone's hooks. Writing there would clobber the host's global hooks. So:
#   * dispatcher set  -> target the clone-local `<git-dir>/hooks`, which is exactly
#     where a _chain-style dispatcher delegates (`$(git rev-parse --git-dir)/hooks`).
#   * no dispatcher   -> use git's resolved hooks path (repo/common `.git/hooks`).
if [ -n "$HOOKS_PATH" ]; then
  HOOK_DIR="$(cd "$REPO_ROOT" && git rev-parse --git-dir)/hooks"
  log "core.hooksPath is set ('$HOOKS_PATH'); pre-commit install would refuse — writing managed hooks into the clone-local hook dir the dispatcher delegates to."
else
  HOOK_DIR="$(cd "$REPO_ROOT" && git rev-parse --git-path hooks)"
  log "core.hooksPath is unset; writing managed hooks into git's hook path."
fi
case "$HOOK_DIR" in
  /*) : ;;
  *)  HOOK_DIR="$REPO_ROOT/$HOOK_DIR" ;;
esac

# --- Hook body --------------------------------------------------------------

# Emit a managed hook script for the given hook type to stdout. The verify probe
# (GC_HOOK_VERIFY) lets install-hooks confirm this exact file is reached through
# git's effective dispatch without mutating the work tree. The body carries no
# secrets, tokens, or network/API calls — pure local dispatch to pre-commit.
managed_hook_body() {
  local hook_type="$1"
  cat <<EOF
#!/usr/bin/env bash
# ${MANAGED_MARKER} ${MANAGED_VERSION} (${hook_type})
# Installed by scripts/install-hooks.sh — do not edit. Regenerate with: make hooks
# Activates .pre-commit-config.yaml at ${hook_type} time, including under a global
# core.hooksPath dispatcher that delegates to this clone's git hook path (ADR-079).
set -euo pipefail

# Activation probe used by scripts/install-hooks.sh — proves git's effective
# dispatch reaches this managed hook, then exits without running pre-commit.
if [ -n "\${GC_HOOK_VERIFY:-}" ]; then
  printf '%s\n' '${MANAGED_MARKER}:${hook_type}'
  exit 0
fi

ARGS=(hook-impl --config=.pre-commit-config.yaml --hook-type=${hook_type} --hook-dir="\$(cd "\$(dirname "\$0")" && pwd)")
INSTALL_PYTHON="${INSTALL_PYTHON}"
if [ -n "\$INSTALL_PYTHON" ] && [ -x "\$INSTALL_PYTHON" ]; then
  exec "\$INSTALL_PYTHON" -mpre_commit "\${ARGS[@]}" -- "\$@"
elif command -v pre-commit >/dev/null 2>&1; then
  exec pre-commit "\${ARGS[@]}" -- "\$@"
else
  echo '${hook_type}: pre-commit not found; run scripts/install-hooks.sh' >&2
  exit 1
fi
EOF
}

# Fail closed unless the existing hook is safe to replace: our managed marker, or a
# stock pre-commit-generated shim. An unmanaged, hand-written hook is never clobbered
# without --force (ADR-079: managed vs user-managed content must be distinguished).
guard_existing_hook() {
  local target="$1" hook_type="$2"
  # A symlink is never GC-managed (we write regular files) and must never be
  # followed — writing through it would clobber the link's target (e.g. a shared
  # dispatcher). Inspect with lstat semantics only.
  if [ -L "$target" ]; then
    if [ "$force" -eq 1 ]; then
      log "replacing symlinked $hook_type hook at $target (--force)"
      return 0
    fi
    fail "refusing to replace symlinked $hook_type hook at $target (-> $(readlink "$target" 2>/dev/null)) — re-run with --force to replace"
  fi
  [ -e "$target" ] || return 0
  if grep -q "$MANAGED_MARKER" "$target" 2>/dev/null; then
    return 0  # ours — safe to re-manage idempotently
  fi
  if grep -q "File generated by pre-commit" "$target" 2>/dev/null; then
    return 0  # stock pre-commit shim — safe to replace with the managed equivalent
  fi
  if [ "$force" -eq 1 ]; then
    log "overwriting unmanaged $hook_type hook at $target (--force)"
    return 0
  fi
  fail "refusing to overwrite unmanaged $hook_type hook at $target — inspect it, then merge its logic or re-run with --force"
}

write_managed_hook() {
  local hook_type="$1"
  local target="$HOOK_DIR/$hook_type"
  guard_existing_hook "$target" "$hook_type"
  if [ "$dry_run" -eq 1 ]; then
    log "[dry-run] would write managed $hook_type hook to $target"
    return 0
  fi
  # Write to a temp file in the same dir, then atomically mv into place. mv
  # replaces the directory entry and NEVER writes through an existing symlink.
  local tmp="$HOOK_DIR/.${hook_type}.gc.$$"
  managed_hook_body "$hook_type" > "$tmp"
  chmod +x "$tmp"
  mv -f "$tmp" "$target"
  log "wrote managed $hook_type hook to $target"
}

# --- Activation proof -------------------------------------------------------

# Prove git's EFFECTIVE dispatch (including any global core.hooksPath dispatcher)
# reaches our managed hook for this type. Never infers activation from config
# presence, installed packages, or CI (ADR-079 Verification Boundary).
verify_activation() {
  local hook_type="$1"
  local out
  if ! out="$(GC_HOOK_VERIFY=1 git hook run "$hook_type" 2>&1)"; then
    # `git hook run` needs git >= 2.36. A non-zero here with no marker means the
    # probe could not be dispatched — surface it rather than passing silently.
    if ! printf '%s' "$out" | grep -q "${MANAGED_MARKER}:${hook_type}"; then
      fail "could not verify $hook_type activation via 'git hook run' (git >= 2.36 required); output: ${out:-<none>}"
    fi
  fi
  printf '%s' "$out" | grep -q "${MANAGED_MARKER}:${hook_type}" || fail \
    "$hook_type hook is NOT reached through git's effective dispatch (core.hooksPath='${HOOKS_PATH:-<unset>}' points at a dispatcher that does not delegate to this clone's hook path). Global git config was left untouched; wire the dispatcher to '$HOOK_DIR' or clear core.hooksPath for this clone, then re-run."
  log "verified: git dispatches $hook_type to the managed hook"
}

# --- Run --------------------------------------------------------------------

mkdir -p "$HOOK_DIR"
for hook_type in "${MANAGED_HOOK_TYPES[@]}"; do
  write_managed_hook "$hook_type"
done

if [ "$dry_run" -eq 1 ]; then
  log "[dry-run] skipping activation proof and 'pre-commit run --all-files'"
  exit 0
fi

for hook_type in "${MANAGED_HOOK_TYPES[@]}"; do
  verify_activation "$hook_type"
done

log "running 'pre-commit run --all-files' to confirm the config executes cleanly…"
"$PRE_COMMIT_BIN" run --all-files
log "commit-time hooks are active and verified for this clone."
