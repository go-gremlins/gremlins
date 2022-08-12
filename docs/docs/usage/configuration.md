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
2. The module root
3. `/etc/gremlins/.gremlins.yaml`
4. `$XDG_CONFIG_HOME/gremlins/.gremlins.yaml`
5. `$HOME/.gremlins.yaml`

[//]: # (@formatter:off)
!!! hint
    `XDG_CONFIG_HOME` is usually `~/.config`.

[//]: # (@formatter:on)

### Override

The config file can be overridden with the `--config` flag.

```shell
gremlins unleash --config=myConfig.yaml
```

### Reference

Here is a complete configuration file with all the properties set to their defaults:

```yaml
silent: false
unleash:
  integration: false
  dry-run: false
  tags: ""
  output: ""
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

[//]: # (@formatter:off)
!!! tip
    You can validate the configuration file using the provided JSON Schema (ex. using it in your editor). The schema
    can be found at [{{ config.site_url }}/schema/configuration.json]({{ config.site_url }}/schema/configuration.json). 

[//]: # (@formatter:on)

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