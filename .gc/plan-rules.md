# Plan rules

- Target pull requests at `dev`. The `main` branch is for reviewed promotions from `dev`.
- Keep `.ground-control.yaml`, the GitHub checks, and the Sonar project key in agreement.
- When a change adds or changes a flag or a configuration key, update `docs/docs/schema/configuration.json` and the
  pages in `docs/docs/usage/` in the same change.
- Keep the CLI flags, the configuration keys, the JSON report, and the exit codes stable unless the issue asks for a
  change. autarchy-ai/coldlight and OpenRAE/env-packs depend on them.
- Do not change the module path `github.com/go-gremlins/gremlins` before an ADR records the decision (#69).
- Do not edit or reformat files in `testdata` directories unless the change is to the test fixture itself.
