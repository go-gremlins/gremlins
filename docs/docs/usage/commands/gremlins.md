# Gremlins

The `gremlins` command works with command and flags. Think of commands as verbs and flags as adjectives.

```shell
gremlins <command> [flags]
```

If you type

```shell
gremlins
```

a short usage summary will be printed.

At any time, you can get further help writing

```shell
gremlins help <command>
```

## Global flags

Global flags are not command specific.

### Config

:material-flag:`--config` · :material-sign-direction: Default: empty

Overrides the configuration file.

```shell
gremlins <command> --config=config.yml
```
