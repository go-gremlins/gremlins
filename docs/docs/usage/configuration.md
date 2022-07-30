# Configuration

Gremlins can be configured via (in order of precedence):

1. command flags
2. environment variables
3. configuration files

## Command flags

Flags have the higher priority and override all the other means of setting behaviours. Please refer to the specific
command documentation to learn how to use them.

## Configuration files

Gremlins can be configured with a configuration file.

### Location

The configuration file can be placed in (in order of precedence)

1. `./.gremlins.yaml` (the current directory)
2. `/etc/gremlins/gremlins.yaml`
3. `$XDG_CONFIG_HOME/gremlins/gremlins.yaml`
4. `$HOME/.gremlins.yaml`

!!! hint
    `XDG_CONFIG_HOME` is usually `~/.config`.

### Override

The config file can be overridden with the `--config` flag.

```shell
gremlins unleash --config=myConfig.yaml
```

### Reference

Here is a complete configuration file with all the properties set to their defaults:

```yaml
unleash:
  dry-run: false
  tags: ""
  threshold: #(1)
    efficacy: 0
    mutant-coverage: 0

mutants:
  arithmetic-base:
    enabled: true
  conditionals-boundary:
    enabled: true
  conditionals-negation:
    enabled: true
  increment-decrement:
    enabled: true
  invert-negatives:
    enabled: true

```

1. Thresholds are set by default to `0`, which means they are not enforced. For further information check the specific
   documentation.

## Environment variables

Gremlins can be configured via environment variables as well. You can construct the variable name referring to the
configuration file format. They start with `GREMLINS_`, and each dot and dash becomes an underscore.

For example:

```yaml
mutants:
  arithmetic-base:
    enabled: true
```

Can be set with:

```shell
export GREMLINS_MUTANTS_ARITHMETIC_BASE=true
```