---
title: GitHub Action
---

# GitHub Action

Gremlins can be used in [GitHub Actions](https://github.com/features/actions) through the
official [Run gremlins unleash action](https://github.com/marketplace/actions/run-gremlins-unleash).

## Example usage

```yaml
name: gremlins

on:
  pull_request:
  push:

jobs:
  gremlins:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
    - uses: actions/gremlins-action@v1
      with:
        version: latest
        args: --tags="tag1,tag2"
        workdir: test/dir
```

## Customization

| Name          | Type     | Default  | Description                                              |
|---------------|----------|----------|----------------------------------------------------------|
| `version`[^1] | `string` | `latest` | Te version of Gremlins to use                            | 
| `args`        | `string` |          | The command line arguments to pass to `gremlins unleash` |
| `workdir`     | `string` | `.`      | Working directory relative to repository root            |  

[^1]: Can be `latest`, a fixed version like `v0.1.2` or a semver range like `~0.2`. In this case this will
     return `v0.2.2`.
